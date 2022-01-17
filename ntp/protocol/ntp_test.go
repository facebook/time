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

package protocol

import (
	"net"
	"testing"
	"time"

	"github.com/facebook/time/timestamp"
	"golang.org/x/sys/unix"

	"github.com/stretchr/testify/require"
)

var (
	// Unix
	usec  = int64(1585147599)
	unsec = int64(631495778)
	// NTP
	nsec  = uint32(3794136399)
	nfrac = uint32(2712253714)

	// Network Delays
	forwardDelay = 10 * time.Millisecond
	returnDelay  = 20 * time.Millisecond

	// avgNetworkDelay nanoseconds
	avgNetworkDelay = int64(15000000)

	// offset between local and remote clock
	offset = 123 * time.Microsecond

	// Packet request. From ntpdate run
	ntpRequest = &Packet{
		Settings:       227,
		Stratum:        0,
		Poll:           3,
		Precision:      -6,
		RootDelay:      65536,
		RootDispersion: 65536,
		ReferenceID:    0,
		RefTimeSec:     0,
		RefTimeFrac:    0,
		OrigTimeSec:    0,
		OrigTimeFrac:   0,
		RxTimeSec:      0,
		RxTimeFrac:     0,
		TxTimeSec:      3794210679,
		TxTimeFrac:     2718216404,
	}

	// Same request as above in bytes
	ntpRequestBytes = []byte{227, 0, 3, 250, 0, 1, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 226, 39, 15, 119, 162, 4, 176, 212}

	// Packet response
	ntpResponse = &Packet{
		Settings:       36,
		Stratum:        1,
		Poll:           3,
		Precision:      -32,
		RootDelay:      0,
		RootDispersion: 10,
		ReferenceID:    1178738720,
		RefTimeSec:     3794209800,
		RefTimeFrac:    0,
		OrigTimeSec:    3794210679,
		OrigTimeFrac:   2718216404,
		RxTimeSec:      3794210679,
		RxTimeFrac:     2718375472,
		TxTimeSec:      3794210679,
		TxTimeFrac:     2719753478,
	}
	// Same response as above in bytes
	ntpResponseBytes = []byte{36, 1, 3, 224, 0, 0, 0, 0, 0, 0, 0, 10, 70, 66, 32, 32, 226, 39, 12, 8, 0, 0, 0, 0, 226, 39, 15, 119, 162, 4, 176, 212, 226, 39, 15, 119, 162, 7, 30, 48, 226, 39, 15, 119, 162, 28, 37, 6}

	ntpBadRequest = &Packet{Settings: 0}
)

// Testing conversion so if Packet structure changes we notice
func TestRequestConversion(t *testing.T) {
	bytes, err := ntpRequest.Bytes()
	require.NoError(t, err)
	require.Equal(t, ntpRequestBytes, bytes)
}

// Testing conversion so if Packet structure changes we notice
func TestResponseConersion(t *testing.T) {
	bytes, err := ntpResponse.Bytes()
	require.NoError(t, err)
	require.Equal(t, ntpResponseBytes, bytes)
}

func TestBytesToPacket(t *testing.T) {
	packet, err := BytesToPacket(ntpResponseBytes)
	require.NoError(t, err)
	require.Equal(t, ntpResponse, packet)
}

func TestBytesToPacketError(t *testing.T) {
	bytes := []byte{}
	packet, err := BytesToPacket(bytes)
	require.NotNil(t, err)
	require.Equal(t, &Packet{}, packet)
}

// Testing conversion so if Packet structure changes we notice
func TestPacketConversionFailure(t *testing.T) {
	bytes, err := ntpRequest.Bytes()
	require.NoError(t, err)
	require.Equal(t, ntpRequestBytes, bytes)
}

func TestRequestSize(t *testing.T) {
	require.Equal(t, PacketSizeBytes, len(ntpRequestBytes))
}

func TestResponseSize(t *testing.T) {
	require.Equal(t, PacketSizeBytes, len(ntpResponseBytes))
}

func TestValidSettingsFormat(t *testing.T) {
	require.True(t, ntpRequest.ValidSettingsFormat())
}

