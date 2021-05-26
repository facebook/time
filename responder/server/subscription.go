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
	"sync/atomic"
	"time"

	ptp "github.com/facebookincubator/ptp/protocol"
	log "github.com/sirupsen/logrus"
)

// SubscriptionClient is sending subscriptionType messages periodically
type SubscriptionClient struct {
	sync.Mutex

	clientIP         net.IP
	worker           *sendWorker
	subscriptionType ptp.MessageType
	serverConfig     *Config

	interval   time.Duration
	expire     time.Time
	sequenceID uint16
	running    bool
}

// NewSubscriptionClient gets minimal required arguments to create a subscription
func NewSubscriptionClient(w *sendWorker, ip net.IP, st ptp.MessageType, sc *Config, i time.Duration, e time.Time) *SubscriptionClient {
	return &SubscriptionClient{
		clientIP:         ip,
		subscriptionType: st,
		interval:         i,
		expire:           e,
		worker:           w,
		serverConfig:     sc,
	}
}

// Start launches the subscription timers and exit on expire
func (sc *SubscriptionClient) Start() {
	sc.setRunning(true)
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
	log.Infof("Starting a new %s subscription for %s", sc.subscriptionType, sc.clientIP)
	log.Debugf("Starting a new %s subscription for %s with load %d with interval %s which expires in %s", sc.subscriptionType, sc.clientIP, l, sc.interval, sc.expire)
	atomic.AddInt64(&sc.worker.load, l)
	defer atomic.AddInt64(&sc.worker.load, -l)

	// Send first message right away
	sc.worker.queue <- sc

	for {
		log.Debugf("Subscription %s for %s is valid until %s", sc.subscriptionType, sc.clientIP, sc.expire)
		remaining := time.Until(sc.expire)
		if !sc.Running() {
			remaining = 0
		}

		select {
		case <-time.After(sc.interval):
			// Add myself to the worker queue
			sc.worker.queue <- sc
		case <-time.After(remaining):
			// When subscription is over
			log.Infof("Subscription %s is over for %s", sc.subscriptionType, sc.clientIP)
			// TODO send cancellation
			return
		}
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

// syncPacket generates ptp Sync packet
func (sc *SubscriptionClient) syncPacket() *ptp.SyncDelayReq {
	return &ptp.SyncDelayReq{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageSync, 0),
			Version:         ptp.Version,
			MessageLength:   uint16(binary.Size(ptp.SyncDelayReq{})),
			DomainNumber:    0,
			FlagField:       ptp.FlagUnicast | ptp.FlagTwoStep,
			SequenceID:      sc.sequenceID,
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber:    1,
				ClockIdentity: sc.serverConfig.clockIdentity,
			},
			LogMessageInterval: 0x7f,
			ControlField:       0,
		},
	}
}

// followupPacket generates ptp Follow Up packet
func (sc *SubscriptionClient) followupPacket(hwts time.Time) *ptp.FollowUp {
	i, err := ptp.NewLogInterval(sc.interval)
	if err != nil {
		log.Errorf("Failed to get interval: %v", err)
	}
	return &ptp.FollowUp{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageFollowUp, 0),
			Version:         ptp.Version,
			MessageLength:   uint16(binary.Size(ptp.FollowUp{})),
			DomainNumber:    0,
			FlagField:       ptp.FlagUnicast,
			SequenceID:      sc.sequenceID,
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber:    1,
				ClockIdentity: sc.serverConfig.clockIdentity,
			},
			LogMessageInterval: i,
			ControlField:       2,
		},
		FollowUpBody: ptp.FollowUpBody{
			PreciseOriginTimestamp: ptp.NewTimestamp(hwts),
		},
	}
}

// announcePacket generates ptp Announce packet
func (sc *SubscriptionClient) announcePacket() *ptp.Announce {
	i, err := ptp.NewLogInterval(sc.interval)
	if err != nil {
		log.Errorf("Failed to get interval: %v", err)
	}
	return &ptp.Announce{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageAnnounce, 0),
			Version:         ptp.Version,
			MessageLength:   uint16(binary.Size(ptp.Announce{})),
			DomainNumber:    0,
			FlagField:       ptp.FlagUnicast | ptp.FlagPTPTimescale,
			SequenceID:      sc.sequenceID,
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber:    1,
				ClockIdentity: sc.serverConfig.clockIdentity,
			},
			LogMessageInterval: i,
			ControlField:       5,
		},
		AnnounceBody: ptp.AnnounceBody{
			CurrentUTCOffset:     int16(sc.serverConfig.UTCOffset.Seconds()),
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
