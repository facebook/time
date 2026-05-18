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
	result := fmtThreshold(100)
	require.Contains(t, result, "100")
}

func TestTxTypeString(t *testing.T) {
	require.Equal(t, "off", TxType(0).String())
	require.Equal(t, "on", TxType(1).String())
	require.Equal(t, "stepsync", TxType(2).String())
	require.Equal(t, "?", TxType(99).String())
}

func TestRxFilterString(t *testing.T) {
	require.Equal(t, "none", RxFilter(0).String())
	require.Equal(t, "all", RxFilter(1).String())
	require.Equal(t, "some", RxFilter(2).String())
	require.Equal(t, "?", RxFilter(99).String())
}
