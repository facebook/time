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

package cmd

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCheckAgainstThresholdEdgeCases(t *testing.T) {
	s, _ := checkAgainstThreshold("test", int64(5), int64(10), int64(20), "")
	require.Equal(t, OK, s)

	s, _ = checkAgainstThreshold("test", int64(15), int64(10), int64(20), "")
	require.Equal(t, WARN, s)

	s, _ = checkAgainstThreshold("test", int64(25), int64(10), int64(20), "")
	require.Equal(t, FAIL, s)
}

func TestCheckAgainstThresholdPositive(t *testing.T) {
	s, _ := checkAgainstThresholdPositive("test", int64(5), int64(10), int64(20), "")
	require.Equal(t, OK, s)

	s, _ = checkAgainstThresholdPositive("test", int64(0), int64(10), int64(20), "")
	require.Equal(t, FAIL, s)

	s, _ = checkAgainstThresholdPositive("test", int64(-1), int64(10), int64(20), "")
	require.Equal(t, FAIL, s)
}

func TestCheckAgainstThresholdNonZero(t *testing.T) {
	s, _ := checkAgainstThresholdNonZero("test", int64(5), int64(10), int64(20), "")
	require.Equal(t, OK, s)

	s, _ = checkAgainstThresholdNonZero("test", int64(0), int64(10), int64(20), "")
	require.Equal(t, FAIL, s)
}

func TestCalculateJitterBounds(t *testing.T) {
	result := CalculateJitter(0)
	require.Equal(t, time.Duration(0), result)

	result = CalculateJitter(-1 * time.Second)
	require.Equal(t, time.Duration(0), result)

	result = CalculateJitter(100 * time.Millisecond)
	require.GreaterOrEqual(t, result, time.Duration(0))
	require.Less(t, result, 100*time.Millisecond)
}

func TestSelectNonLinkLocalAddrEdgeCases(t *testing.T) {
	// no addresses
	_, err := selectNonLinkLocalAddr(nil)
	require.Error(t, err)

	// only IPv4 (should be skipped)
	addrs := []net.Addr{
		&net.IPNet{IP: net.ParseIP("192.168.1.1"), Mask: net.CIDRMask(24, 32)},
	}
	_, err = selectNonLinkLocalAddr(addrs)
	require.Error(t, err)

	// link-local IPv6 (should be skipped)
	addrs = []net.Addr{
		&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
	}
	_, err = selectNonLinkLocalAddr(addrs)
	require.Error(t, err)

	// global unicast IPv6
	addrs = []net.Addr{
		&net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(64, 128)},
	}
	ip, err := selectNonLinkLocalAddr(addrs)
	require.NoError(t, err)
	require.Equal(t, "2001:db8::1", ip.String())
}

func TestFmtThreshold(t *testing.T) {
	// fmtThreshold returns a non-empty colored string for any value
	result := fmtThreshold(100)
	require.NotEmpty(t, result)

	// different values produce different output
	result2 := fmtThreshold(200)
	require.NotEqual(t, result, result2)
}

func TestTxTypeStringBoundary(t *testing.T) {
	// All valid TxType values return a named string, not the fallback
	for i := range 3 {
		s := TxType(i).String()
		require.NotEqual(t, "?", s, "TxType(%d) should have a name", i)
	}
	// Out of range returns fallback
	require.Equal(t, "?", TxType(99).String())
}

func TestRxFilterStringBoundary(t *testing.T) {
	// All valid RxFilter values return a named string, not the fallback
	for i := range 15 {
		s := RxFilter(i).String()
		require.NotEqual(t, "?", s, "RxFilter(%d) should have a name", i)
	}
	// Out of range returns fallback
	require.Equal(t, "?", RxFilter(99).String())
}
