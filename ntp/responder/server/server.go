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
	"net"
	"time"

	ntp "github.com/facebook/time/ntp/protocol"
	"github.com/facebook/time/timestamp"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// task is a data structure with everything needed to work independently on NTP packet.
type task struct {
	connFd   int
	addr     unix.Sockaddr
	received time.Time
	request  *ntp.Packet
	stats    Stats
}

// Server is a type for UDP server which handles connections.
type Server struct {
	ListenConfig ListenConfig
	Workers      int
	Announce     Announce
	Stats        Stats
	Checker      Checker
	tasks        chan task
	ExtraOffset  time.Duration
	RefID        string
	Stratum      int
}

// Start UDP server.
func (s *Server) Start(ctx context.Context, cancelFunc context.CancelFunc) {
	log.Infof("Creating %d goroutine workers", s.Workers)
	s.tasks = make(chan task, s.Workers)
	// Pre-create workers
	for i := 0; i < s.Workers; i++ {
		go s.startWorker()
	}

	log.Infof("Starting %d listener(s)", len(s.ListenConfig.IPs))

	for _, ip := range s.ListenConfig.IPs {
		log.Infof("Starting listener on %s:%d", ip.String(), s.ListenConfig.Port)

		go func(ip net.IP) {
			s.Stats.IncListeners()
			// Need to be sure IP is on interface:
			if err := s.addIPToInterface(ip); err != nil {
				log.Errorf("[server]: %v", err)
			}

			s.startListener(ip, s.ListenConfig.Port)
			s.Stats.DecListeners()
		}(ip)
	}

	// Run checker periodically
	go func() {
		for {
			time.Sleep(time.Minute)
			log.Debug("[Checker] running internal health checks")
			err := s.Checker.Check()
			if err != nil {
				log.Errorf("[Checker] internal error: %v", err)
				cancelFunc()
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			break
		case <-time.After(30 * time.Second):
			if s.ListenConfig.ShouldAnnounce {
				// First run will be 30 seconds delayed
				log.Debug("Requesting VIPs announce")
				err := s.Announce.Advertise(s.ListenConfig.IPs)
				if err != nil {
					log.Errorf("Error during announcement: %v", err)
					s.Stats.ResetAnnounce()
				} else {
					s.Stats.SetAnnounce()
				}
			} else {
				s.Stats.ResetAnnounce()
			}
		}
	}
}

// Stop will stop announcement, delete IPs from interfaces
func (s *Server) Stop() {
	if err := s.Announce.Withdraw(); err != nil {
		log.Errorf("[server] failed to withdraw announce: %v", err)
	}
	s.DeleteAllIPs()
}

func (s *Server) startListener(ip net.IP, port int) {
	s.Checker.IncListeners()
	defer s.Checker.DecListeners()

	// listen to incoming udp ntp.
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: ip, Port: port})
	if err != nil {
		log.Fatalf("listening error: %s", err)
	}
	defer conn.Close()

	// get connection file descriptor
	connFd, err := timestamp.ConnFd(conn)
	if err != nil {
		log.Fatalf("Getting event connection FD: %s", err)
	}

	// Allow reading of kernel timestamps via socket
	if err := timestamp.EnableSWTimestampsRx(connFd); err != nil {
		log.Fatalf("enabling timestamp error: %s", err)
	}

	buf := make([]byte, timestamp.PayloadSizeBytes)
	oob := make([]byte, timestamp.ControlSizeBytes)
	request := new(ntp.Packet)

	err = unix.SetNonblock(connFd, false)
	if err != nil {
		log.Fatalf("Failed to set socket to blocking: %s", err)
	}

	for {
		// read kernel timestamp from incoming packet
		bbuf, clisa, rxTS, err := timestamp.ReadPacketWithRXTimestampBuf(connFd, buf, oob)
		if err != nil {
			log.Errorf("Failed to read packet on %s: %v", conn.LocalAddr(), err)
			s.Stats.IncReadError()
			continue
		}

		if err := request.UnmarshalBinary(buf[:bbuf]); err != nil {
			log.Errorf("failed to parse ntp packet: %s", err)
			s.Stats.IncReadError()
			continue
		}
		s.Stats.IncRequests()
		s.tasks <- task{connFd: connFd, addr: clisa, received: rxTS, request: request, stats: s.Stats}
	}
}

