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

package timestamp

// Here we have basic HW and SW timestamping support

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/sys/unix"
)

// from include/uapi/linux/net_tstamp.h
const (
	// HWTSTAMP_TX_ON int 1
	hwtstampTXON int32 = 0x00000001
	// HWTSTAMP_FILTER_ALL int 1
	hwtstampFilterAll int32 = 0x00000001
	// HWTSTAMP_FILTER_PTP_V2_EVENT int 12
	hwtstampFilterPTPv2Event int32 = 0x0000000c
)

const (
	// Control is a socket control message containing TX/RX timestamp
	// If the read fails we may endup with multiple timestamps in the buffer
	// which is best to read right away
	ControlSizeBytes = 128
	// ptp packets usually up to 66 bytes
	PayloadSizeBytes = 128
	// look only for X sequential TS
	maxTXTS = 100
	// Socket Control Message Header Offset on Linux
)

const (
	// HWTIMESTAMP is a hardware timestamp
	HWTIMESTAMP = "hardware"
	// SWTIMESTAMP is a software timestmap
	SWTIMESTAMP = "software"
)

// Ifreq is a struct for ioctl ethernet manipulation syscalls.
type ifreq struct {
	name [unix.IFNAMSIZ]byte
	data uintptr
}

// from include/uapi/linux/net_tstamp.h
type hwtstampСonfig struct {
	flags    int32
	txType   int32
	rxFilter int32
}

// ConnFd returns file descriptor of a connection
func ConnFd(conn *net.UDPConn) (int, error) {
	sc, err := conn.SyscallConn()
	if err != nil {
		return -1, err
	}
	var intfd int
	err = sc.Control(func(fd uintptr) {
		intfd = int(fd)
	})
	if err != nil {
		return -1, err
	}
	return intfd, nil
}

// ReadPacketWithRXTimestamp returns byte packet and HW RX timestamp
func ReadPacketWithRXTimestamp(connFd int) ([]byte, unix.Sockaddr, time.Time, error) {
	// Accessing hw timestamp
	buf := make([]byte, PayloadSizeBytes)
	oob := make([]byte, ControlSizeBytes)

	bbuf, sa, t, err := ReadPacketWithRXTimestampBuf(connFd, buf, oob)
	return buf[:bbuf], sa, t, err
}

// ReadPacketWithRXTimestampBuf writes byte packet into provide buffer buf, and returns number of bytes copied to the buffer, client ip and HW RX timestamp.
// oob buffer can be reaused after ReadPacketWithRXTimestampBuf call.
func ReadPacketWithRXTimestampBuf(connFd int, buf, oob []byte) (int, unix.Sockaddr, time.Time, error) {
	bbuf, boob, _, saddr, err := unix.Recvmsg(connFd, buf, oob, 0)
	if err != nil {
		return 0, nil, time.Time{}, fmt.Errorf("failed to read timestamp: %v", err)
	}

	timestamp, err := socketControlMessageTimestamp(oob[:boob])
	return bbuf, saddr, timestamp, err
}

// IPToSockaddr converts IP + port into a socket address
// Somewhat copy from https://github.com/golang/go/blob/16cd770e0668a410a511680b2ac1412e554bd27b/src/net/ipsock_posix.go#L145
func IPToSockaddr(ip net.IP, port int) unix.Sockaddr {
	if ip.To4() != nil {
		sa := &unix.SockaddrInet4{Port: port}
		copy(sa.Addr[:], ip.To4())
		return sa
	} else {
		sa := &unix.SockaddrInet6{Port: port}
		copy(sa.Addr[:], ip.To16())
		return sa
	}
}

// SockaddrToIP converts socket address to an IP
// Somewhat copy from https://github.com/golang/go/blob/658b5e66ecbc41a49e6fb5aa63c5d9c804cf305f/src/net/udpsock_posix.go#L15
func SockaddrToIP(sa unix.Sockaddr) net.IP {
	switch sa := sa.(type) {
	case *unix.SockaddrInet4:
		return sa.Addr[0:]
	case *unix.SockaddrInet6:
		return sa.Addr[0:]
	}
	return nil
}