func TestInvalidSettingsFormat(t *testing.T) {
	require.False(t, ntpBadRequest.ValidSettingsFormat())
}

func TestTime(t *testing.T) {
	testtime := time.Unix(usec, unsec)
	sec, frac := Time(testtime)

	require.Equal(t, nsec, sec)
	require.Equal(t, nfrac, frac)
}

func TestUnix(t *testing.T) {
	testtime := Unix(nsec, nfrac)

	require.Equal(t, usec, testtime.Unix())
	// +1ns is a rounding issue
	require.Equal(t, unsec, int64(testtime.Nanosecond())+1)
}

func TestAbs(t *testing.T) {
	require.Equal(t, abs(1), int64(1))
	require.Equal(t, abs(-1), int64(1))
}

func TestAvgNetworkDelay(t *testing.T) {
	// Time on server is = of time on client
	clientTransmitTime := time.Now()
	// Network delay client -> server 10ms
	serverReceiveTime := clientTransmitTime.Add(forwardDelay)
	// OS delay server 10us
	serverTransmitTime := serverReceiveTime.Add(10 * time.Microsecond)
	// Network delay client -> server 20ms
	clientReceiveTime := serverTransmitTime.Add(returnDelay)

	actualAvgNetworkDelay := AvgNetworkDelay(clientTransmitTime, serverReceiveTime, serverTransmitTime, clientReceiveTime)
	require.Equal(t, avgNetworkDelay, actualAvgNetworkDelay)
}

func TestAvgNetworkDelayPositive(t *testing.T) {
	// Assuming time on client is > of time on server
	clientToServer := 50 * time.Millisecond

	clientTransmitTime := time.Now()
	// Network delay client -> server 10ms
	serverReceiveTime := clientTransmitTime.Add(forwardDelay)
	// OS delay server 10us
	serverTransmitTime := serverReceiveTime.Add(10 * time.Microsecond)
	// Network delay client -> server 20ms
	clientReceiveTime := serverTransmitTime.Add(returnDelay)

	actualAvgNetworkDelay := AvgNetworkDelay(clientTransmitTime.Add(clientToServer), serverReceiveTime, serverTransmitTime, clientReceiveTime.Add(clientToServer))
	require.Equal(t, avgNetworkDelay, actualAvgNetworkDelay)
}

func TestAvgNetworkDelayNegative(t *testing.T) {
	// Assuming time on client is < of time on server
	clientToServer := -50 * time.Millisecond

	clientTransmitTime := time.Now()
	// Network delay client -> server 10ms
	serverReceiveTime := clientTransmitTime.Add(forwardDelay)
	// OS delay server 10us
	serverTransmitTime := serverReceiveTime.Add(10 * time.Microsecond)
	// Network delay client -> server 20ms
	clientReceiveTime := serverTransmitTime.Add(returnDelay)

	actualAvgNetworkDelay := AvgNetworkDelay(clientTransmitTime.Add(clientToServer), serverReceiveTime, serverTransmitTime, clientReceiveTime.Add(clientToServer))
	require.Equal(t, avgNetworkDelay, actualAvgNetworkDelay)
}

func TestCurrentRealTime(t *testing.T) {
	serverTransmitTime := time.Now()
	currentRealTime := CurrentRealTime(serverTransmitTime, avgNetworkDelay)
	require.Equal(t, serverTransmitTime.Add(time.Duration(avgNetworkDelay)*time.Nanosecond), currentRealTime)
}

func TestCalculateOffset(t *testing.T) {
	curentLocaTime := time.Now()
	currentRealTime := curentLocaTime.Add(offset)

	actualOffset := CalculateOffset(currentRealTime, curentLocaTime)
	require.Equal(t, offset.Nanoseconds(), actualOffset)
}

func TestReadNTPPacket(t *testing.T) {
	// listen to incoming udp packets
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("localhost"), Port: 0})
	require.NoError(t, err)
	defer conn.Close()

	// Send a client request
	cconn, err := net.Dial("udp", conn.LocalAddr().String())
	require.NoError(t, err)
	defer cconn.Close()
	_, err = cconn.Write(ntpRequestBytes)
	require.NoError(t, err)

	request, returnaddr, err := ReadNTPPacket(conn)
	require.Equal(t, ntpRequest, request, "We should have the same request arriving on the server")
	require.Equal(t, cconn.LocalAddr().String(), returnaddr.String())
	require.NoError(t, err)
}