func (s *Server) startWorker() {
	s.Checker.IncWorkers()
	defer s.Checker.DecWorkers()
	defer s.Stats.DecWorkers()

	// Pre-allocating response buffer
	response := &ntp.Packet{}
	s.fillStaticHeaders(response)
	s.Stats.IncWorkers()
	for {
		task := <-s.tasks
		task.serve(response, s.ExtraOffset)
	}
}

// serve checks the request format
// gets time from local and respond.
func (t *task) serve(response *ntp.Packet, extraoffset time.Duration) {
	log.Debugf("Received request: %+v", t.request)
	if t.request.ValidSettingsFormat() {
		generateResponse(time.Now().Add(extraoffset), t.received.Add(extraoffset), t.request, response)
		responseBytes, err := response.Bytes()
		if err != nil {
			log.Errorf("Failed to convert ntp.%v to bytes %v: %v", response, responseBytes, err)
			return
		}

		log.Debugf("Writing response: %+v", response)
		if err := unix.Sendto(t.connFd, responseBytes, 0, t.addr); err != nil {
			log.Debugf("Failed to respond to the request: %v", err)
		}
		t.stats.IncResponses()
		return
	}
	log.Debugf("Invalid query, discarding: %v", t.request)
	t.stats.IncInvalidFormat()
}

// fillStaticHeaders pre-sets all the headers per worker which will never change
// numbers are taken from tcpdump.
func (s *Server) fillStaticHeaders(response *ntp.Packet) {
	response.Stratum = uint8(s.Stratum)
	response.Precision = -32
	// Root delay. We pretend to be stratum 1
	response.RootDelay = 0
	// Root dispersion, big-endian 0.000152
	response.RootDispersion = 10
	// Reference ID ATOM. Only for stratum 1
	response.ReferenceID = binary.BigEndian.Uint32([]byte(fmt.Sprintf("%-4s", s.RefID)))
}

// generateResponse generates response NTP packet
// See more in protocol/ntp/packet.go.
func generateResponse(now time.Time, received time.Time, request, response *ntp.Packet) {
	var vn = request.Settings & 0x38
	response.Settings = vn + 4

	// Poll
	response.Poll = request.Poll

	// Reference Timestamp
	// RFC: "Local time at which the local clock was last set or corrected."
	// Because we don't have this info (no access to chronyd/ntpd) we need to
	// come up with something. Just returning "now" will not fly and chronyd/ntpd
	// will exclude "inconsistent host". So once per 1000s sounds "consistent" enough
	lastSync := time.Unix(now.Unix()/1000*1000, 0)
	lastSyncSec, lastSyncFrac := ntp.Time(lastSync)
	response.RefTimeSec = lastSyncSec
	response.RefTimeFrac = lastSyncFrac

	// Originate Timestamp
	// RFC: "Local time at which the request departed the client host for the service host."
	response.OrigTimeSec = request.TxTimeSec
	response.OrigTimeFrac = request.TxTimeFrac

	// Receive Timestamp
	// RFC: "Local time at which the request arrived at the service host."
	receivedSec, receivedFrac := ntp.Time(received)
	response.RxTimeSec = receivedSec
	response.RxTimeFrac = receivedFrac

	// Transmit Timestamp
	// RFC: "Local time at which the reply departed the service host for the client host."
	nowSec, nowFrac := ntp.Time(now)
	response.TxTimeSec = nowSec
	response.TxTimeFrac = nowFrac
}
