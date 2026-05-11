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

func TestCalculateJitter(t *testing.T) {
	tests := []struct {
		name      string
		maxJitter time.Duration
	}{
		{
			name:      "zero jitter",
			maxJitter: 0,
		},
		{
			name:      "negative jitter",
			maxJitter: -time.Second,
		},
		{
			name:      "positive jitter",
			maxJitter: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jitter := CalculateJitter(tt.maxJitter)

			if tt.maxJitter <= 0 {
				require.Zero(t, jitter)
			} else {
				require.GreaterOrEqual(t, jitter, time.Duration(0))
				require.Less(t, jitter, tt.maxJitter)
			}
		})
	}
}

func TestCalculateJitterDistribution(t *testing.T) {
	maxJitter := 30 * time.Second
	seen := make(map[time.Duration]bool)

	// Run multiple times to verify randomness produces different values
	for range 100 {
		jitter := CalculateJitter(maxJitter)
		require.GreaterOrEqual(t, jitter, time.Duration(0))
		require.Less(t, jitter, maxJitter)
		seen[jitter] = true
	}

	// Verify that different values are produced (randomness check)
	require.Greater(t, len(seen), 1, "CalculateJitter should produce different values across multiple calls")
}

func TestSelectNonLinkLocalAddr(t *testing.T) {
	tests := []struct {
		name    string
		addrs   []net.Addr
		wantIP  net.IP
		wantErr bool
	}{
		{
			name: "global unicast IPv6 present",
			addrs: []net.Addr{
				&net.IPNet{IP: net.ParseIP("2401:db00::1"), Mask: net.CIDRMask(64, 128)},
			},
			wantIP: net.ParseIP("2401:db00::1"),
		},
		{
			name: "only link-local addresses",
			addrs: []net.Addr{
				&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
			},
			wantErr: true,
		},
		{
			name: "only IPv4 addresses",
			addrs: []net.Addr{
				&net.IPNet{IP: net.ParseIP("10.0.0.1"), Mask: net.CIDRMask(24, 32)},
			},
			wantErr: true,
		},
		{
			name: "mixed IPv4 + link-local + global",
			addrs: []net.Addr{
				&net.IPNet{IP: net.ParseIP("10.0.0.1"), Mask: net.CIDRMask(24, 32)},
				&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
				&net.IPNet{IP: net.ParseIP("2401:db00::1"), Mask: net.CIDRMask(64, 128)},
			},
			wantIP: net.ParseIP("2401:db00::1"),
		},
		{
			name:    "empty list",
			addrs:   []net.Addr{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectNonLinkLocalAddr(tt.addrs)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.True(t, tt.wantIP.Equal(got), "expected %s, got %s", tt.wantIP, got)
		})
	}
}