func Benchmark_PacketToBytesConversion(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = ntpResponse.Bytes()
	}
}

func Benchmark_BytesToPacketConversion(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = BytesToPacket(ntpResponseBytes)
	}
}

/*
Benchmark_ServerWithoutKernelTimestamps is a benchmark to determine speed of
reading NTP packets without kernel timestamps
Usually numbers look like:

~/go/src/github.com/facebook/time/ntp/protocol/ntp go test -bench=ServerWithoutKernelTimestamps
goos: linux
goarch: amd64
pkg: github.com/facebook/time/ntp/protocol/ntp
Benchmark_ServerWithoutKernelTimestamps-24    	  204441	      4997 ns/op
PASS
ok  	github.com/facebook/time/ntp/protocol/ntp	1.094s
*/
func Benchmark_ServerWithoutKernelTimestamps(b *testing.B) {
	// Server
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("localhost"), Port: 0})
	require.Nil(b, err)
	defer conn.Close()

	// Client
	addr, err := net.ResolveUDPAddr("udp", conn.LocalAddr().String())
	require.Nil(b, err)
	cconn, err := net.DialUDP("udp", nil, addr)
	require.Nil(b, err)
	defer cconn.Close()

	for i := 0; i < b.N; i++ {
		_, _ = cconn.Write(ntpRequestBytes)
		_, _, _ = ReadNTPPacket(conn)
	}
}

func Benchmark_ServerWithKernelTimestamps(b *testing.B) {
	// Server
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("localhost"), Port: 0})
	require.Nil(b, err)
	defer conn.Close()

	// get connection file descriptor
	connFd, err := timestamp.ConnFd(conn)
	require.NoError(b, err)

	// Allow reading of kernel timestamps via socket
	err = timestamp.EnableSWTimestampsRx(connFd)
	require.NoError(b, err)

	err = unix.SetNonblock(connFd, false)
	require.NoError(b, err)

	// Client
	addr, err := net.ResolveUDPAddr("udp", conn.LocalAddr().String())
	require.Nil(b, err)
	cconn, err := net.DialUDP("udp", nil, addr)
	require.Nil(b, err)
	defer cconn.Close()

	for i := 0; i < b.N; i++ {
		_, _ = cconn.Write(ntpRequestBytes)
		_, _, _ = ReadNTPPacket(conn)
	}
}

/*
Benchmark_ServerWithKernelTimestampsRead is a benchmark to determine speed of
reading NTP packets with kernel timestamps
Usually numbers look like:

~/go/src/github.com/facebook/time/ntp/protocol/ntp go test -bench=ServerWithKernelTimestampsRead
goos: linux
goarch: amd64
pkg: github.com/facebook/time/ntp/protocol/ntp
Benchmark_ServerWithKernelTimestampsRead-24    	  143074	      8084 ns/op
PASS
ok  	github.com/facebook/time/ntp/protocol/ntp	1.778s
*/
func Benchmark_ServerWithKernelTimestampsRead(b *testing.B) {
	request := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 42}
	// Server
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("localhost"), Port: 0})
	require.Nil(b, err)
	defer conn.Close()

	// get connection file descriptor
	connFd, err := timestamp.ConnFd(conn)
	require.NoError(b, err)

	// Allow reading of kernel timestamps via socket
	err = timestamp.EnableSWTimestampsRx(connFd)
	require.NoError(b, err)

	err = unix.SetNonblock(connFd, false)
	require.NoError(b, err)

	// Client
	addr, err := net.ResolveUDPAddr("udp", conn.LocalAddr().String())
	require.Nil(b, err)
	cconn, err := net.DialUDP("udp", nil, addr)
	require.Nil(b, err)
	defer cconn.Close()

	for i := 0; i < b.N; i++ {
		_, _ = cconn.Write(request)
		_, _, _, _ = timestamp.ReadPacketWithRXTimestamp(connFd)
	}
}
