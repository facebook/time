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
	"net/netip"
	"time"

	"golang.org/x/sys/unix"
)

const (
	// ControlSizeBytes is a socket control message containing TX/RX timestamp
	// If the read fails we may endup with multiple timestamps in the buffer
	// which is best to read right away
	ControlSizeBytes = 128
	// PayloadSizeBytes is a size of maximum ptp packet which is usually up to 66 bytes
	PayloadSizeBytes = 128
	// look only for X sequential TS
	defaultTXTS = 100
	// SizeofSeqID is the size of the sequence ID field in bytes
	SizeofSeqID = 0x4 // 4 bytes
)

// Timestamp is a type of timestamp
type Timestamp int

const (
	// SW is a software timestamp
	SW Timestamp = iota
	// SWRX is a software RX timestamp
	SWRX
	// HW is a hardware timestamp
	HW
	// HWRX is a hardware RX timestamp
	HWRX
)

// Unsupported is a string for unsupported timestamp
const Unsupported = "Unsupported"

// timestampToString is a map from Timestamp to string
var timestampToString = map[Timestamp]string{
	SW:   "software",
	SWRX: "software_rx",
	HW:   "hardware",
	HWRX: "hardware_rx",
}

// MarshalText timestamp to byte slice
func (t Timestamp) MarshalText() ([]byte, error) {
	_, ok := timestampToString[t]
	if ok {
		return []byte(t.String()), nil
	}
	return []byte(Unsupported), fmt.Errorf("unknown timestamp type %q", Unsupported)
}

// String timestamp to string
func (t Timestamp) String() string {
	v, ok := timestampToString[t]
	if ok {
		return v
	}
	return Unsupported
}

// timestampFromString returns channel from string
func timestampFromString(value string) (*Timestamp, error) {
	for k, v := range timestampToString {
		if v == value {
			return &k, nil
		}
	}
	return nil, fmt.Errorf("unknown timestamp type %q", value)
}

// UnmarshalText timestamp from byte slice
func (t *Timestamp) UnmarshalText(value []byte) error {
	return t.Set(string(value))
}

// Set timestamp from string
func (t *Timestamp) Set(value string) error {
	ts, err := timestampFromString(value)
	if err != nil {
		return err
	}
	*t = *ts
	return nil
}

// Type is required by the cobra.Value interface
func (t *Timestamp) Type() string {
	return "timestamp"
}

// AttemptsTXTS is the configured amount of attempts to read TX timestamp
var AttemptsTXTS = defaultTXTS

// TimeoutTXTS is the configured timeout to read TX timestamp
var TimeoutTXTS = time.Millisecond

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
		return 0, nil, time.Time{}, fmt.Errorf("failed to read timestamp: %w", err)
	}

	timestamp, err := socketControlMessageTimestamp(oob, boob)
	return bbuf, saddr, timestamp, err
}

// IPToSockaddr converts IP + port into a socket address
// Somewhat copy from https://github.com/golang/go/blob/16cd770e0668a410a511680b2ac1412e554bd27b/src/net/ipsock_posix.go#L145
func IPToSockaddr(ip net.IP, port int) unix.Sockaddr {
	if ip.To4() != nil {
		sa := &unix.SockaddrInet4{Port: port}
		copy(sa.Addr[:], ip.To4())
		return sa
	}
	sa := &unix.SockaddrInet6{Port: port}
	copy(sa.Addr[:], ip.To16())
	return sa
}

// AddrToSockaddr converts netip.Addr + port into a socket address
func AddrToSockaddr(ip netip.Addr, port int) unix.Sockaddr {
	if ip.Is4() {
		return &unix.SockaddrInet4{Port: port, Addr: ip.As4()}
	}
	return &unix.SockaddrInet6{Port: port, Addr: ip.As16()}
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

// SockaddrToAddr converts socket address to a netip.Addr
// Somewhat copy from https://github.com/golang/go/blob/658b5e66ecbc41a49e6fb5aa63c5d9c804cf305f/src/net/udpsock_posix.go#L15
func SockaddrToAddr(sa unix.Sockaddr) netip.Addr {
	switch sa := sa.(type) {
	case *unix.SockaddrInet4:
		return netip.AddrFrom4(sa.Addr).Unmap()
	case *unix.SockaddrInet6:
		return netip.AddrFrom16(sa.Addr).Unmap()
	}
	return netip.Addr{}
}

// SockaddrToPort converts socket address to an IP
// Somewhat copy from https://github.com/golang/go/blob/658b5e66ecbc41a49e6fb5aa63c5d9c804cf305f/src/net/udpsock_posix.go#L15
func SockaddrToPort(sa unix.Sockaddr) int {
	switch sa := sa.(type) {
	case *unix.SockaddrInet4:
		return sa.Port
	case *unix.SockaddrInet6:
		return sa.Port
	}
	return 0
}

// NewSockaddrWithPort creates a new socket address with the same IP and new port
func NewSockaddrWithPort(sa unix.Sockaddr, port int) unix.Sockaddr {
	switch sa := sa.(type) {
	case *unix.SockaddrInet4:
		return &unix.SockaddrInet4{Addr: sa.Addr, Port: port}
	case *unix.SockaddrInet6:
		return &unix.SockaddrInet6{Addr: sa.Addr, Port: port}
	}
	return nil
}
