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
	"fmt"
	"net"
	"sync"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/ptp4u/stats"
	"github.com/facebook/time/timestamp"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

func enableDSCP(fd int, localAddr net.IP, dscp int) error {
	if localAddr.To4() == nil {
		if err := unix.SetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_TCLASS, dscp<<2); err != nil {
			return err
		}
	} else {
		if err := unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_TOS, dscp<<2); err != nil {
			return err
		}
	}
	return nil
}

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

func (s *sendWorker) listen() (eventFD, generalFD int, err error) {
	// socket domain differs depending whether we are listening on ipv4 or ipv6
	domain := unix.AF_INET6
	if s.config.IP.To4() != nil {
		domain = unix.AF_INET
	}
	// set up event connection
	eventFD, err = unix.Socket(domain, unix.SOCK_DGRAM, unix.IPPROTO_UDP)
	if err != nil {
		return -1, -1, fmt.Errorf("creating event socket error: %w", err)
	}
	sockAddrAnyPort := timestamp.IPToSockaddr(s.config.IP, 0)

	// set SO_REUSEPORT so we can potentially trace network path from same source port.
	// needs to be set before we bind to a port.
	if err = unix.SetsockoptInt(eventFD, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); err != nil {
		return -1, -1, fmt.Errorf("failed to set SO_REUSEPORT on event socket: %w", err)
	}
	// bind to any ephemeral port
	if err := unix.Bind(eventFD, sockAddrAnyPort); err != nil {
		return -1, -1, fmt.Errorf("unable to bind event socket connection: %w", err)
	}

	// get local port we'll send packets from
	localSockAddr, err := unix.Getsockname(eventFD)
	if err != nil {
		return -1, -1, fmt.Errorf("unable to find local ip: %w", err)
	}
	switch v := localSockAddr.(type) {
	case *unix.SockaddrInet4:
		log.Infof("Started worker#%d event on [%v]:%d", s.id, net.IP(v.Addr[:]), v.Port)
	case *unix.SockaddrInet6:
		log.Infof("Started worker#%d event on [%v]:%d", s.id, net.IP(v.Addr[:]), v.Port)
	default:
		log.Errorf("Unexpected local addr type %T", v)
	}

	if err = enableDSCP(eventFD, s.config.IP, s.config.DSCP); err != nil {
		return -1, -1, fmt.Errorf("setting DSCP on event socket: %w", err)
	}

	// Syncs sent from event port, so need to turn on timestamping here
	switch s.config.TimestampType {
	case timestamp.HWTIMESTAMP:
		if err := timestamp.EnableHWTimestamps(eventFD, s.config.Interface); err != nil {
			return -1, -1, fmt.Errorf("failed to enable RX hardware timestamps: %w", err)
		}
	case timestamp.SWTIMESTAMP:
		if err := timestamp.EnableSWTimestamps(eventFD); err != nil {
			return -1, -1, fmt.Errorf("unable to enable RX software timestamps: %w", err)
		}
	default:
		return -1, -1, fmt.Errorf("unrecognized timestamp type: %s", s.config.TimestampType)
	}

	// set up general connection
	generalFD, err = unix.Socket(domain, unix.SOCK_DGRAM, unix.IPPROTO_UDP)
	if err != nil {
		return -1, -1, fmt.Errorf("creating general socket error: %w", err)
	}
	// set SO_REUSEPORT so we can potentially trace network path from same source port.
	// needs to be set before we bind to a port.
	if err = unix.SetsockoptInt(generalFD, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); err != nil {
		return -1, -1, fmt.Errorf("failed to set SO_REUSEPORT on general socket: %w", err)
	}
	// bind to any ephemeral port
	if err := unix.Bind(generalFD, sockAddrAnyPort); err != nil {
		return -1, -1, fmt.Errorf("binding event socket connection: %w", err)
	}
	// enable DSCP
	if err = enableDSCP(generalFD, s.config.IP, s.config.DSCP); err != nil {
		return -1, -1, fmt.Errorf("setting DSCP on general socket: %w", err)
	}
	return
}

// Start a SendWorker which will pull data from the queue and send Sync and Followup packets
func (s *sendWorker) Start() {
	eFd, gFd, err := s.listen()
	if err != nil {
		log.Fatal(err)
	}
	defer unix.Close(eFd)
	defer unix.Close(gFd)

	// reusable buffers
	buf := make([]byte, timestamp.PayloadSizeBytes)
	oob := make([]byte, timestamp.ControlSizeBytes)

	// TMP buffers
	toob := make([]byte, timestamp.ControlSizeBytes)

	var (
		n        int
		attempts int
		txTS     time.Time
		c        *SubscriptionClient
	)

	for c = range s.queue {
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

			txTS, attempts, err = timestamp.ReadTXtimestampBuf(eFd, oob, toob)
			s.stats.SetMaxTXTSAttempts(s.id, int64(attempts))
			if err != nil {
				log.Warningf("Failed to read TX timestamp: %v", err)
				continue
			}
			if s.config.TimestampType != timestamp.HWTIMESTAMP {
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
				continue
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
