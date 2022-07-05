/*
Copyright (c) Facebook, Inc. and its affiliates.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

/*
Package server implements simple UDP server to work with NTP packets.
In addition, it run checker, announce and stats implementations
*/
package server

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/timestamp"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// SubscriptionClient is sending subscriptionType messages periodically
type SubscriptionClient struct {
	sync.Mutex

	queue            chan *SubscriptionClient
	subscriptionType ptp.MessageType
	serverConfig     *Config

	interval   time.Duration
	expire     time.Time
	sequenceID uint16
	running    bool

	// socket addresses
	eclisa unix.Sockaddr
	gclisa unix.Sockaddr

	// packets
	syncP      *ptp.SyncDelayReq
	followupP  *ptp.FollowUp
	announceP  *ptp.Announce
	delayRespP *ptp.DelayResp
	grant      *ptp.Signaling
}

// NewSubscriptionClient gets minimal required arguments to create a subscription
func NewSubscriptionClient(q chan *SubscriptionClient, eclisa, gclisa unix.Sockaddr, st ptp.MessageType, sc *Config, i time.Duration, e time.Time) *SubscriptionClient {
	s := &SubscriptionClient{
		eclisa:           eclisa,
		gclisa:           gclisa,
		subscriptionType: st,
		interval:         i,
		expire:           e,
		queue:            q,
		serverConfig:     sc,
	}

	s.initSync()
	s.initFollowup()
	s.initAnnounce()
	s.initDelayResp()
	s.initGrant()

	return s
}

// Start launches the subscription timers and exit on expire
func (sc *SubscriptionClient) Start(ctx context.Context) {
	log.Infof("Starting a new %s subscription for %s", sc.subscriptionType, timestamp.SockaddrToIP(sc.eclisa))
	sc.setRunning(true)

	over := fmt.Sprintf("Subscription %s is over for %s", sc.subscriptionType, timestamp.SockaddrToIP(sc.eclisa))
	// Send first message right away
	if sc.subscriptionType != ptp.MessageDelayResp {
		sc.Once()
	}

	intervalTicker := time.NewTicker(sc.interval)
	oldInterval := sc.interval

	defer intervalTicker.Stop()
	defer sc.setRunning(false)

	for {
		select {
		case <-ctx.Done():
			log.Infof(over)
			// TODO send cancellation
			return

		case <-intervalTicker.C:
			if sc.Expired() {
				log.Infof(over)
				// TODO send cancellation
				return
			}
			// check if interval changed, maybe update our ticker
			if oldInterval != sc.interval {
				intervalTicker.Reset(sc.interval)
				oldInterval = sc.interval
			}
			if sc.subscriptionType != ptp.MessageDelayResp {
				// Add myself to the worker queue
				sc.Once()
			}
		}
	}
}

// Once adds itself to the worker queue once
func (sc *SubscriptionClient) Once() {
	sc.queue <- sc
}

// Expired checks if the subscription expired or not
func (sc *SubscriptionClient) Expired() bool {
	sc.Lock()
	defer sc.Unlock()
	return time.Now().After(sc.expire)
}

// Stop stops the subscription
func (sc *SubscriptionClient) Stop() {
	sc.Lock()
	defer sc.Unlock()
	// Simply set the expiration time and subscription will be stopped
	sc.expire = time.Now()
}

// setRunning atomically sets running
func (sc *SubscriptionClient) setRunning(running bool) {
	sc.Lock()
	defer sc.Unlock()
	sc.running = running
}

// setExpire atomically sets expire
func (sc *SubscriptionClient) setExpire(expire time.Time) {
	sc.Lock()
	defer sc.Unlock()
	sc.expire = expire
}

// setInterval atomically sets interval
func (sc *SubscriptionClient) setInterval(interval time.Duration) {
	sc.Lock()
	defer sc.Unlock()
	sc.interval = interval
}

// Running returns the running bool
func (sc *SubscriptionClient) Running() bool {
	sc.Lock()
	defer sc.Unlock()
	return sc.running
}

