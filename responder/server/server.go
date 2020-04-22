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
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/facebookincubator/ntp/protocol/ntp"
	log "github.com/sirupsen/logrus"
)

type task struct {
	conn     net.PacketConn
	addr     net.Addr
	received time.Time
	request  *ntp.Packet
	stats    Stats
}

// Server is a type for UDP server which handles connections
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

// Start UDP server
func (s *Server) Start(ctx context.Context, cancelFunc context.CancelFunc) {
	log.Warningf("Creating %d goroutine workers", s.Workers)
	s.tasks = make(chan task, s.Workers)
	// Pre-create workers
	for i := 0; i < s.Workers; i++ {
		go s.startWorker()
	}

	log.Warningf("Starting %d listener(s)", len(s.ListenConfig.IPs))

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

	// Run active metric reporting
	go func() {
		for {
			<-time.After(1 * time.Minute)
			err := s.Stats.Report()
			if err != nil {
				log.Errorf("[stats] %v", err)
			}
		}
	}()

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
	s.DeleteAllIPs()
	if err := s.Announce.Withdraw(); err != nil {
		log.Errorf("[server] failed to withdraw announce: %v", err)
	}
}

func (s *Server) startListener(ip net.IP, port int) {
	s.Checker.IncListeners()
	defer s.Checker.DecListeners()

	// listen to incoming udp ntp.
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: ip, Port: port})
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Allow reading of hardware/kernel timestamps via socket
	if err := ntp.EnableKernelTimestampsSocket(conn); err != nil {
		log.Fatalln(err)
	}

	for {
		// read HW/kernel timestamp from incoming packet
		request, nowHWtimestamp, returnaddr, err := ntp.ReadPacketWithKernelTimestamp(conn)
		if err != nil {
			log.Fatalln(err)
			continue
		}
		s.Stats.IncRequests()
		s.tasks <- task{conn: conn, addr: returnaddr, received: nowHWtimestamp, request: request, stats: s.Stats}
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

// serve checks the request format.
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

		log.Debugf("Writing from: %v", t.conn.LocalAddr())
		log.Debugf("Writing response: %+v", response)
		_, err = t.conn.WriteTo(responseBytes, t.addr)
		if err != nil {
			log.Infof("Failed to respond to the request: %v", err)
		}
		t.stats.IncResponses()
		return
	}
	log.Infof("Invalid query, discarding: %v", t.request)
	t.stats.IncInvalidFormat()
}

// fillStaticHeaders pre-sets all the headers per worker which will never change
// numbers are taken from tcpdump
func (s *Server) fillStaticHeaders(response *ntp.Packet) {
	response.Stratum = uint8(s.Stratum)
	response.Precision = -32
	// Root delay. We pretend to be stratum 1
	response.RootDelay = 0
	// Root dispersion, big-endian 0.000152
	response.RootDispersion = 10
	// Refference ID ATOM. Only for stratum 1
	response.ReferenceID = binary.BigEndian.Uint32([]byte(fmt.Sprintf("%-4s", s.RefID)))
}

// generateResponse generates ntp.in a proper format
/*
	http://seriot.ch/ntp.php
	https://tools.ietf.org/html/rfc958
	0                   1                   2                   3
	0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
0 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|LI | VN  |Mode |    Stratum     |     Poll      |  Precision   |
4 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                         Root Delay                            |
8 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                         Root Dispersion                       |
12+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                          Reference ID                         |
16+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                                                               |
	+                     Reference Timestamp (64)                  +
	|                                                               |
24+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                                                               |
	+                      Origin Timestamp (64)                    +
	|                                                               |
32+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                                                               |
	+                      Receive Timestamp (64)                   +
	|                                                               |
40+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                                                               |
	+                      Transmit Timestamp (64)                  +
	|                                                               |
48+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
*/
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
