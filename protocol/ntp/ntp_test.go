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

package ntp

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	syscall "golang.org/x/sys/unix"
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
func Test_RequestConversion(t *testing.T) {
	bytes, err := ntpRequest.Bytes()
	assert.Nil(t, err)
	assert.Equal(t, ntpRequestBytes, bytes)
}

// Testing conversion so if Packet structure changes we notice
func Test_ResponseConersion(t *testing.T) {
	bytes, err := ntpResponse.Bytes()
	assert.Nil(t, err)
	assert.Equal(t, ntpResponseBytes, bytes)
}

func Test_BytesToPacket(t *testing.T) {
	packet, err := BytesToPacket(ntpResponseBytes)
	assert.Nil(t, err)
	assert.Equal(t, ntpResponse, packet)
}

func Test_BytesToPacketError(t *testing.T) {
	bytes := []byte{}
	packet, err := BytesToPacket(bytes)
	assert.NotNil(t, err)
	assert.Equal(t, &Packet{}, packet)
}

// Testing conversion so if Packet structure changes we notice
func Test_PacketConversionFailure(t *testing.T) {
	bytes, err := ntpRequest.Bytes()
	assert.Nil(t, err)
	assert.Equal(t, ntpRequestBytes, bytes)
}

func Test_RequestSize(t *testing.T) {
	assert.Equal(t, NTPPacketSizeBytes, len(ntpRequestBytes))
}

func Test_ResponseSize(t *testing.T) {
	assert.Equal(t, NTPPacketSizeBytes, len(ntpResponseBytes))
}

func Test_ValidSettingsFormat(t *testing.T) {
	assert.True(t, ntpRequest.ValidSettingsFormat())
}

func Test_invalidSettingsFormat(t *testing.T) {
	assert.False(t, ntpBadRequest.ValidSettingsFormat())
}

func Test_Time(t *testing.T) {
	testtime := time.Unix(usec, unsec)
	sec, frac := Time(testtime)

	assert.Equal(t, nsec, sec)
	assert.Equal(t, nfrac, frac)
}

func Test_Unix(t *testing.T) {
	testtime := Unix(nsec, nfrac)

	assert.Equal(t, usec, testtime.Unix())
	// +1ns is a rounding issue
	assert.Equal(t, unsec, int64(testtime.Nanosecond())+1)
}

func Test_abs(t *testing.T) {
	assert.Equal(t, abs(1), int64(1))
	assert.Equal(t, abs(-1), int64(1))
}

func Test_AvgNetworkDelay(t *testing.T) {
	// Time on server is = of time on client
	clientTransmitTime := time.Now()
	// Network delay client -> server 10ms
	serverReceiveTime := clientTransmitTime.Add(forwardDelay)
	// OS delay server 10us
	serverTransmitTime := serverReceiveTime.Add(10 * time.Microsecond)
	// Network delay client -> server 20ms
	clientReceiveTime := serverTransmitTime.Add(returnDelay)

	actualAvgNetworkDelay := AvgNetworkDelay(clientTransmitTime, serverReceiveTime, serverTransmitTime, clientReceiveTime)
	assert.Equal(t, avgNetworkDelay, actualAvgNetworkDelay)
}

func Test_AvgNetworkDelayPositive(t *testing.T) {
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
	assert.Equal(t, avgNetworkDelay, actualAvgNetworkDelay)
}

func Test_AvgNetworkDelayNegative(t *testing.T) {
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
	assert.Equal(t, avgNetworkDelay, actualAvgNetworkDelay)
}

func Test_CurrentRealTime(t *testing.T) {
	serverTransmitTime := time.Now()
	currentRealTime := CurrentRealTime(serverTransmitTime, avgNetworkDelay)
	assert.Equal(t, serverTransmitTime.Add(time.Duration(avgNetworkDelay)*time.Nanosecond), currentRealTime)
}

func Test_CalculateOffset(t *testing.T) {
	curentLocaTime := time.Now()
	currentRealTime := curentLocaTime.Add(offset)

	actualOffset := CalculateOffset(currentRealTime, curentLocaTime)
	assert.Equal(t, offset.Nanoseconds(), actualOffset)
}

func Test_connFd(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	assert.Nil(t, err)
	defer conn.Close()

	connfd, err := connFd(conn)
	assert.Nil(t, err)
	assert.Greater(t, connfd, 0, "connection fd must be > 0")
}

