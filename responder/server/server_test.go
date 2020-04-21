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

	"github.com/facebookincubator/ntp/protocol/ntp"
	"github.com/stretchr/testify/assert"
)

var timestamp = time.Unix(1585231321, 148166539)

func Test_fillStaticHeadersStratum(t *testing.T) {
	stratum := 1
	s := &Server{Stratum: stratum}
	response := &ntp.Packet{}
	s.fillStaticHeaders(response)
	assert.Equal(t, uint8(stratum), response.Stratum)
}

func Test_fillStaticHeadersReferenceID(t *testing.T) {
	s := &Server{RefID: "CHANDLER"}
	response := &ntp.Packet{}

	s.fillStaticHeaders(response)
	assert.Equal(t, binary.BigEndian.Uint32([]byte("CHAN")), response.ReferenceID, "Reference-ID must be 4 bytes")
}

func Test_fillStaticHeadersRootDelay(t *testing.T) {
	s := &Server{}
	response := &ntp.Packet{}

	s.fillStaticHeaders(response)
	assert.Equal(t, uint32(0), response.RootDelay, "Root delay should be 0 if stratum is 1")
}

func Test_fillStaticHeadersRootDispersion(t *testing.T) {
	s := &Server{}
	response := &ntp.Packet{}

	s.fillStaticHeaders(response)
	assert.Equal(t, uint32(10), response.RootDispersion, "Root dispersion should be 0.000152")
}

func Test_generateResponsePoll(t *testing.T) {
	request := &ntp.Packet{Poll: 8}
	response := &ntp.Packet{}
	generateResponse(timestamp, timestamp, request, response)
	assert.Equal(t, request.Poll, response.Poll)
}

func Test_generateResponseTimestamps(t *testing.T) {
	request := &ntp.Packet{TxTimeSec: 3794210679, TxTimeFrac: 2718216404}
	response := &ntp.Packet{}
	nowSec, nowFrac := ntp.Time(timestamp)

	generateResponse(timestamp, timestamp, request, response)

	// Reference Timestamp must to the closest /1000s
	lastSync := time.Unix(timestamp.Unix()/1000*1000, 0)
	lastSyncSec, lastSyncFrac := ntp.Time(lastSync)
	assert.Equal(t, lastSyncSec, response.RefTimeSec)
	assert.Equal(t, lastSyncFrac, response.RefTimeFrac)

	// Originate Timestamp must be the same
	assert.Equal(t, request.TxTimeSec, response.OrigTimeSec)
	assert.Equal(t, request.TxTimeFrac, response.OrigTimeFrac)

	// Receive Timestamp must be current timestamp
	assert.Equal(t, nowSec, response.RxTimeSec)
	assert.Equal(t, nowFrac, response.RxTimeFrac)

	// Transmit Timestamp must be current timestamp
	assert.Equal(t, nowSec, response.TxTimeSec)
	assert.Equal(t, nowFrac, response.TxTimeFrac)
}

func Benchmark_generateResponse(b *testing.B) {
	for i := 0; i < b.N; i++ {
		request := &ntp.Packet{}
		response := &ntp.Packet{}
		generateResponse(timestamp, timestamp, request, response)
	}
}

func Benchmark_fillStaticHeaders(b *testing.B) {
	s := &Server{}
	for i := 0; i < b.N; i++ {
		response := &ntp.Packet{}
		s.fillStaticHeaders(response)
	}
}
