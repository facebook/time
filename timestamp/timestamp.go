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
	//HWTSTAMP_FILTER_PTP_V2_L4_EVENT int 6
	hwtstampFilterPTPv2L4Event int32 = 0x00000006
	// HWTSTAMP_FILTER_PTP_V2_EVENT int 12
	hwtstampFilterPTPv2Event int32 = 0x0000000c
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
	// Socket Control Message Header Offset on Linux
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

// EthtoolGetTSInfo is get time stamping and PHC info command
const EthtoolGetTSInfo uint32 = 0x00000041

// Ifreq is a struct for ioctl ethernet manipulation syscalls.
type ifreq struct {
	name [unix.IFNAMSIZ]byte
	data uintptr
}

// from include/uapi/linux/net_tstamp.h
type hwtstampConfig struct {
	flags    int32
	txType   int32
	rxFilter int32
}

// from include/uapi/linux/ethtool.h struct ethtool_ts_info
type hwtstampCaps struct {
	cmd             uint32
	sofTimestamping uint32 /* SOF_TIMESTAMPING_* bitmask */
	phcIndex        int32
	txTypes         uint32 /* HWTSTAMP_TX_* */
	txReserved0     uint32
	txReserved1     uint32
	txReserved2     uint32
	rxFilters       uint32 /* HWTSTAMP_FILTER_ */
	rxReserved0     uint32
	rxReserved1     uint32
	rxReserved2     uint32
}

// AttemptsTXTS is configured amount of attempts to read TX timestamp
var AttemptsTXTS = defaultTXTS

// TimeoutTXTS is configured timeout to read TX timestamp
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
	}
	sa := &unix.SockaddrInet6{Port: port}
	copy(sa.Addr[:], ip.To16())
	return sa
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
