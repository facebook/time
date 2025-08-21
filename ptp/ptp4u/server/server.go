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
Package server implements simple Unicast PTP UDP server.
*/
package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"time"
	"unsafe"

	"github.com/cespare/xxhash"
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

// fixed subscription duration for sptp clients
const subscriptionDuration = time.Minute * 5

// Start the workers send bind to event and general UDP ports
func (s *Server) Start() error {
	if err := s.Config.CreatePidFile(); err != nil {
		return err
	}

	// Set clock identity
	iface, err := net.InterfaceByName(s.Config.Interface)
	if err != nil {
		return fmt.Errorf("unable to get mac address of the interface: %w", err)
	}
	s.Config.clockIdentity, err = ptp.NewClockIdentity(iface.HardwareAddr)
	if err != nil {
		return fmt.Errorf("unable to get the Clock Identity (EUI-64 address) of the interface: %w", err)
	}

	// initialize the context for the subscriptions
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// Done channel signals the graceful shutdown
	done := make(chan bool)
	// Fail channel signals the failure and shutdown
	fail := make(chan bool)

	// start X workers
	s.sw = make([]*sendWorker, s.Config.SendWorkers)
	for i := 0; i < s.Config.SendWorkers; i++ {
		// Each worker to monitor own queue
		s.sw[i] = newSendWorker(i, s.Config, s.Stats)
		go func(i int) {
			s.sw[i].Start()
			fail <- true
		}(i)
	}

	go func() {
		s.startGeneralListener()
		fail <- true
	}()
	go func() {
		s.startEventListener()
		fail <- true
	}()

	// Drain check
	go func() {
		for ; true; <-time.After(s.Config.DrainInterval) {
			var shouldDrain bool
			for _, check := range s.Checks {
				if check.Check() {
					shouldDrain = true
					log.Warningf("%T engaged", check)
					break
				}
			}

			if drain.Undrain(s.Config.UndrainFileName) {
				log.Warningf("Force undrain file %s is planted, undraining!", s.Config.UndrainFileName)
				shouldDrain = false
			}

			if shouldDrain {
				log.Warning("shifting traffic")
				s.Drain()
				s.Stats.SetDrain(1)
			} else {
				s.Undrain()
				s.Stats.SetDrain(0)
			}
		}
		fail <- true
	}()

	// Watch for SIGHUP and reload dynamic config
	go func() {
		s.handleSighup()
		fail <- true
	}()

	// Watch for SIGTERM and remove pid file
	go func() {
		s.handleSigterm()
		done <- true
	}()

	// Run active metric reporting
	go func() {
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
		fail <- true
	}()

	// Wait for ANY goroutine to finish
	select {
	case <-done:
		return nil
	case <-fail:
		return fmt.Errorf("one of server routines finished")
	}
}

// startEventListener launches the listener which listens to subscription requests
func (s *Server) startEventListener() {
	var err error

	iface, err := net.InterfaceByName(s.Config.Interface)
	if err != nil {
		log.Fatalf("failed to get interface: %v", err)
	}

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
	if err := timestamp.EnableTimestamps(s.Config.TimestampType, s.eFd, iface); err != nil {
		log.Fatal(err)
	}

	err = unix.SetNonblock(s.eFd, false)
	if err != nil {
		log.Fatalf("Failed to set socket to blocking: %s", err)
	}

	fail := make(chan bool)
	for i := 0; i < s.Config.RecvWorkers; i++ {
		go func() {
			s.handleEventMessages(eventConn)
			fail <- true
		}()
	}
	<-fail
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

	fail := make(chan bool)
	for i := 0; i < s.Config.RecvWorkers; i++ {
		go func() {
			s.handleGeneralMessages(generalConn)
			fail <- true
		}()
	}
	<-fail
}

func readPacketBuf(connFd int, buf []byte) (int, unix.Sockaddr, error) {
	n, saddr, err := unix.Recvfrom(connFd, buf, 0)
	if err != nil {
		return 0, nil, err
	}

	return n, saddr, err
}

func updateSockaddrWithPort(sa unix.Sockaddr, port int) {
	switch sa := sa.(type) {
	case *unix.SockaddrInet4:
		sa.Port = port
	case *unix.SockaddrInet6:
		sa.Port = port
	}
}