// IncSequenceID adds 1 to a sequence id
func (sc *SubscriptionClient) IncSequenceID() {
	sc.sequenceID++
}

func (sc *SubscriptionClient) initSync() {
	sc.syncP = &ptp.SyncDelayReq{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageSync, 0),
			Version:         ptp.Version,
			MessageLength:   uint16(binary.Size(ptp.SyncDelayReq{})),
			DomainNumber:    0,
			FlagField:       ptp.FlagUnicast | ptp.FlagTwoStep,
			SequenceID:      0,
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber:    1,
				ClockIdentity: sc.serverConfig.clockIdentity,
			},
			LogMessageInterval: 0x7f,
			ControlField:       0,
		},
	}
}

// UpdateSync updates ptp Sync packet
func (sc *SubscriptionClient) UpdateSync() {
	sc.syncP.SequenceID = sc.sequenceID
}

// Sync returns ptp Sync packet
func (sc *SubscriptionClient) Sync() *ptp.SyncDelayReq {
	return sc.syncP
}

func (sc *SubscriptionClient) initFollowup() {
	sc.followupP = &ptp.FollowUp{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageFollowUp, 0),
			Version:         ptp.Version,
			MessageLength:   uint16(binary.Size(ptp.FollowUp{})),
			DomainNumber:    0,
			FlagField:       ptp.FlagUnicast,
			SequenceID:      0,
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber:    1,
				ClockIdentity: sc.serverConfig.clockIdentity,
			},
			LogMessageInterval: 0,
			ControlField:       2,
		},
		FollowUpBody: ptp.FollowUpBody{
			PreciseOriginTimestamp: ptp.NewTimestamp(time.Now()),
		},
	}
}

// UpdateFollowup updates ptp Follow Up packet
func (sc *SubscriptionClient) UpdateFollowup(hwts time.Time) {
	i, _ := ptp.NewLogInterval(sc.interval)
	sc.followupP.SequenceID = sc.sequenceID
	sc.followupP.LogMessageInterval = i
	sc.followupP.PreciseOriginTimestamp = ptp.NewTimestamp(hwts)
}

// Followup returns ptp Follow Up packet
func (sc *SubscriptionClient) Followup() *ptp.FollowUp {
	return sc.followupP
}

func (sc *SubscriptionClient) initAnnounce() {
	sc.announceP = &ptp.Announce{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageAnnounce, 0),
			Version:         ptp.Version,
			MessageLength:   uint16(binary.Size(ptp.Announce{}) + binary.Size(ptp.AnnounceBody{})),
			DomainNumber:    0,
			FlagField:       ptp.FlagUnicast | ptp.FlagPTPTimescale,
			SequenceID:      0,
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber:    1,
				ClockIdentity: sc.serverConfig.clockIdentity,
			},
			LogMessageInterval: 0,
			ControlField:       5,
		},
		AnnounceBody: ptp.AnnounceBody{
			CurrentUTCOffset:     0,
			Reserved:             0,
			GrandmasterPriority1: 128,
			GrandmasterClockQuality: ptp.ClockQuality{
				ClockClass:              0,
				ClockAccuracy:           0,
				OffsetScaledLogVariance: 23008,
			},
			GrandmasterPriority2: 128,
			GrandmasterIdentity:  sc.serverConfig.clockIdentity,
			StepsRemoved:         0,
			TimeSource:           ptp.TimeSourceGNSS,
		},
	}
}

// UpdateAnnounce updates ptp Announce packet
func (sc *SubscriptionClient) UpdateAnnounce() {
	i, _ := ptp.NewLogInterval(sc.interval)
	sc.announceP.SequenceID = sc.sequenceID
	sc.announceP.LogMessageInterval = i
	sc.announceP.CurrentUTCOffset = int16(sc.serverConfig.UTCOffset.Seconds())
	sc.announceP.GrandmasterClockQuality.ClockClass = sc.serverConfig.ClockClass
	sc.announceP.GrandmasterClockQuality.ClockAccuracy = sc.serverConfig.ClockAccuracy
}

