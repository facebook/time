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
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"sync"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/ptp4u/drain"
	"github.com/facebook/time/ptp/ptp4u/stats"
	"github.com/facebook/time/timestamp"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// Server is PTP unicast server
type Server struct {
	Config *Config
	Stats  stats.Stats
	Checks []drain.Drain
	sw     []*sendWorker

	// server source fds
	eFd int
	gFd int

	// drain logic
	cancel context.CancelFunc
	ctx    context.Context
}

// Start the workers send bind to event and general UDP ports
func (s *Server) Start() error {
	if err := s.Config.CreatePidFile(); err != nil {
		return err
	}

	// Set clock identity
	iface, err := net.InterfaceByName(s.Config.Interface)
	if err != nil {
		return fmt.Errorf("unable to get mac address of the interface: %v", err)
	}
	s.Config.clockIdentity, err = ptp.NewClockIdentity(iface.HardwareAddr)
	if err != nil {
		return fmt.Errorf("unable to get the Clock Identity (EUI-64 address) of the interface: %v", err)
	}

	// initialize the context for the subscriptions
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// Call wg.Add(1) ONLY once
	// If ANY goroutine finishes no matter how many of them we run
	// wg.Done will unblock
	var wg sync.WaitGroup
	wg.Add(1)

	// start X workers
	s.sw = make([]*sendWorker, s.Config.SendWorkers)
	for i := 0; i < s.Config.SendWorkers; i++ {
		// Each worker to monitor own queue
		s.sw[i] = newSendWorker(i, s.Config, s.Stats)
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

	// Drain check
	go func() {
		defer wg.Done()
		for ; true; <-time.After(s.Config.DrainInterval) {
			var drain bool
			for _, check := range s.Checks {
				if check.Check() {
					drain = true
					log.Warningf("%T engaged shifting traffic", check)
					break
				}
			}

			if drain {
				s.Drain()
				s.Stats.SetDrain(1)
			} else {
				s.Undrain()
				s.Stats.SetDrain(0)
			}
		}
	}()

	// Watch for SIGHUP and reload dynamic config
	go func() {
		defer wg.Done()
		s.handleSighup()
	}()

	// Watch for SIGTERM and remove pid file
	go func() {
		defer wg.Done()
		s.handleSigterm()
	}()

	// Run active metric reporting
	go func() {
		defer wg.Done()
		for ; true; <-time.After(s.Config.MetricInterval) {
			for _, w := range s.sw {
				w.inventoryClients()
			}
			s.Stats.SetUTCOffsetSec(int64(s.Config.UTCOffset.Seconds()))
			s.Stats.SetClockAccuracy(int64(s.Config.ClockAccuracy))
			s.Stats.SetClockClass(int64(s.Config.ClockClass))

			s.Stats.Snapshot()
			s.Stats.Reset()
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
	s.eFd, err = timestamp.ConnFd(eventConn)
	if err != nil {
		log.Fatalf("Getting event connection FD: %s", err)
	}

	// Enable RX timestamps. Delay requests need to be timestamped by ptp4u on receipt
	switch s.Config.TimestampType {
	case timestamp.HWTIMESTAMP:
		if err := timestamp.EnableHWTimestamps(s.eFd, s.Config.Interface); err != nil {
			log.Fatalf("Cannot enable hardware RX timestamps: %v", err)
		}
	case timestamp.SWTIMESTAMP:
		if err := timestamp.EnableSWTimestamps(s.eFd); err != nil {
			log.Fatalf("Cannot enable software RX timestamps: %v", err)
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

	for i := 0; i < s.Config.RecvWorkers; i++ {
		go func() {
			defer wg.Done()
			s.handleEventMessages(eventConn)
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
	s.gFd, err = timestamp.ConnFd(generalConn)
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

	for i := 0; i < s.Config.RecvWorkers; i++ {
		go func() {
			defer wg.Done()
			s.handleGeneralMessages(generalConn)
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
func (s *Server) handleEventMessages(eventConn *net.UDPConn) {
	buf := make([]byte, timestamp.PayloadSizeBytes)
	oob := make([]byte, timestamp.ControlSizeBytes)
	dReq := &ptp.SyncDelayReq{}
	// Initialize the new random. We will re-seed it every time in findWorker
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	var msgType ptp.MessageType
	var worker *sendWorker
	var sc *SubscriptionClient

	for {
		bbuf, clisa, rxTS, err := timestamp.ReadPacketWithRXTimestampBuf(s.eFd, buf, oob)
		if err != nil {
			log.Errorf("Failed to read packet on %s: %v", eventConn.LocalAddr(), err)
			continue
		}
		if s.Config.TimestampType != timestamp.HWTIMESTAMP {
			rxTS = rxTS.Add(s.Config.UTCOffset)
		}

		msgType, err = ptp.ProbeMsgType(buf[:bbuf])
		if err != nil {
			log.Errorf("Failed to probe the ptp message type: %v", err)
			continue
		}

		s.Stats.IncRX(msgType)

		switch msgType {
		case ptp.MessageDelayReq:
			if err := ptp.FromBytes(buf[:bbuf], dReq); err != nil {
				log.Errorf("Failed to read the ptp SyncDelayReq: %v", err)
				continue
			}

			log.Debugf("Got delay request")
			worker = s.findWorker(dReq.Header.SourcePortIdentity, r)
			sc = worker.FindSubscription(dReq.Header.SourcePortIdentity, ptp.MessageDelayResp)
			if sc == nil {
				log.Infof("Delay request from %s is not in the subscription list", timestamp.SockaddrToIP(clisa))
				continue
			}
			sc.UpdateDelayResp(&dReq.Header, rxTS)
			sc.Once()
		default:
			log.Errorf("Got unsupported message type %s(%d)", msgType, msgType)
		}
	}
}

// handleGeneralMessage is a handler which gets called every time General Message arrives
func (s *Server) handleGeneralMessages(generalConn *net.UDPConn) {
	buf := make([]byte, timestamp.PayloadSizeBytes)
	signaling := &ptp.Signaling{}
	zerotlv := []ptp.TLV{}
	// Initialize the new random. We will re-seed it every time in findWorker
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	var grantType ptp.MessageType
	var durationt time.Duration
	var intervalt time.Duration
	var expire time.Time
	var worker *sendWorker
	var sc *SubscriptionClient

	for {
		bbuf, gclisa, err := readPacketBuf(s.gFd, buf)
		if err != nil {
			log.Errorf("Failed to read packet on %s: %v", generalConn.LocalAddr(), err)
			continue
		}

		msgType, err := ptp.ProbeMsgType(buf[:bbuf])
		if err != nil {
			log.Errorf("Failed to probe the ptp message type: %v", err)
			continue
		}

		switch msgType {
		case ptp.MessageSignaling:
			signaling.TLVs = zerotlv
			if err := ptp.FromBytes(buf[:bbuf], signaling); err != nil {
				log.Error(err)
				continue
			}

			for _, tlv := range signaling.TLVs {
				switch v := tlv.(type) {
				case *ptp.RequestUnicastTransmissionTLV:
					grantType = v.MsgTypeAndReserved.MsgType()
					log.Debugf("Got %s grant request", grantType)
					durationt = time.Duration(v.DurationField) * time.Second
					expire = time.Now().Add(durationt)
					intervalt = v.LogInterMessagePeriod.Duration()

					switch grantType {
					case ptp.MessageAnnounce, ptp.MessageSync, ptp.MessageDelayResp:
						worker = s.findWorker(signaling.SourcePortIdentity, r)
						sc = worker.FindSubscription(signaling.SourcePortIdentity, grantType)
						if sc == nil {
							ip := timestamp.SockaddrToIP(gclisa)
							eclisa := timestamp.IPToSockaddr(ip, ptp.PortEvent)
							sc = NewSubscriptionClient(worker.queue, eclisa, gclisa, grantType, s.Config, intervalt, expire)
							worker.RegisterSubscription(signaling.SourcePortIdentity, grantType, sc)
						} else {
							// Update existing subscription data
							sc.expire = expire
							sc.interval = intervalt
							// Update gclisa in case of renewal. This is against the standard,
							// but we want to be able to respond to DelayResps coming from ephemeral ports
							sc.gclisa = gclisa
						}

						// Reject queries out of limit
						if intervalt < s.Config.MinSubInterval || durationt > s.Config.MaxSubDuration || s.ctx.Err() != nil {
							s.sendGrant(sc, signaling, v.MsgTypeAndReserved, v.LogInterMessagePeriod, 0, gclisa)
							continue
						}

						// Send confirmation grant
						s.sendGrant(sc, signaling, v.MsgTypeAndReserved, v.LogInterMessagePeriod, v.DurationField, gclisa)

						if !sc.Running() {
							go sc.Start(s.ctx)
						}
						// Send 3 announce messages to allow quick restart of ptp4l
						// https://sourceforge.net/p/linuxptp/mailman/message/37685839/
						// https://github.com/richardcochran/linuxptp/blob/ef9ba9489c2f664ea34e5e4dbddbb76cddef5254/foreign.h#L28
						// https://github.com/richardcochran/linuxptp/blob/33ac7d25cd9212e79be6f7023ba18cfa5020e35b/port.c#L2546
						if signaling.Header.SequenceID == 0 && grantType == ptp.MessageAnnounce {
							for i := 0; i < 3; i++ {
								sc.Once()
							}
						}
					default:
						log.Errorf("Got unsupported grant type %s", grantType)
					}
					s.Stats.IncRXSignaling(grantType)
				case *ptp.CancelUnicastTransmissionTLV:
					grantType = v.MsgTypeAndFlags.MsgType()
					log.Debugf("Got %s cancel request", grantType)
					worker = s.findWorker(signaling.SourcePortIdentity, r)
					sc = worker.FindSubscription(signaling.SourcePortIdentity, grantType)
					if sc != nil {
						sc.Stop()
					}
				default:
					log.Errorf("Got unsupported message type %s(%d)", msgType, msgType)
				}
			}
		}
	}
}

func (s *Server) findWorker(clientID ptp.PortIdentity, r *rand.Rand) *sendWorker {
	// Seeding random with the same value will produce the same number
	r.Seed(int64(clientID.ClockIdentity) + int64(clientID.PortNumber))
	return s.sw[r.Intn(s.Config.SendWorkers)]
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

// Drain traffic
func (s *Server) Drain() {
	if s.ctx != nil && s.ctx.Err() == nil {
		s.cancel()
	}
}

// Undrain traffic
func (s *Server) Undrain() {
	if s.ctx != nil && s.ctx.Err() != nil {
		s.ctx, s.cancel = context.WithCancel(context.Background())
	}
}

// handleSighup watches for SIGHUP and reloads the dynamic config
func (s *Server) handleSighup() {
	log.Infof("Engaging the SIGHUP monitoring")
	sigchan := make(chan os.Signal, 10)
	signal.Notify(sigchan, unix.SIGHUP)
	for range sigchan {
		log.Info("SIGHUP received, reloading config")
		dc, err := ReadDynamicConfig(s.Config.ConfigFile)
		if err != nil {
			log.Errorf("Failed to reload config: %v. Moving on", err)
			continue
		}
		dcMux.Lock()
		s.Config.DynamicConfig = *dc
		dcMux.Unlock()

		// Sending announces
		log.Info("Sending announces")
		for _, worker := range s.sw {
			clients := worker.FindClients(ptp.MessageAnnounce)
			for _, sc := range clients {
				sc.Once()
			}
		}
		s.Stats.IncReload()
	}
}

// handleSigterm watches for SIGTERM and SIGINT and removes the pid file
func (s *Server) handleSigterm() {
	log.Infof("Engaging the SIGTERM/SIGINT monitoring")
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, unix.SIGTERM, unix.SIGINT)
	<-sigchan
	log.Warning("Shutting down ptp4u")
	if err := s.Config.DeletePidFile(); err != nil {
		log.Fatalf("Failed to remove pid file: %v", err)
	}
}