// handleEventMessage is a handler which gets called every time Event Message arrives
func (s *Server) handleEventMessages(eventConn *net.UDPConn) {
	buf := make([]byte, timestamp.PayloadSizeBytes)
	oob := make([]byte, timestamp.ControlSizeBytes)
	dReq := &ptp.SyncDelayReq{}
	zerotlv := []ptp.TLV{}
	var msgType ptp.MessageType
	var worker *sendWorker
	var sc *SubscriptionClient
	var gclisa unix.Sockaddr
	var expire time.Time
	var workerOffset int64

	for {
		bbuf, eclisa, rxTS, err := timestamp.ReadPacketWithRXTimestampBuf(s.eFd, buf, oob)
		if err != nil {
			log.Errorf("Failed to read packet on %s: %v", eventConn.LocalAddr(), err)
			continue
		}
		if s.Config.TimestampType != timestamp.HW {
			rxTS = rxTS.Add(s.Config.UTCOffset)
		}

		msgType, err = ptp.ProbeMsgType(buf[:bbuf])
		if err != nil {
			log.Errorf("Failed to probe the ptp message type: %v", err)
			continue
		}

		s.Stats.IncRX(msgType)

		// Don't respond on event (delay) requests while being drained
		if s.ctx.Err() != nil {
			continue
		}

		switch msgType {
		case ptp.MessageDelayReq:
			dReq.TLVs = zerotlv
			workerOffset = 0
			if err := ptp.FromBytes(buf[:bbuf], dReq); err != nil {
				log.Errorf("Failed to read the ptp SyncDelayReq: %v", err)
				continue
			}
			log.Debug("Got delay request")
			// CSPTP AlternateResponsePortTLV POC
			for _, tlv := range dReq.TLVs {
				switch v := tlv.(type) {
				case *ptp.AlternateResponsePortTLV:
					workerOffset = int64(v.Offset)
				}
			}

			worker = s.findWorker(dReq.Header.SourcePortIdentity, workerOffset)
			if dReq.FlagField == ptp.FlagProfileSpecific1|ptp.FlagUnicast {
				expire = time.Now().Add(subscriptionDuration)
				// SYNC DELAY_REQUEST and ANNOUNCE
				if sc = worker.FindSubscription(dReq.Header.SourcePortIdentity, ptp.MessageDelayReq); sc == nil {
					// if the port number is > 10, it's a ptping request which expects announce to come to the same ephemeral port
					if dReq.SourcePortIdentity.PortNumber > 10 {
						gclisa = eclisa
					} else {
						gclisa = timestamp.NewSockaddrWithPort(eclisa, ptp.PortGeneral)
					}
					// Create a new subscription
					sc = NewSubscriptionClient(worker.queue, worker.signalingQueue, eclisa, gclisa, ptp.MessageDelayReq, s.Config, subscriptionDuration, expire)
					worker.RegisterSubscription(dReq.Header.SourcePortIdentity, ptp.MessageDelayReq, sc)
					go sc.Start(s.ctx)
				} else {
					// bump the subscription
					sc.SetExpire(expire)
					// sptp is stateless, port can change
					sc.eclisa = eclisa
					// if the port number is > 10, it's a ptping request which expects announce to come to the same ephemeral port
					if dReq.SourcePortIdentity.PortNumber > 10 {
						sc.gclisa = eclisa
					} else {
						updateSockaddrWithPort(sc.gclisa, ptp.PortGeneral)
					}
				}
				sc.UpdateSyncDelayReq(rxTS, dReq.SequenceID)
				sc.UpdateAnnounceDelayReq(dReq.CorrectionField, dReq.SequenceID)
			} else {
				// DELAY_RESPONSE
				if sc = worker.FindSubscription(dReq.Header.SourcePortIdentity, ptp.MessageDelayResp); sc == nil {
					log.Infof("Delay request from %s is not in the subscription list", timestamp.SockaddrToIP(eclisa))
					continue
				}
				sc.UpdateDelayResp(&dReq.Header, rxTS)
			}
			sc.Once()
		default:
			log.Warningf("Got unsupported message type %s(%d)", msgType, msgType)
		}
	}
}

