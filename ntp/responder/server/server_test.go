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
	"testing"
	"time"

	ntp "github.com/facebook/time/ntp/protocol"
	"github.com/stretchr/testify/require"
)

var ts = time.Unix(1585231321, 148166539)

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
