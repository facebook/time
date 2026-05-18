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

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestSockaddrToAddrIPv4(t *testing.T) {
	sa := &unix.SockaddrInet4{
		Addr: [4]byte{192, 168, 1, 1},
		Port: 123,
	}
	addr := SockaddrToAddr(sa)
	require.True(t, addr.IsValid())
	require.Equal(t, "192.168.1.1", addr.String())
}

func TestSockaddrToAddrIPv6(t *testing.T) {
	sa := &unix.SockaddrInet6{
		Addr: [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
		Port: 319,
	}
	addr := SockaddrToAddr(sa)
	require.True(t, addr.IsValid())
	require.Equal(t, "2001:db8::1", addr.String())
}

func TestSockaddrToAddrNil(t *testing.T) {
	addr := SockaddrToAddr(nil)
	require.False(t, addr.IsValid())
}

func TestSockaddrToPortIPv4(t *testing.T) {
	sa := &unix.SockaddrInet4{
		Addr: [4]byte{10, 0, 0, 1},
		Port: 319,
	}
	require.Equal(t, 319, SockaddrToPort(sa))
}

func TestSockaddrToPortIPv6(t *testing.T) {
	sa := &unix.SockaddrInet6{
		Addr: [16]byte{},
		Port: 320,
	}
	require.Equal(t, 320, SockaddrToPort(sa))
}

func TestSockaddrToPortNil(t *testing.T) {
	require.Equal(t, 0, SockaddrToPort(nil))
}

func TestNewSockaddrWithPortIPv4(t *testing.T) {
	sa := &unix.SockaddrInet4{
		Addr: [4]byte{10, 0, 0, 1},
		Port: 123,
	}
	newSa := NewSockaddrWithPort(sa, 456)
	require.NotNil(t, newSa)
	sa4, ok := newSa.(*unix.SockaddrInet4)
	require.True(t, ok)
	require.Equal(t, 456, sa4.Port)
	require.Equal(t, [4]byte{10, 0, 0, 1}, sa4.Addr)
}

func TestNewSockaddrWithPortIPv6(t *testing.T) {
	sa := &unix.SockaddrInet6{
		Addr: [16]byte{0x20, 0x01, 0x0d, 0xb8},
		Port: 319,
	}
	newSa := NewSockaddrWithPort(sa, 320)
	require.NotNil(t, newSa)
	sa6, ok := newSa.(*unix.SockaddrInet6)
	require.True(t, ok)
	require.Equal(t, 320, sa6.Port)
	require.Equal(t, [16]byte{0x20, 0x01, 0x0d, 0xb8}, sa6.Addr)
}

func TestNewSockaddrWithPortNil(t *testing.T) {
	require.Nil(t, NewSockaddrWithPort(nil, 123))
}
