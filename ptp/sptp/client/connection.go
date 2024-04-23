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
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/facebook/time/timestamp"
)

// UDPConn describes what functionality we expect from UDP connection
type UDPConn interface {
	ReadFromUDP(b []byte) (int, *net.UDPAddr, error)
	WriteTo(b []byte, addr net.Addr) (int, error)
	Close() error
}

// UDPConnWithTS describes what functionality we expect from UDP connection that allows us to read TX timestamps
type UDPConnWithTS interface {
	UDPConn
	WriteToWithTS(b []byte, addr net.Addr) (int, time.Time, error)
	ReadPacketWithRXTimestamp() ([]byte, unix.Sockaddr, time.Time, error)
}

// UDPConnTS is a wrapper around udp connection and a corresponding fd
type UDPConnTS struct {
	*net.UDPConn
	connFd int
	l      sync.Mutex
}

// NewUDPConnTS initialises a new struct UDPConnTS
func NewUDPConnTS(conn *net.UDPConn, connFd int) *UDPConnTS {
	return &UDPConnTS{
		UDPConn: conn,
		connFd:  connFd,
	}
}

// WriteToWithTS writes bytes to addr via underlying UDPConn
func (c *UDPConnTS) WriteToWithTS(b []byte, addr net.Addr) (int, time.Time, error) {
	c.l.Lock()
	defer c.l.Unlock()
	n, err := c.WriteTo(b, addr)
	if err != nil {
		return 0, time.Time{}, err
	}
	hwts, _, err := timestamp.ReadTXtimestamp(c.connFd)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to get timestamp of last packet: %w", err)
	}
	return n, hwts, nil
}

// ReadPacketWithRXTimestamp reads bytes and a timestamp from underlying fd
func (c *UDPConnTS) ReadPacketWithRXTimestamp() ([]byte, unix.Sockaddr, time.Time, error) {
	return timestamp.ReadPacketWithRXTimestamp(c.connFd)
}
