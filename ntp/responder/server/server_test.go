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
	"testing"
	"time"

	ntp "github.com/facebook/time/ntp/protocol"
	"github.com/facebook/time/ntp/responder/checker"
	"github.com/facebook/time/ntp/responder/stats"
	"github.com/facebook/time/timestamp"
	"github.com/stretchr/testify/require"
)

var ts = time.Unix(1585231321, 148166539)

// Packet request. From ntpdate run
var ntpRequest = &ntp.Packet{
	Settings:       227,
	Poll:           3,
	Precision:      -6,
	RootDelay:      65536,
	RootDispersion: 65536,
	TxTimeSec:      3794210679,
	TxTimeFrac:     2718216404,
}

func TestFillStaticHeadersStratum(t *testing.T) {
	stratum := 1
	s := &Server{Stratum: stratum}
	response := &ntp.Packet{}
	s.fillStaticHeaders(response)
	require.Equal(t, uint8(stratum), response.Stratum)
}

func TestFillStaticHeadersReferenceID(t *testing.T) {
	s := &Server{RefID: "CHANDLER"}
	response := &ntp.Packet{}

	s.fillStaticHeaders(response)
	require.Equal(t, binary.BigEndian.Uint32([]byte("CHAN")), response.ReferenceID, "Reference-ID must be 4 bytes")
}

func TestFillStaticHeadersRootDelay(t *testing.T) {
	s := &Server{}
	response := &ntp.Packet{}

	s.fillStaticHeaders(response)
	require.Equal(t, uint32(0), response.RootDelay, "Root delay should be 0 if stratum is 1")
}

func TestFillStaticHeadersRootDispersion(t *testing.T) {
	s := &Server{}
	response := &ntp.Packet{}

	s.fillStaticHeaders(response)
	require.Equal(t, uint32(10), response.RootDispersion, "Root dispersion should be 0.000152")
}

func TestGenerateResponsePoll(t *testing.T) {
	request := &ntp.Packet{Poll: 8}
	response := &ntp.Packet{}
	generateResponse(ts, ts, request, response)
	require.Equal(t, request.Poll, response.Poll)
}

func TestGenerateResponsetss(t *testing.T) {
	request := &ntp.Packet{TxTimeSec: 3794210679, TxTimeFrac: 2718216404}
	response := &ntp.Packet{}
	nowSec, nowFrac := ntp.Time(ts)

	generateResponse(ts, ts, request, response)

	// Reference ts must to the closest /1000s
	lastSync := time.Unix(ts.Unix()/1000*1000, 0)
	lastSyncSec, lastSyncFrac := ntp.Time(lastSync)
	require.Equal(t, lastSyncSec, response.RefTimeSec)
	require.Equal(t, lastSyncFrac, response.RefTimeFrac)

	// Originate ts must be the same
	require.Equal(t, request.TxTimeSec, response.OrigTimeSec)
	require.Equal(t, request.TxTimeFrac, response.OrigTimeFrac)

	// Receive ts must be current ts
	require.Equal(t, nowSec, response.RxTimeSec)
	require.Equal(t, nowFrac, response.RxTimeFrac)

	// Transmit ts must be current ts
	require.Equal(t, nowSec, response.TxTimeSec)
	require.Equal(t, nowFrac, response.TxTimeFrac)
}

func TestListener(t *testing.T) {
	s := &Server{
		Checker: &checker.SimpleChecker{
			ExpectedListeners: 1,
			ExpectedWorkers:   0,
		},
	}
	go s.startListener(net.ParseIP("127.0.0.1"), 0)
	time.Sleep(100 * time.Millisecond)

	err := s.Checker.Check()
	require.NoError(t, err)
}

func TestWorker(t *testing.T) {
	s := &Server{
		Checker: &checker.SimpleChecker{
			ExpectedListeners: 0,
			ExpectedWorkers:   1,
		},
		Stats: &stats.JSONStats{},
		tasks: make(chan task),
	}

	// listen to incoming udp ntp.
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)
	defer conn.Close()

	// get connection file descriptor
	connFd, err := timestamp.ConnFd(conn)
	require.NoError(t, err)

	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 0)

	go s.startWorker()
	time.Sleep(100 * time.Millisecond)
	err = s.Checker.Check()
	require.NoError(t, err)
	s.tasks <- task{connFd: connFd, addr: sa, received: time.Now(), request: ntpRequest, stats: &stats.JSONStats{}}
}

func Benchmark_generateResponse(b *testing.B) {
	for i := 0; i < b.N; i++ {
		request := &ntp.Packet{}
		response := &ntp.Packet{}
		generateResponse(ts, ts, request, response)
	}
}

func Benchmark_fillStaticHeaders(b *testing.B) {
	s := &Server{}
	for i := 0; i < b.N; i++ {
		response := &ntp.Packet{}
		s.fillStaticHeaders(response)
	}
}
