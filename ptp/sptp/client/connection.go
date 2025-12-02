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

package client

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/facebook/time/dscp"
	"github.com/facebook/time/timestamp"
)

// UDPConnNoTS describes the functionality we expect from a UDP connection
type UDPConnNoTS interface {
	WriteTo(b []byte, addr unix.Sockaddr) (int, error)
	ReadPacketBuf(buf []byte) (int, netip.Addr, error)
	Close() error
}

// UDPConnWithTS describes the functionality we expect from a UDP connection that will allow us to read TX timestamps
type UDPConnWithTS interface {
	WriteToWithTS(b []byte, addr unix.Sockaddr, seq uint16) (time.Time, error)
	ReadPacketWithRXTimestampBuf(buf, oob []byte) (int, unix.Sockaddr, time.Time, error)
	Close() error
}

// UDPConn is a wrapper around udp connection and a corresponding fd
type UDPConn struct {
	connFd int
}

// NewUDPConn initialises a new struct UDPConn
func NewUDPConn(address net.IP, port int) (*UDPConn, error) {
	connFd, err := listenUDP(address, port)
	if err != nil {
		return nil, err
	}
	return &UDPConn{
		connFd: connFd,
	}, nil
}

// WriteTo writes bytes to addr via underlying UDPConn
func (c *UDPConn) WriteTo(b []byte, addr unix.Sockaddr) (int, error) {
	return 0, unix.Sendto(c.connFd, b, 0, addr)
}

// ReadPacketBuf reads bytes from underlying fd
func (c *UDPConn) ReadPacketBuf(buf []byte) (int, netip.Addr, error) {
	n, saddr, err := unix.Recvfrom(c.connFd, buf, 0)
	if err != nil {
		return 0, netip.Addr{}, err
	}

	return n, timestamp.SockaddrToAddr(saddr), err
}

// Close closes underlying fd
func (c *UDPConn) Close() error {
	return unix.Close(c.connFd)
}

// UDPConnTS is a wrapper around udp connection and a corresponding fd
type UDPConnTS struct {
	UDPConn

	l           sync.Mutex
	newerKernel bool
}

// NewUDPConnTS initialises a new struct UDPConnTS
func NewUDPConnTS(address net.IP, port int, ts timestamp.Timestamp, iface *net.Interface, dscpValue int) (*UDPConnTS, error) {
	udpConn, err := NewUDPConn(address, port)
	if err != nil {
		return nil, err
	}
	if err = dscp.Enable(udpConn.connFd, address, dscpValue); err != nil {
		return nil, fmt.Errorf("setting DSCP on event socket: %w", err)
	}

	// we need to enable HW or SW timestamps on event port
	if err := timestamp.EnableTimestamps(ts, udpConn.connFd, iface); err != nil {
		return nil, fmt.Errorf("failed to enable timestamps on port %d: %w", port, err)
	}

	return &UDPConnTS{
		UDPConn:     *udpConn,
		newerKernel: true, // assume kernel is recent enough to support SCM_TS_OPT_ID
	}, nil
}

// WriteToWithTS writes bytes to addr via underlying UDPConn. Uses the Sequence ID for
// reliable matching of HW TX timestamps with socket control messages returned in the
// socket error queue by the kernel (if supported by kernel)
func (c *UDPConnTS) WriteToWithTS(b []byte, addr unix.Sockaddr, seq uint16) (time.Time, error) {
	c.l.Lock()
	defer c.l.Unlock()

	if c.newerKernel {
		hwts, err := c.sendMsgSeqIDTS(b, addr, seq)
		if err != nil {
			if errors.Is(err, unix.EINVAL) {
				c.newerKernel = false
			} else {
				return time.Time{}, fmt.Errorf("failed to send message to %v: %w", addr, err)
			}
		} else {
			return hwts, nil
		}
	}
	hwts, err := c.sendMsgTS(b, addr)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to send message to %v: %w", addr, err)
	}
	return hwts, nil
}

func (c *UDPConnTS) sendMsgSeqIDTS(b []byte, addr unix.Sockaddr, seq uint16) (time.Time, error) {
	seqID := uint32(seq)
	soob := make([]byte, unix.CmsgSpace(timestamp.SizeofSeqID))
	timestamp.SeqIDSocketControlMessage(seqID, soob)
	if err := unix.Sendmsg(c.connFd, b, soob, addr, 0); err != nil {
		return time.Time{}, fmt.Errorf("message sent to socket failed: %w", err)
	}
	toob := make([]byte, timestamp.ControlSizeBytes)
	hwts, _, err := timestamp.ReadTimeStampSeqIDBuf(c.connFd, toob, seqID)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to read TX timestamp: %w", err)
	}
	return hwts, nil
}

func (c *UDPConnTS) sendMsgTS(b []byte, addr unix.Sockaddr) (time.Time, error) {
	if err := unix.Sendto(c.connFd, b, 0, addr); err != nil {
		return time.Time{}, fmt.Errorf("message sent to socket failed: %w", err)
	}
	hwts, _, err := timestamp.ReadTXtimestamp(c.connFd)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to read TX timestamp: %w", err)
	}
	return hwts, nil
}

// ReadPacketWithRXTimestampBuf reads bytes and a timestamp from underlying fd
func (c *UDPConnTS) ReadPacketWithRXTimestampBuf(buf, oob []byte) (int, unix.Sockaddr, time.Time, error) {
	return timestamp.ReadPacketWithRXTimestampBuf(c.connFd, buf, oob)
}

func listenUDP(address net.IP, port int) (int, error) {
	domain := unix.AF_INET6
	if address.To4() != nil {
		domain = unix.AF_INET
	}
	// create a UDP socket
	connFd, err := unix.Socket(domain, unix.SOCK_DGRAM, unix.IPPROTO_UDP)
	if err != nil {
		return 0, fmt.Errorf("unable to create connection: %w", err)
	}
	if err = unix.SetsockoptInt(connFd, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); err != nil {
		return 0, fmt.Errorf("setting SO_REUSEPORT on socket: %w", err)
	}
	// set the connection to blocking mode, otherwise recvmsg will just return with nothing most of the time
	if err := unix.SetNonblock(connFd, false); err != nil {
		return 0, fmt.Errorf("failed to set event socket to blocking: %w", err)
	}
	// bind the socket to the address + port
	localAddr := timestamp.IPToSockaddr(address, port)
	if err := unix.Bind(connFd, localAddr); err != nil {
		return 0, fmt.Errorf("unable to bind %v connection: %w", localAddr, err)
	}
	return connFd, nil
}
