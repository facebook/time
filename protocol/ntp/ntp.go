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
	"time"

	syscall "golang.org/x/sys/unix"
)

// NTPEpochNanosecond is the difference between NTP and Unix epoch in NS
const NTPEpochNanosecond = int64(2208988800000000000)

// Time is converting Unix time to sec and frac NTP format
func Time(t time.Time) (seconds uint32, fracions uint32) {
	nsec := t.UnixNano() + NTPEpochNanosecond
	sec := nsec / time.Second.Nanoseconds()
	return uint32(sec), uint32((nsec - sec*time.Second.Nanoseconds()) << 32 / time.Second.Nanoseconds())
}

// Unix is converting NTP seconds and fractions into Unix time
func Unix(seconds, fractions uint32) time.Time {
	secs := int64(seconds) - NTPEpochNanosecond/time.Second.Nanoseconds()
	nanos := (int64(fractions) * time.Second.Nanoseconds()) >> 32 // convert fractional to nanos
	return time.Unix(secs, nanos)
}

// abs returns the absolute value of x
func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// AvgNetworkDelay uses formula from RFC #958 to calculate average network delay
func AvgNetworkDelay(clientTransmitTime, serverReceiveTime, serverTransmitTime, clientReceiveTime time.Time) int64 {
	forwardPath := serverReceiveTime.Sub(clientTransmitTime).Nanoseconds()
	returnPath := clientReceiveTime.Sub(serverTransmitTime).Nanoseconds()

	return abs(forwardPath+returnPath) / 2
}

// CurrentRealTime returns "true" unix time after adjusting to avg network offset
func CurrentRealTime(serverTransmitTime time.Time, avgNetworkDelay int64) time.Time {
	return serverTransmitTime.Add(time.Duration(avgNetworkDelay) * time.Nanosecond)
}

// CalculateOffset returns offset between local time and "real" time
func CalculateOffset(currentRealTime, curentLocaTime time.Time) int64 {
	return currentRealTime.UnixNano() - curentLocaTime.UnixNano()
}

// sockaddrToUDP converts syscall.Sockaddr to net.Addr
func sockaddrToUDP(sa syscall.Sockaddr) net.Addr {
	switch sa := sa.(type) {
	case *syscall.SockaddrInet4:
		return &net.UDPAddr{IP: sa.Addr[0:], Port: sa.Port}
	case *syscall.SockaddrInet6:
		return &net.UDPAddr{IP: sa.Addr[0:], Port: sa.Port}
	}
	return nil
}

// connFd returns file descriptor of a connection
func connFd(conn *net.UDPConn) (int, error) {
	connfd, err := conn.File()
	if err != nil {
		return -1, err
	}
	return int(connfd.Fd()), nil
}
