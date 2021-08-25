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
	"fmt"
	"hash"
	"hash/fnv"
	"net"
	"sync"
	"time"

	ptp "github.com/facebookincubator/ptp/protocol"
	"github.com/facebookincubator/ptp/ptp4u/stats"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// Server is PTP unicast server
type Server struct {
	Config *Config
	Stats  stats.Stats
	sw     []*sendWorker

	// server source fds
	eFd int
	gFd int
}

// Start the workers send bind to event and general UDP ports
func (s *Server) Start() error {
	// Set clock identity
	iface, err := net.InterfaceByName(s.Config.Interface)
	if err != nil {
		return fmt.Errorf("unable to get mac address of the interface: %v", err)
	}
	s.Config.clockIdentity, err = ptp.NewClockIdentity(iface.HardwareAddr)
	if err != nil {
		return fmt.Errorf("unable to get the Clock Identity (EUI-64 address) of the interface: %v", err)
	}

	// Call wg.Add(1) ONLY once
	// If ANY goroutine finishes no matter how many of them we run
	// wg.Done will unblock
	var wg sync.WaitGroup
	wg.Add(1)

	// start X workers
	s.sw = make([]*sendWorker, s.Config.Workers)
	for i := 0; i < s.Config.Workers; i++ {
		// Each worker to monitor own queue
		s.sw[i] = NewSendWorker(i, s.Config, s.Stats)
		go func(i int) {
			defer wg.Done()
			s.sw[i].Start()
		}(i)
	}

	go func() {
		defer wg.Done()
		s.startGeneralListener()
	}()
	go func() {
		defer wg.Done()
		s.startEventListener()
	}()

	// Run active metric reporting
	go func() {
		defer wg.Done()
		for {
			<-time.After(s.Config.MetricInterval)
			for _, w := range s.sw {
				w.inventoryClients()
			}
			s.Stats.SetUTCOffset(int64(s.Config.UTCOffset.Seconds()))

			s.Stats.Snapshot()
			s.Stats.Reset()
		}
	}()

	// Update UTC offset periodically
	go func() {
		defer wg.Done()
		for {
			<-time.After(1 * time.Minute)
			if s.Config.SHM {
				if err := s.Config.SetUTCOffsetFromSHM(); err != nil {
					log.Errorf("Failed to update UTC offset: %v. Keeping the last known: %s", err, s.Config.UTCOffset)
				}
			}
			log.Debugf("UTC offset is: %v", s.Config.UTCOffset.Seconds())
		}
	}()

	// Wait for ANY gorouine to finish
	wg.Wait()
	return fmt.Errorf("one of server routines finished")
}

// startEventListener launches the listener which listens to subscription requests
func (s *Server) startEventListener() {
	var err error
	log.Infof("Binding on %s %d", s.Config.IP, ptp.PortEvent)
	eventConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: s.Config.IP, Port: ptp.PortEvent})
	if err != nil {
		log.Fatalf("Listening error: %s", err)
	}
	defer eventConn.Close()

	// get connection file descriptor
	s.eFd, err = ptp.ConnFd(eventConn)
	if err != nil {
		log.Fatalf("Getting event connection FD: %s", err)
	}

	// Enable RX timestamps. Delay requests need to be timestamped by ptp4u on receipt
	switch s.Config.TimestampType {
	case ptp.HWTIMESTAMP:
		if err := ptp.EnableHWTimestampsSocket(s.eFd, s.Config.Interface); err != nil {
			log.Fatalf("Cannot enable hardware RX timestamps")
		}
	case ptp.SWTIMESTAMP:
		if err := ptp.EnableSWTimestampsSocket(s.eFd); err != nil {
			log.Fatalf("Cannot enable software RX timestamps")
		}
	default:
		log.Fatalf("Unrecognized timestamp type: %s", s.Config.TimestampType)
	}

	err = unix.SetNonblock(s.eFd, false)
	if err != nil {
		log.Fatalf("Failed to set socket to blocking: %s", err)
	}

	// Call wg.Add(1) ONLY once
	// If ANY goroutine finishes no matter how many of them we run
	// wg.Done will unblock
	var wg sync.WaitGroup
	wg.Add(1)

	for i := 0; i < s.Config.Workers; i++ {
		go func() {
			defer wg.Done()
			buf := make([]byte, ptp.PayloadSizeBytes)
			oob := make([]byte, ptp.ControlSizeBytes)
			h := fnv.New32a()

			for {
				// reset hash
				h.Reset()
				bbuf, clisa, rxTS, err := ptp.ReadPacketWithRXTimestampBuf(s.eFd, buf, oob)
				if err != nil {
					log.Errorf("Failed to read packet on %s: %v", eventConn.LocalAddr(), err)
					continue
				}
				if s.Config.TimestampType != ptp.HWTIMESTAMP {
					rxTS = rxTS.Add(s.Config.UTCOffset)
				}
				s.handleEventMessage(buf[:bbuf], clisa, rxTS, h)
			}
		}()
	}
	wg.Wait()
}