// handleGeneralMessage is a handler which gets called every time General Message arrives
func (s *Server) handleGeneralMessages(generalConn *net.UDPConn) {
	buf := make([]byte, timestamp.PayloadSizeBytes)
	signaling := &ptp.Signaling{}
	zerotlv := []ptp.TLV{}

	var signalingType ptp.MessageType
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
					signalingType = v.MsgTypeAndReserved.MsgType()
					s.Stats.IncRXSignalingGrant(signalingType)
					log.Debugf("Got %s grant request", signalingType)
					durationt = time.Duration(v.DurationField) * time.Second
					expire = time.Now().Add(durationt)
					intervalt = v.LogInterMessagePeriod.Duration()

					switch signalingType {
					case ptp.MessageAnnounce, ptp.MessageSync, ptp.MessageDelayResp:
						worker = s.findWorker(signaling.SourcePortIdentity, 0)
						sc = worker.FindSubscription(signaling.SourcePortIdentity, signalingType)
						if sc == nil || !sc.Running() {
							ip := timestamp.SockaddrToIP(gclisa)
							eclisa := timestamp.IPToSockaddr(ip, ptp.PortEvent)
							sc = NewSubscriptionClient(worker.queue, worker.signalingQueue, eclisa, gclisa, signalingType, s.Config, intervalt, expire)
							worker.RegisterSubscription(signaling.SourcePortIdentity, signalingType, sc)
						} else {
							// Update existing subscription data
							sc.SetExpire(expire)
							sc.SetInterval(intervalt)
							// Update gclisa in case of renewal. This is against the standard,
							// but we want to be able to respond to DelayResps coming from ephemeral ports
							sc.SetGclisa(gclisa)
						}

						// Reject queries out of limit
						if intervalt < s.Config.MinSubInterval || durationt > s.Config.MaxSubDuration || s.ctx.Err() != nil {
							sc.sendSignalingGrant(signaling, v.MsgTypeAndReserved, v.LogInterMessagePeriod, 0)
							continue
						}

						// Send confirmation grant
						sc.sendSignalingGrant(signaling, v.MsgTypeAndReserved, v.LogInterMessagePeriod, v.DurationField)

						if !sc.Running() {
							go sc.Start(s.ctx)
						}
					default:
						log.Errorf("Got unsupported grant type %s", signalingType)
					}
				case *ptp.CancelUnicastTransmissionTLV:
					signalingType = v.MsgTypeAndFlags.MsgType()
					s.Stats.IncRXSignalingCancel(signalingType)
					log.Debugf("Got %s cancel request", signalingType)
					worker = s.findWorker(signaling.SourcePortIdentity, 0)
					sc = worker.FindSubscription(signaling.SourcePortIdentity, signalingType)
					if sc != nil {
						sc.Stop()
					}
				case *ptp.AcknowledgeCancelUnicastTransmissionTLV:
					log.Debugf("Got %s acknowledge cancel request", signalingType)
				default:
					log.Errorf("Got unsupported message type %s(%d)", msgType, msgType)
				}
			}
		}
	}
}

func (s *Server) findWorker(clientID ptp.PortIdentity, offset int64) *sendWorker {
	val := int64(clientID.ClockIdentity) + int64(clientID.PortNumber) + offset //#nosec G115
	hashableBytes := unsafe.Slice((*byte)(unsafe.Pointer(&val)), 8)
	hash := xxhash.Sum64(hashableBytes)
	return s.sw[hash%uint64(s.Config.SendWorkers)] //#nosec G115
}

// Drain traffic
func (s *Server) Drain() {
	if s.ctx != nil && s.ctx.Err() == nil {
		s.cancel()
	}

	// Wait for drain to complete for up to 10 seconds
	for i := 0; i < 10; i++ {
		// Verifying all subscriptions are over
		for _, w := range s.sw {
			w.inventoryClients()
			for _, subs := range w.clients {
				if len(subs) != 0 {
					log.Warningf("Still waiting for %d subscriptions on worker %d to finish...", len(subs), w.id)
					time.Sleep(time.Second)
					break
				}
			}
		}
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
	log.Info("Engaging the SIGHUP monitoring")
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

		s.Stats.IncReload()
	}
}

// handleSigterm watches for SIGTERM and SIGINT and removes the pid file
func (s *Server) handleSigterm() {
	log.Info("Engaging the SIGTERM/SIGINT monitoring")
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, unix.SIGTERM, unix.SIGINT)
	<-sigchan
	log.Warning("Shutting down ptp4u")

	log.Info("Initiating drain")
	s.Drain()

	log.Info("Removing pid")
	if err := s.Config.DeletePidFile(); err != nil {
		log.Fatalf("Failed to remove pid file: %v", err)
	}
}
