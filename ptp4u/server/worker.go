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
	"net"
	"sync"
	"time"

	ptp "github.com/facebookincubator/ptp/protocol"
	"github.com/facebookincubator/ptp/ptp4u/stats"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// sendWorker monitors the queue of jobs
type sendWorker struct {
	mux    sync.Mutex
	id     int
	queue  chan *SubscriptionClient
	config *Config
	stats  stats.Stats

	clients map[ptp.MessageType]map[ptp.PortIdentity]*SubscriptionClient
}

func NewSendWorker(i int, c *Config, st stats.Stats) *sendWorker {
	s := &sendWorker{
		id:     i,
		config: c,
		stats:  st,
	}
	s.clients = make(map[ptp.MessageType]map[ptp.PortIdentity]*SubscriptionClient)
	s.queue = make(chan *SubscriptionClient, c.QueueSize)
	return s
}

// Start a SendWorker which will pull data from the queue and send Sync and Followup packets
func (s *sendWorker) Start() {
	econn, err := net.ListenUDP("udp", &net.UDPAddr{IP: s.config.IP, Port: 0})
	if err != nil {
		log.Fatalf("Binding to event socket error: %s", err)
	}
	defer econn.Close()

	// get connection file descriptor
	eFd, err := ptp.ConnFd(econn)
	if err != nil {
		log.Fatalf("Getting event connection FD: %s", err)
	}

	// Syncs sent from event port
	switch s.config.TimestampType {
	case ptp.HWTIMESTAMP:
		if err := ptp.EnableHWTimestampsSocket(eFd, s.config.Interface); err != nil {
			log.Fatalf("Failed to enable RX hardware timestamps: %v", err)
		}
	case ptp.SWTIMESTAMP:
		if err := ptp.EnableSWTimestampsSocket(eFd); err != nil {
			log.Fatalf("Unable to enable RX software timestamps")
		}
	default:
		log.Fatalf("Unrecognized timestamp type: %s", s.config.TimestampType)
	}

	gconn, err := net.ListenUDP("udp", &net.UDPAddr{IP: s.config.IP, Port: 0})
	if err != nil {
		log.Fatalf("Binding to general socket error: %s", err)
	}
	defer gconn.Close()

	// get connection file descriptor
	gFd, err := ptp.ConnFd(gconn)
	if err != nil {
		log.Fatalf("Getting general connection FD: %s", err)
	}

	buf := make([]byte, ptp.PayloadSizeBytes)

	// reusable buffers for ReadTXtimestampBuf
	bbuf := make([]byte, ptp.PayloadSizeBytes)
	oob := make([]byte, ptp.ControlSizeBytes)

	// TMP buffers
	tbuf := make([]byte, ptp.PayloadSizeBytes)
	toob := make([]byte, ptp.ControlSizeBytes)

	// TODO: Enable dscp accordingly
	var (
		n        int
		attempts int
		txTS     time.Time
	)

	for c := range s.queue {
		switch c.subscriptionType {
		case ptp.MessageSync:
			// send sync
			c.UpdateSync()
			n, err = ptp.BytesTo(c.Sync(), buf)
			if err != nil {
				log.Errorf("Failed to generate the sync packet: %v", err)
				continue
			}
			log.Debugf("Sending sync")

			err = unix.Sendto(eFd, buf[:n], 0, c.eclisa)
			if err != nil {
				log.Errorf("Failed to send the sync packet: %v", err)
				continue
			}
			s.stats.IncTX(c.subscriptionType)

			txTS, attempts, err = ptp.ReadTXtimestampBuf(eFd, bbuf, oob, tbuf, toob)
			s.stats.SetMaxTXTSAttempts(s.id, int64(attempts))
			if err != nil {
				log.Warningf("Failed to read TX timestamp: %v", err)
				continue
			}
			if s.config.TimestampType != ptp.HWTIMESTAMP {
				txTS = txTS.Add(s.config.UTCOffset)
			}

			// send followup
			c.UpdateFollowup(txTS)
			n, err = ptp.BytesTo(c.Followup(), buf)
			if err != nil {
				log.Errorf("Failed to generate the followup packet: %v", err)
				continue
			}
			log.Debugf("Sending followup")

			err = unix.Sendto(gFd, buf[:n], 0, c.gclisa)
			if err != nil {
				log.Errorf("Failed to send the followup packet: %v", err)
				continue
			}
			s.stats.IncTX(ptp.MessageFollowUp)
		case ptp.MessageAnnounce:
			// send announce
			c.UpdateAnnounce()
			n, err = ptp.BytesTo(c.Announce(), buf)
			if err != nil {
				log.Errorf("Failed to prepare the announce packet: %v", err)
				continue
			}
			log.Debugf("Sending announce")

			err = unix.Sendto(gFd, buf[:n], 0, c.gclisa)
			if err != nil {
				log.Errorf("Failed to send the announce packet: %v", err)
				continue
			}
			s.stats.IncTX(c.subscriptionType)

		case ptp.MessageDelayResp:
			// send delay response
			n, err = ptp.BytesTo(c.DelayResp(), buf)
			if err != nil {
				log.Errorf("Failed to prepare the delay response packet: %v", err)
				continue
			}
			log.Debugf("Sending delay response")

			err = unix.Sendto(gFd, buf[:n], 0, c.gclisa)
			if err != nil {
				log.Errorf("Failed to send the delay response: %v", err)
				return
			}
			s.stats.IncTX(c.subscriptionType)

		default:
			log.Errorf("Unknown subscription type: %v", c.subscriptionType)
			continue
		}

		c.IncSequenceID()
		s.stats.SetMaxWorkerQueue(s.id, int64(len(s.queue)))
	}
}

// FindSubscription retrieves an existing client
func (s *sendWorker) FindSubscription(clientID ptp.PortIdentity, st ptp.MessageType) *SubscriptionClient {
	s.mux.Lock()
	defer s.mux.Unlock()
	m, ok := s.clients[st]
	if !ok {
		return nil
	}
	sub, ok := m[clientID]
	if !ok {
		return nil
	}
	return sub
}

// RegisterSubscription will overwrite an existing subscription.
// Make sure you call findSubscription before this
func (s *sendWorker) RegisterSubscription(clientID ptp.PortIdentity, st ptp.MessageType, sc *SubscriptionClient) {
	s.mux.Lock()
	defer s.mux.Unlock()
	m, ok := s.clients[st]
	if !ok {
		s.clients[st] = map[ptp.PortIdentity]*SubscriptionClient{}
		m = s.clients[st]
	}
	m[clientID] = sc
}

func (s *sendWorker) inventoryClients() {
	s.mux.Lock()
	defer s.mux.Unlock()
	for st, subs := range s.clients {
		for k, sc := range subs {
			if !sc.Running() {
				delete(subs, k)
				continue
			}
			s.stats.IncSubscription(st)
			s.stats.IncWorkerSubs(s.id)
		}
	}
}
