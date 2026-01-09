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

package chrony

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFloat(t *testing.T) {
	testCases := []struct {
		in  chronyFloat
		out float64
	}{
		{
			in:  chronyFloat(0),
			out: 0.0,
		},
		{
			in:  chronyFloat(17091950),
			out: -0.490620,
		},
		{
			in:  chronyFloat(-90077357),
			out: 0.039435696,
		},
	}

	for _, testCase := range testCases {
		// can't really compare big floats, thus measure delta
		require.InDelta(
			t,
			testCase.out,
			testCase.in.ToFloat(),
			0.000001,
		)
	}
}

func TestRefidToString(t *testing.T) {
	testCases := []struct {
		in  uint32
		out string
	}{
		{
			in:  0,
			out: "",
		},
		{
			in:  1196446464,
			out: "GPS",
		},
		{
			in:  2139029761, // This doesn't convert to a printable string
			out: "7F7F0101", // Prints hex
		},
		{
			in:  0xC0A80001, // 192.168.0.1 as uint32
			out: "C0A80001", // Prints hex
		},
	}

	for _, testCase := range testCases {
		require.Equal(
			t,
			testCase.out,
			RefidToString(testCase.in),
		)
	}
}

func TestNTPTestsFlagsString(t *testing.T) {
	testCases := []struct {
		in  uint16
		out []string
	}{
		{
			in:  255,
			out: []string{"tst_delay_dev_ration", "tst_sync_loop"},
		},
		{
			in:  65535,
			out: []string{},
		},
	}

	for _, testCase := range testCases {
		require.ElementsMatch(
			t,
			testCase.out,
			ReadNTPTestFlags(testCase.in),
		)
	}
}

func TestIPAddrToNetIP(t *testing.T) {
	testCases := []struct {
		name  string
		in    *IPAddr
		out   net.IP
		isNil bool
	}{
		{
			name: "IPv4 address",
			in: &IPAddr{
				IP:     IPToBytes(net.ParseIP("192.168.1.1")),
				Family: IPAddrInet4,
			},
			out:   net.IP{192, 168, 1, 1},
			isNil: false,
		},
		{
			name: "IPv6 address",
			in: &IPAddr{
				IP:     IPToBytes(net.ParseIP("2001:db8::1")),
				Family: IPAddrInet6,
			},
			out:   net.ParseIP("2001:db8::1"),
			isNil: false,
		},
		{
			name: "IPADDR_ID (unresolved address)",
			in: &IPAddr{
				IP:     [16]uint8{0, 0, 0, 9},
				Family: IPAddrID,
			},
			out:   nil,
			isNil: true,
		},
		{
			name: "IPADDR_UNSPEC",
			in: &IPAddr{
				Family: IPAddrUnspec,
			},
			out:   nil,
			isNil: true,
		},
		{
			name: "Unknown family type",
			in: &IPAddr{
				Family: 99,
			},
			out:   nil,
			isNil: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.in.ToNetIP()
			if tc.isNil {
				require.Nil(t, result)
			} else {
				require.Equal(t, tc.out, result)
			}
		})
	}
}

func TestIPAddrString(t *testing.T) {
	testCases := []struct {
		name string
		in   *IPAddr
		out  string
	}{
		{
			name: "IPv4 address",
			in: &IPAddr{
				IP:     IPToBytes(net.ParseIP("192.168.1.1")),
				Family: IPAddrInet4,
			},
			out: "192.168.1.1",
		},
		{
			name: "IPv6 address",
			in: &IPAddr{
				IP:     IPToBytes(net.ParseIP("2001:db8::1")),
				Family: IPAddrInet6,
			},
			out: "2001:db8::1",
		},
		{
			name: "IPADDR_ID with ID 9",
			in: &IPAddr{
				IP:     [16]uint8{0, 0, 0, 9},
				Family: IPAddrID,
			},
			out: "ID#0000000009",
		},
		{
			name: "IPADDR_ID with ID 16 (0x10)",
			in: &IPAddr{
				IP:     [16]uint8{0, 0, 0, 0x10},
				Family: IPAddrID,
			},
			out: "ID#0000000010",
		},
		{
			name: "IPADDR_ID with larger ID",
			in: &IPAddr{
				IP:     [16]uint8{0x12, 0x34, 0x56, 0x78},
				Family: IPAddrID,
			},
			out: "ID#0012345678",
		},
		{
			name: "IPADDR_UNSPEC",
			in: &IPAddr{
				Family: IPAddrUnspec,
			},
			out: "",
		},
		{
			name: "Unknown family type",
			in: &IPAddr{
				Family: 99,
			},
			out: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.in.String()
			require.Equal(t, tc.out, result)
		})
	}
}
