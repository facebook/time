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
package server

import (
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	ptp "github.com/facebookincubator/ptp/protocol"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// SubscriptionClient is sending subscriptionType messages periodically
type SubscriptionClient struct {
	sync.Mutex

	worker           *sendWorker
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
func NewSubscriptionClient(w *sendWorker, eclisa, gclisa unix.Sockaddr, st ptp.MessageType, sc *Config, i time.Duration, e time.Time) *SubscriptionClient {
	s := &SubscriptionClient{
		eclisa:           eclisa,
		gclisa:           gclisa,
		subscriptionType: st,
		interval:         i,
		expire:           e,
		worker:           w,
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
func (sc *SubscriptionClient) Start() {
	sc.SetRunning(true)
	defer sc.Stop()
	/*
		Calculate the load we add to the worker. Ex:
		sc.interval = 10000ms. l = 1
		sc.interval = 2000ms.  l = 5
		sc.interval = 500ms.   l = 20
		sc.interval = 7ms.     l = 1428
		https://play.golang.org/p/XKnACWjKd24
	*/
	l := 10 * time.Second.Microseconds() / sc.interval.Microseconds()
	atomic.AddInt64(&sc.worker.load, l)
	defer atomic.AddInt64(&sc.worker.load, -l)
	log.Infof("Starting a new %s subscription for %s", sc.subscriptionType, ptp.SockaddrToIP(sc.eclisa))
	over := fmt.Sprintf("Subscription %s is over for %s", sc.subscriptionType, ptp.SockaddrToIP(sc.eclisa))

	// Send first message right away
	sc.Once()

	intervalTicker := time.NewTicker(sc.interval)
	oldInterval := sc.interval

	defer intervalTicker.Stop()
	for range intervalTicker.C {
		if !sc.Running() {
			return
		}
		if time.Now().After(sc.expire) {
			log.Infof(over)
			// TODO send cancellation
			return
		}
		// check if interval changed, maybe update our ticker
		if oldInterval != sc.interval {
			intervalTicker.Reset(sc.interval)
			oldInterval = sc.interval
		}
		// Add myself to the worker queue
		sc.Once()
	}
}

// Once adds itself to the worker queue once
func (sc *SubscriptionClient) Once() {
	sc.worker.queue <- sc
}

// setRunning sets running with the lock
func (sc *SubscriptionClient) SetRunning(running bool) {
	sc.Lock()
	defer sc.Unlock()
	sc.running = running
}

// Running returns the status of the Subscription
func (sc *SubscriptionClient) Running() bool {
	sc.Lock()
	defer sc.Unlock()
	return sc.running
}

// Stop stops the subscription
func (sc *SubscriptionClient) Stop() {
	sc.SetRunning(false)
}

// Stop stops the subscription
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
			MessageLength:   uint16(binary.Size(ptp.Announce{})),
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
				ClockClass:              6,
				ClockAccuracy:           33, // 0x21 - Time Accurate within 100ns
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
		Header:             ptp.Header{},
		TargetPortIdentity: ptp.PortIdentity{},
		TLVs: []ptp.TLV{
			&ptp.GrantUnicastTransmissionTLV{},
		},
	}
}

// UpdateGrant updates ptp Signaling packet granting the requested subscription
func (sc *SubscriptionClient) UpdateGrant(sg *ptp.Signaling, mt ptp.UnicastMsgTypeAndFlags, interval ptp.LogInterval, duration uint32) {
	sc.grant.Header = sg.Header
	sc.grant.TargetPortIdentity = sg.SourcePortIdentity
	sc.grant.Header.FlagField = ptp.FlagUnicast
	sc.grant.Header.SourcePortIdentity = ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: sc.serverConfig.clockIdentity,
	}
	sc.grant.Header.MessageLength = uint16(binary.Size(ptp.Header{}) + binary.Size(ptp.PortIdentity{}) + binary.Size(ptp.GrantUnicastTransmissionTLV{}))

	sc.grant.TLVs[0] = &ptp.GrantUnicastTransmissionTLV{
		TLVHead: ptp.TLVHead{
			TLVType:     ptp.TLVGrantUnicastTransmission,
			LengthField: uint16(binary.Size(ptp.GrantUnicastTransmissionTLV{}) - binary.Size(ptp.TLVHead{})),
		},
		MsgTypeAndReserved:    mt,
		LogInterMessagePeriod: interval,
		DurationField:         duration,
		Reserved:              0,
		Renewal:               1,
	}
}

// Grant returns ptp Signaling packet granting the requested subscription
func (sc *SubscriptionClient) Grant() *ptp.Signaling {
	return sc.grant
}

type syncMapCli struct {
	sync.Mutex
	m map[ptp.PortIdentity]*syncMapSub
}

// init initializes the underlying map
func (s *syncMapCli) init() {
	s.m = make(map[ptp.PortIdentity]*syncMapSub)
}

// load gets the value by the key
func (s *syncMapCli) load(key ptp.PortIdentity) (*syncMapSub, bool) {
	s.Lock()
	defer s.Unlock()
	subs, found := s.m[key]
	return subs, found
}

// store saves the value with the key
func (s *syncMapCli) store(key ptp.PortIdentity, val *syncMapSub) {
	s.Lock()
	if _, ok := s.m[key]; !ok {
		subs := &syncMapSub{}
		subs.init()
		s.m[key] = subs
	}
	s.m[key] = val
	s.Unlock()
}

// delete deletes the value by the key
func (s *syncMapCli) delete(key ptp.PortIdentity) {
	s.Lock()
	delete(s.m, key)
	s.Unlock()
}

// keys returns slice of keys of the underlying map
func (s *syncMapCli) keys() []ptp.PortIdentity {
	keys := make([]ptp.PortIdentity, 0, len(s.m))
	s.Lock()
	for k := range s.m {
		keys = append(keys, k)
	}
	s.Unlock()
	return keys
}

type syncMapSub struct {
	sync.Mutex
	m map[ptp.MessageType]*SubscriptionClient
}

// init initializes the underlying map
func (s *syncMapSub) init() {
	s.m = make(map[ptp.MessageType]*SubscriptionClient)
}

// load gets the value by the key
func (s *syncMapSub) load(key ptp.MessageType) (*SubscriptionClient, bool) {
	s.Lock()
	defer s.Unlock()
	sc, found := s.m[key]
	return sc, found
}

// store saves the value with the key
func (s *syncMapSub) store(key ptp.MessageType, val *SubscriptionClient) {
	s.Lock()
	s.m[key] = val
	s.Unlock()
}

// keys returns slice of keys of the underlying map
func (s *syncMapSub) keys() []ptp.MessageType {
	keys := make([]ptp.MessageType, 0, len(s.m))
	s.Lock()
	for k := range s.m {
		keys = append(keys, k)
	}
	s.Unlock()
	return keys
}