// startGeneralListener launches the listener which listens to announces
func (s *Server) startGeneralListener() {
	var err error
	log.Infof("Binding on %s %d", s.Config.IP, ptp.PortGeneral)
	generalConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: s.Config.IP, Port: ptp.PortGeneral})
	if err != nil {
		log.Fatalf("Listening error: %s", err)
	}
	defer generalConn.Close()

	// get connection file descriptor
	s.gFd, err = ptp.ConnFd(generalConn)
	if err != nil {
		log.Fatalf("Getting general connection FD: %s", err)
	}

	err = unix.SetNonblock(s.gFd, false)
	if err != nil {
		log.Fatalf("Failed to set socket to blocking: %s", err)
	}

	// Call wg.Add(1) ONLY once
	// If ANY goroutine finishes no matter how many of them we run
	// wg.Done will unblock
	var wg sync.WaitGroup
	wg.Add(1)

	for i := 0; i < s.Config.Workers; i++ {
		go func() {
			defer wg.Done()
			buf := make([]byte, ptp.PayloadSizeBytes)
			h := fnv.New32a()

			for {
				// reset hash
				h.Reset()
				bbuf, clisa, err := readPacketBuf(s.gFd, buf)
				if err != nil {
					log.Errorf("Failed to read packet on %s: %v", generalConn.LocalAddr(), err)
					continue
				}
				s.handleGeneralMessage(buf[:bbuf], clisa, h)
			}
		}()
	}
	wg.Wait()
}

func readPacketBuf(connFd int, buf []byte) (int, unix.Sockaddr, error) {
	n, saddr, err := unix.Recvfrom(connFd, buf, 0)
	if err != nil {
		return 0, nil, err
	}

	return n, saddr, err
}

// handleEventMessage is a handler which gets called every time Event Message arrives
func (s *Server) handleEventMessage(request []byte, clisa unix.Sockaddr, rxTS time.Time, h hash.Hash32) {
	msgType, err := ptp.ProbeMsgType(request)
	if err != nil {
		log.Errorf("Failed to probe the ptp message type: %v", err)
		return
	}

	s.Stats.IncRX(msgType)

	switch msgType {
	case ptp.MessageDelayReq:
		dReq := &ptp.SyncDelayReq{}
		if err := ptp.FromBytes(request, dReq); err != nil {
			log.Errorf("Failed to read the ptp SyncDelayReq: %v", err)
			return
		}

		log.Debugf("Got delay request")
		worker := s.findWorker(dReq.Header.SourcePortIdentity, h)
		sc := worker.FindSubscription(dReq.Header.SourcePortIdentity, ptp.MessageDelayResp)
		if sc == nil {
			log.Warningf("Delay request from %s is not in the subscription list", ptp.SockaddrToIP(clisa))
			return
		}
		sc.UpdateDelayResp(&dReq.Header, rxTS)
		sc.Once()
	default:
		log.Errorf("Got unsupported message type %s(%d)", msgType, msgType)
	}

}

