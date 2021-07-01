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
	"net"
	"sync"
	"time"

	ptp "github.com/facebookincubator/ptp/protocol"
	log "github.com/sirupsen/logrus"
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

	// addresses
	ecliAddr *net.UDPAddr
	gcliAddr *net.UDPAddr

	// packets
	syncP     *ptp.SyncDelayReq
	followupP *ptp.FollowUp
	announceP *ptp.Announce
}

// NewSubscriptionClient gets minimal required arguments to create a subscription
func NewSubscriptionClient(q chan *SubscriptionClient, ip net.IP, st ptp.MessageType, sc *Config, i time.Duration, e time.Time) *SubscriptionClient {
	s := &SubscriptionClient{
		ecliAddr:         &net.UDPAddr{IP: ip, Port: ptp.PortEvent},
		gcliAddr:         &net.UDPAddr{IP: ip, Port: ptp.PortGeneral},
		subscriptionType: st,
		interval:         i,
		expire:           e,
		queue:            q,
		serverConfig:     sc,
	}

	s.initSync()
	s.initFollowup()
	s.initAnnounce()

	return s
}

// Start launches the subscription timers and exit on expire
func (sc *SubscriptionClient) Start() {
	sc.setRunning(true)
	defer sc.Stop()
	l := 10 * time.Second.Microseconds() / sc.interval.Microseconds()
	log.Infof("Starting a new %s subscription for %s", sc.subscriptionType, sc.ecliAddr.IP)
	log.Debugf("Starting a new %s subscription for %s with load %d with interval %s which expires in %s", sc.subscriptionType, sc.ecliAddr.IP, l, sc.interval, sc.expire)

	// Send first message right away
	sc.queue <- sc

	intervalTicker := time.NewTicker(sc.interval)
	oldInterval := sc.interval

	defer intervalTicker.Stop()
	for range intervalTicker.C {
		log.Debugf("Subscription %s for %s is valid until %s", sc.subscriptionType, sc.ecliAddr.IP, sc.expire)
		if !sc.Running() {
			return
		}
		if time.Now().After(sc.expire) {
			log.Infof("Subscription %s is over for %s", sc.subscriptionType, sc.ecliAddr.IP)
			// TODO send cancellation
			return
		}
		// check if interval changed, maybe update our ticker
		if oldInterval != sc.interval {
			intervalTicker.Reset(sc.interval)
			oldInterval = sc.interval
		}
		// Add myself to the worker queue
		sc.queue <- sc
	}
}

// setRunning sets running with the lock
func (sc *SubscriptionClient) setRunning(running bool) {
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
	sc.setRunning(false)
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

// syncPacket generates ptp Sync packet
func (sc *SubscriptionClient) syncPacket() *ptp.SyncDelayReq {
	sc.syncP.SequenceID = sc.sequenceID
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

// followupPacket generates ptp Follow Up packet
func (sc *SubscriptionClient) followupPacket(hwts time.Time) *ptp.FollowUp {
	i, err := ptp.NewLogInterval(sc.interval)
	if err != nil {
		log.Errorf("Failed to get interval: %v", err)
	}

	sc.followupP.SequenceID = sc.sequenceID
	sc.followupP.LogMessageInterval = i
	sc.followupP.PreciseOriginTimestamp = ptp.NewTimestamp(hwts)
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

// announcePacket generates ptp Announce packet
func (sc *SubscriptionClient) announcePacket() *ptp.Announce {
	i, err := ptp.NewLogInterval(sc.interval)
	if err != nil {
		log.Errorf("Failed to get interval: %v", err)
	}

	sc.announceP.SequenceID = sc.sequenceID
	sc.announceP.LogMessageInterval = i
	sc.announceP.CurrentUTCOffset = int16(sc.serverConfig.UTCOffset.Seconds())

	return sc.announceP
}
