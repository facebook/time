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
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/facebook/time/dscp"
	"github.com/facebook/time/timestamp"
)

// UDPConnNoTS describes what functionality we expect from UDP connection
type UDPConnNoTS interface {
	WriteTo(b []byte, addr unix.Sockaddr) (int, error)
	ReadPacketBuf(buf []byte) (int, netip.Addr, error)
	Close() error
}

// UDPConnWithTS describes what functionality we expect from UDP connection that allows us to read TX timestamps
type UDPConnWithTS interface {
	WriteToWithTS(b []byte, addr unix.Sockaddr) (int, time.Time, error)
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

	l sync.Mutex
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
		UDPConn: *udpConn,
	}, nil
}

// WriteToWithTS writes bytes to addr via underlying UDPConn
func (c *UDPConnTS) WriteToWithTS(b []byte, addr unix.Sockaddr) (int, time.Time, error) {
	c.l.Lock()
	defer c.l.Unlock()
	var n int
	var err error
	err = unix.Sendto(c.connFd, b, 0, addr)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to send to %v: %w", addr, err)
	}
	hwts, _, err := timestamp.ReadTXtimestamp(c.connFd)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to get timestamp of last packet: %w", err)
	}
	return n, hwts, nil
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