func Test_EnableKernelTimestampsSocket(t *testing.T) {
	// listen to incoming udp packets
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	assert.Nil(t, err)
	defer conn.Close()

	connfd, err := connFd(conn)
	assert.Nil(t, err)

	// Allow reading of hardware/kernel timestamps via socket
	err = EnableKernelTimestampsSocket(conn)
	assert.Nil(t, err)

	// Check that socket option is set
	hwTimestampsEnabled, err := syscall.GetsockoptInt(connfd, syscall.SOL_SOCKET, syscall.SO_TIMESTAMPNS)
	assert.Nil(t, err)
	kernelTimestampsEnabled, err := syscall.GetsockoptInt(connfd, syscall.SOL_SOCKET, syscall.SO_TIMESTAMP)
	assert.Nil(t, err)

	// At least one of them should be set, which it > 0
	assert.Greater(t, hwTimestampsEnabled+kernelTimestampsEnabled, 0, "None of the socket options is set")
}

func Test_ReadNTPPacket(t *testing.T) {
	// listen to incoming udp packets
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	assert.Nil(t, err)
	defer conn.Close()

	// Send a client request
	addr, err := net.ResolveUDPAddr("udp", conn.LocalAddr().String())
	assert.Nil(t, err)
	cconn, err := net.DialUDP("udp", nil, addr)
	assert.Nil(t, err)
	defer cconn.Close()
	_, err = cconn.Write(ntpRequestBytes)
	assert.Nil(t, err)

	request, returnaddr, err := ReadNTPPacket(conn)
	assert.Equal(t, ntpRequest, request, "We should have the same request arriving on the server")
	assert.Equal(t, returnaddr, cconn.LocalAddr())
	assert.Nil(t, err)
}

func Test_ReadPacketWithKernelTimestamp(t *testing.T) {
	// listen to incoming udp packets
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	assert.Nil(t, err)
	defer conn.Close()

	// Allow reading of hardware/kernel timestamps via socket
	err = EnableKernelTimestampsSocket(conn)
	assert.Nil(t, err)

	// Send a client request
	timeout := 1 * time.Second
	cconn, err := net.DialTimeout("udp", conn.LocalAddr().String(), timeout)
	assert.Nil(t, err)
	defer cconn.Close()
	_, err = cconn.Write(ntpRequestBytes)
	assert.Nil(t, err)

	// read HW/kernel timestamp from incoming packet
	request, nowHWtimestamp, returnaddr, err := ReadPacketWithKernelTimestamp(conn)
	assert.Equal(t, ntpRequest, request, "We should have the same request arriving on the server")
	assert.Equal(t, time.Now().Unix()/10, nowHWtimestamp.Unix()/10, "hwtimestamps should be within 10s")
	assert.Equal(t, returnaddr, cconn.LocalAddr())
	assert.Nil(t, err)
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

func Benchmark_ServerWithoutHWTimestamps(b *testing.B) {
	// Server
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	assert.Nil(b, err)
	defer conn.Close()

	// Client
	addr, err := net.ResolveUDPAddr("udp", conn.LocalAddr().String())
	assert.Nil(b, err)
	cconn, err := net.DialUDP("udp", nil, addr)
	assert.Nil(b, err)
	defer cconn.Close()

	for i := 0; i < b.N; i++ {
		_, _ = cconn.Write(ntpRequestBytes)
		_, _, _ = ReadNTPPacket(conn)
	}
}

func Benchmark_ServerWithHWTimestamps(b *testing.B) {
	// Server
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	assert.Nil(b, err)
	defer conn.Close()

	// Allow reading of hardware/kernel timestamps via socket
	err = EnableKernelTimestampsSocket(conn)
	assert.Nil(b, err)

	// Client
	addr, err := net.ResolveUDPAddr("udp", conn.LocalAddr().String())
	assert.Nil(b, err)
	cconn, err := net.DialUDP("udp", nil, addr)
	assert.Nil(b, err)
	defer cconn.Close()

	for i := 0; i < b.N; i++ {
		_, _ = cconn.Write(ntpRequestBytes)
		_, _, _ = ReadNTPPacket(conn)
	}
}

func Benchmark_ServerWithHWTimestampsRead(b *testing.B) {
	// Server
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	assert.Nil(b, err)
	defer conn.Close()

	// Allow reading of hardware/kernel timestamps via socket
	err = EnableKernelTimestampsSocket(conn)
	assert.Nil(b, err)

	// Client
	addr, err := net.ResolveUDPAddr("udp", conn.LocalAddr().String())
	assert.Nil(b, err)
	cconn, err := net.DialUDP("udp", nil, addr)
	assert.Nil(b, err)
	defer cconn.Close()

	for i := 0; i < b.N; i++ {
		_, _ = cconn.Write(ntpRequestBytes)
		_, _, _, _ = ReadPacketWithKernelTimestamp(conn)
	}
}