// handleGeneralMessage is a handler which gets called every time General Message arrives
func (s *Server) handleGeneralMessage(request []byte, gclisa unix.Sockaddr, h hash.Hash32) {
	msgType, err := ptp.ProbeMsgType(request)
	if err != nil {
		log.Errorf("Failed to probe the ptp message type: %v", err)
		return
	}

	switch msgType {
	case ptp.MessageSignaling:
		signaling := &ptp.Signaling{}
		if err := ptp.FromBytes(request, signaling); err != nil {
			log.Error(err)
			return
		}

		for _, tlv := range signaling.TLVs {
			switch v := tlv.(type) {
			case *ptp.RequestUnicastTransmissionTLV:
				grantType := v.MsgTypeAndReserved.MsgType()
				log.Debugf("Got %s grant request", grantType)

				switch grantType {
				case ptp.MessageAnnounce, ptp.MessageSync:
					durationt := time.Duration(v.DurationField) * time.Second
					intervalt := v.LogInterMessagePeriod.Duration()
					expire := time.Now().Add(durationt)

					worker := s.findWorker(signaling.SourcePortIdentity, h)
					sc := worker.FindSubscription(signaling.SourcePortIdentity, grantType)
					if sc == nil {
						ip := ptp.SockaddrToIP(gclisa)
						eclisa := ptp.IPToSockaddr(ip, ptp.PortEvent)
						sc = NewSubscriptionClient(worker.queue, eclisa, gclisa, grantType, s.Config, intervalt, expire)
						worker.RegisterSubscription(signaling.SourcePortIdentity, grantType, sc)
					} else {
						// Update existing subscription data
						sc.expire = expire
						sc.interval = intervalt
					}

					// Reject queries out of limit
					if intervalt < s.Config.MinSubInterval || durationt > s.Config.MaxSubDuration {
						log.Warningf("Got too demanding %s request. Duration: %s, Interval: %s. Rejecting. Consider changing -maxsubduration and -minsubinterval", grantType, durationt, intervalt)
						s.sendGrant(sc, signaling, v.MsgTypeAndReserved, v.LogInterMessagePeriod, 0, gclisa)
						return
					}

					// The subscription is over or a new cli. Starting
					if !sc.running {
						go sc.Start()
					}

					// Send confirmation grant
					s.sendGrant(sc, signaling, v.MsgTypeAndReserved, v.LogInterMessagePeriod, v.DurationField, gclisa)

				case ptp.MessageDelayResp:
					worker := s.findWorker(signaling.SourcePortIdentity, h)
					sc := worker.FindSubscription(signaling.SourcePortIdentity, grantType)
					if sc == nil {
						ip := ptp.SockaddrToIP(gclisa)
						eclisa := ptp.IPToSockaddr(ip, ptp.PortEvent)
						sc = NewSubscriptionClient(worker.queue, eclisa, gclisa, grantType, s.Config, time.Second, time.Time{})
						worker.RegisterSubscription(signaling.SourcePortIdentity, grantType, sc)
					}
					sc.SetRunning(true)

					// Send confirmation grant
					s.sendGrant(sc, signaling, v.MsgTypeAndReserved, 0, v.DurationField, gclisa)

				default:
					log.Errorf("Got unsupported grant type %s", grantType)
				}
				s.Stats.IncRXSignaling(grantType)
			case *ptp.CancelUnicastTransmissionTLV:
				grantType := v.MsgTypeAndFlags.MsgType()
				log.Debugf("Got %s cancel request", grantType)
				worker := s.findWorker(signaling.SourcePortIdentity, h)
				sc := worker.FindSubscription(signaling.SourcePortIdentity, grantType)
				if sc != nil {
					sc.Stop()
				}

			default:
				log.Errorf("Got unsupported message type %s(%d)", msgType, msgType)
			}
		}
	}
}

func (s *Server) findWorker(clientID ptp.PortIdentity, h hash.Hash32) *sendWorker {
	// that's how we do binary.Write(h, binary.BigEndian, clientID) without allocations
	_, _ = h.Write([]byte{
		byte(0xff & clientID.ClockIdentity),
		byte(0xff & (clientID.ClockIdentity >> 8)),
		byte(0xff & (clientID.ClockIdentity >> 16)),
		byte(0xff & (clientID.ClockIdentity >> 24)),
		byte(0xff & (clientID.ClockIdentity >> 32)),
		byte(0xff & (clientID.ClockIdentity >> 40)),
		byte(0xff & (clientID.ClockIdentity >> 48)),
		byte(0xff & (clientID.ClockIdentity >> 56)),
	})
	_, _ = h.Write([]byte{
		byte(0xff & clientID.PortNumber),
		byte(0xff & (clientID.PortNumber >> 8)),
	})
	return s.sw[h.Sum32()%uint32(s.Config.Workers)]
}

// sendGrant sends a Unicast Grant message
func (s *Server) sendGrant(sc *SubscriptionClient, sg *ptp.Signaling, mt ptp.UnicastMsgTypeAndFlags, interval ptp.LogInterval, duration uint32, sa unix.Sockaddr) {
	sc.UpdateGrant(sg, mt, interval, duration)
	grantb, err := ptp.Bytes(sc.Grant())
	if err != nil {
		log.Errorf("Failed to prepare the unicast grant: %v", err)
		return
	}
	err = unix.Sendto(s.gFd, grantb, 0, sa)
	if err != nil {
		log.Errorf("Failed to send the unicast grant: %v", err)
		return
	}
	log.Debugf("Sent unicast grant")
	s.Stats.IncTXSignaling(sc.subscriptionType)
}