// Announce returns ptp Announce packet
func (sc *SubscriptionClient) Announce() *ptp.Announce {
	return sc.announceP
}

func (sc *SubscriptionClient) initDelayResp() {
	sc.delayRespP = &ptp.DelayResp{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageDelayResp, 0),
			Version:         ptp.Version,
			MessageLength:   uint16(binary.Size(ptp.DelayResp{})),
			DomainNumber:    0,
			FlagField:       ptp.FlagUnicast,
			SequenceID:      0,
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber:    1,
				ClockIdentity: sc.serverConfig.clockIdentity,
			},
			LogMessageInterval: 0x7f,
			ControlField:       3,
			CorrectionField:    0,
		},
		DelayRespBody: ptp.DelayRespBody{},
	}
}

// UpdateDelayResp updates ptp Delay Response packet
func (sc *SubscriptionClient) UpdateDelayResp(h *ptp.Header, received time.Time) {
	sc.delayRespP.SequenceID = h.SequenceID
	sc.delayRespP.CorrectionField = h.CorrectionField
	sc.delayRespP.DelayRespBody = ptp.DelayRespBody{
		ReceiveTimestamp:       ptp.NewTimestamp(received),
		RequestingPortIdentity: h.SourcePortIdentity,
	}
}

// DelayResp returns ptp Delay Response packet
func (sc *SubscriptionClient) DelayResp() *ptp.DelayResp {
	return sc.delayRespP
}

func (sc *SubscriptionClient) initGrant() {
	sc.grant = &ptp.Signaling{
		Header: ptp.Header{
			Version:       ptp.Version,
			MessageLength: uint16(binary.Size(ptp.Header{}) + binary.Size(ptp.PortIdentity{}) + binary.Size(ptp.GrantUnicastTransmissionTLV{})),
			FlagField:     ptp.FlagUnicast,
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber:    1,
				ClockIdentity: sc.serverConfig.clockIdentity,
			},
		},
		TargetPortIdentity: ptp.PortIdentity{},
		TLVs: []ptp.TLV{
			&ptp.GrantUnicastTransmissionTLV{
				TLVHead:  ptp.TLVHead{TLVType: ptp.TLVGrantUnicastTransmission, LengthField: uint16(binary.Size(ptp.GrantUnicastTransmissionTLV{}) - binary.Size(ptp.TLVHead{}))},
				Reserved: 0,
				Renewal:  1,
			},
		},
	}
}

// UpdateGrant updates ptp Signaling packet granting the requested subscription
func (sc *SubscriptionClient) UpdateGrant(sg *ptp.Signaling, mt ptp.UnicastMsgTypeAndFlags, interval ptp.LogInterval, duration uint32) {
	sc.grant.Header.SdoIDAndMsgType = sg.Header.SdoIDAndMsgType
	sc.grant.Header.DomainNumber = sg.Header.DomainNumber
	sc.grant.Header.MinorSdoID = sg.Header.MinorSdoID
	sc.grant.Header.CorrectionField = sg.Header.CorrectionField
	sc.grant.Header.MessageTypeSpecific = sg.Header.MessageTypeSpecific
	sc.grant.Header.SequenceID = sg.Header.SequenceID
	sc.grant.Header.ControlField = sg.Header.ControlField
	sc.grant.Header.LogMessageInterval = sg.Header.LogMessageInterval

	sc.grant.TargetPortIdentity = sg.SourcePortIdentity
	tlv := sc.grant.TLVs[0].(*ptp.GrantUnicastTransmissionTLV)

	tlv.MsgTypeAndReserved = mt
	tlv.LogInterMessagePeriod = interval
	tlv.DurationField = duration

	sc.grant.TLVs[0] = tlv
}

// Grant returns ptp Signaling packet granting the requested subscription
func (sc *SubscriptionClient) Grant() *ptp.Signaling {
	return sc.grant
}
