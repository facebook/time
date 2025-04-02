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
	"net/netip"
	"testing"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/stretchr/testify/require"
)

func TestIsAsymmetric(t *testing.T) {
	tests := []struct {
		name               string
		result             *RunResult
		asymmetryThreshold time.Duration
		expected           bool
	}{
		{
			name: "Asymmetric path with ptp.ClockClass6 and offset greater than threshold",
			result: &RunResult{
				Measurement: &MeasurementResult{
					Offset: 200 * time.Nanosecond,
					Announce: ptp.Announce{
						AnnounceBody: ptp.AnnounceBody{
							GrandmasterClockQuality: ptp.ClockQuality{
								ClockClass: ptp.ClockClass6,
							},
						},
					},
				},
			},
			asymmetryThreshold: 100 * time.Nanosecond,
			expected:           true,
		},
		{
			name: "Symmetric path with ptp.ClockClass6 and offset less than threshold",
			result: &RunResult{
				Measurement: &MeasurementResult{
					Offset: 50 * time.Nanosecond,
					Announce: ptp.Announce{
						AnnounceBody: ptp.AnnounceBody{
							GrandmasterClockQuality: ptp.ClockQuality{
								ClockClass: ptp.ClockClass6,
							},
						},
					},
				},
			},
			asymmetryThreshold: 100 * time.Nanosecond,
			expected:           false,
		},
		{
			name: "result with clock class != ptp.ClockClass6",
			result: &RunResult{
				Measurement: &MeasurementResult{
					Offset: 200 * time.Nanosecond,
					Announce: ptp.Announce{
						AnnounceBody: ptp.AnnounceBody{
							GrandmasterClockQuality: ptp.ClockQuality{
								ClockClass: 5,
							},
						},
					},
				},
			},
			asymmetryThreshold: 100 * time.Nanosecond,
			expected:           false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := isAsymmetric(tt.result, tt.asymmetryThreshold)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetAlternateResponsePortTLV(t *testing.T) {
	tests := []struct {
		name     string
		client   *Client
		expected *ptp.AlternateResponsePortTLV
	}{
		{
			name: "Contains AlternateResponsePortTLV",
			client: &Client{
				delayRequest: &ptp.SyncDelayReq{
					TLVs: []ptp.TLV{
						&ptp.AlternateResponsePortTLV{Offset: 1},
					},
				},
			},
			expected: &ptp.AlternateResponsePortTLV{Offset: 1},
		},
		{
			name: "Does not contain AlternateResponsePortTLV",
			client: &Client{
				delayRequest: &ptp.SyncDelayReq{
					TLVs: []ptp.TLV{
						// Some other TLV type
						ptp.AcknowledgeCancelUnicastTransmissionTLV{},
					},
				},
			},
			expected: nil,
		},
		{
			name: "Nil delayRequest",
			client: &Client{
				delayRequest: nil,
			},
			expected: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fmt.Print(tt.name)
			actual := getAlternateResponsePortTLV(tt.client)
			if tt.expected == nil {
				require.Nil(t, actual)
			} else {
				require.Equal(t, tt.expected.Offset, actual.Offset)
			}
		})
	}
}

func TestSelectedGMAsymmetric(t *testing.T) {
	tests := []struct {
		name     string
		clients  map[netip.Addr]*Client
		config   AsymmetryConfig
		expected bool
	}{
		{
			name: "One client has offset larger than limit",
			clients: map[netip.Addr]*Client{
				netip.MustParseAddr("192.0.2.1"): {
					asymmetric: true,
					delayRequest: &ptp.SyncDelayReq{
						TLVs: []ptp.TLV{
							&ptp.AlternateResponsePortTLV{Offset: 10},
						},
					},
				},
				netip.MustParseAddr("192.0.2.2"): {
					asymmetric: true,
					delayRequest: &ptp.SyncDelayReq{
						TLVs: []ptp.TLV{
							&ptp.AlternateResponsePortTLV{Offset: 5},
						},
					},
				},
			},
			config:   AsymmetryConfig{MaxPortChanges: 8},
			expected: true,
		},
		{
			name: "No client has offset larger than limit",
			clients: map[netip.Addr]*Client{
				netip.MustParseAddr("192.0.2.1"): {
					asymmetric: true,
					delayRequest: &ptp.SyncDelayReq{
						TLVs: []ptp.TLV{
							&ptp.AlternateResponsePortTLV{Offset: 10},
						},
					},
				},
				netip.MustParseAddr("192.0.2.2"): {
					asymmetric: true,
					delayRequest: &ptp.SyncDelayReq{
						TLVs: []ptp.TLV{
							&ptp.AlternateResponsePortTLV{Offset: 5},
						},
					},
				},
			},
			config:   AsymmetryConfig{MaxPortChanges: 11},
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := selectedGMAsymmetric(tt.clients, tt.config)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func TestCorrectSelectedGMAsymmetry(t *testing.T) {
	// Setup test data
	bestAddr := netip.MustParseAddr("192.0.2.1")
	otherAddr := netip.MustParseAddr("192.0.2.2")
	bestClient := &Client{
		delayRequest: &ptp.SyncDelayReq{
			TLVs: []ptp.TLV{
				&ptp.AlternateResponsePortTLV{Offset: 5},
			},
		},
		asymmetric:       false,
		asymmetryCounter: 2,
	}
	otherClient := &Client{
		delayRequest: &ptp.SyncDelayReq{
			TLVs: []ptp.TLV{
				&ptp.AlternateResponsePortTLV{Offset: 3},
			},
		},
		asymmetric:       true,
		asymmetryCounter: 5,
	}
	clients := map[netip.Addr]*Client{
		bestAddr:  bestClient,
		otherAddr: otherClient,
	}
	// Act
	correctSelectedGMAsymmetry(clients, bestAddr)
	// Assertions
	require.Equal(t, uint16(6), bestClient.delayRequest.TLVs[0].(*ptp.AlternateResponsePortTLV).Offset, "Best GM offset should be incremented")
	require.True(t, bestClient.asymmetric, "Best GM should be marked as asymmetric")
	require.Equal(t, uint16(0), otherClient.delayRequest.TLVs[0].(*ptp.AlternateResponsePortTLV).Offset, "Other GM offset should be reset to 0")
	require.False(t, otherClient.asymmetric, "Other GM should not be marked as asymmetric")
	require.Equal(t, uint16(0), otherClient.asymmetryCounter, "Other GM asymmetry grace should be reset to 0")
}

func TestCorrectNonSelectedGMsAsymmetry(t *testing.T) {
	bestAddr := netip.MustParseAddr("192.0.2.2")
	config := AsymmetryConfig{
		AsymmetryThreshold:      10 * time.Millisecond,
		MaxConsecutiveAsymmetry: 2,
	}
	clientIP := netip.MustParseAddr("192.0.2.1")
	tests := []struct {
		name           string
		clients        map[netip.Addr]*Client
		results        map[netip.Addr]*RunResult
		config         AsymmetryConfig
		expectedOffset uint16
		expectedCount  int
	}{
		{
			name: "Result offset higher than threshold but asymmetryCounter <= MaxConsecutiveAsymmetry",
			clients: map[netip.Addr]*Client{
				clientIP: {
					asymmetric:       false,
					asymmetryCounter: 1,
					delayRequest: &ptp.SyncDelayReq{
						TLVs: []ptp.TLV{
							&ptp.AlternateResponsePortTLV{Offset: 0},
						},
					},
				},
			},
			results: map[netip.Addr]*RunResult{
				clientIP: {
					Server: clientIP,
					Measurement: &MeasurementResult{
						Announce: ptp.Announce{
							AnnounceBody: ptp.AnnounceBody{
								GrandmasterClockQuality: ptp.ClockQuality{
									ClockClass: ptp.ClockClass6,
								},
							},
						},
						Offset: 15 * time.Millisecond,
					},
				},
			},
			expectedOffset: 0, // Offset should not increase
		},
		{
			name: "Result offset higher than threshold and asymmetryCounter > MaxConsecutiveAsymmetry",
			clients: map[netip.Addr]*Client{
				clientIP: {
					asymmetric:       false,
					asymmetryCounter: 3,
					delayRequest: &ptp.SyncDelayReq{
						TLVs: []ptp.TLV{
							&ptp.AlternateResponsePortTLV{Offset: 0},
						},
					},
				},
			},
			results: map[netip.Addr]*RunResult{
				clientIP: {
					Server: clientIP,
					Measurement: &MeasurementResult{
						Announce: ptp.Announce{
							AnnounceBody: ptp.AnnounceBody{
								GrandmasterClockQuality: ptp.ClockQuality{
									ClockClass: ptp.ClockClass6,
								},
							},
						},
						Offset: 15 * time.Millisecond,
					},
				},
			},
			expectedOffset: 1, // Offset should increase
		},
		{
			name: "Result offset lower than threshold",
			clients: map[netip.Addr]*Client{
				clientIP: {
					asymmetric:       false,
					asymmetryCounter: 1,
					delayRequest: &ptp.SyncDelayReq{
						TLVs: []ptp.TLV{
							&ptp.AlternateResponsePortTLV{Offset: 0},
						},
					},
				},
			},
			results: map[netip.Addr]*RunResult{
				clientIP: {
					Server: clientIP,
					Measurement: &MeasurementResult{
						Announce: ptp.Announce{
							AnnounceBody: ptp.AnnounceBody{
								GrandmasterClockQuality: ptp.ClockQuality{
									ClockClass: ptp.ClockClass6,
								},
							},
						},
						Offset: 5 * time.Millisecond,
					},
				},
			},
			expectedOffset: 0, // Offset should not increase
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			correctNonSelectedGMsAsymmetry(tt.clients, tt.results, bestAddr, config)
			client := tt.clients[clientIP]
			alternateResponsePortTlv := getAlternateResponsePortTLV(client)
			require.NotNil(t, alternateResponsePortTlv, "AlternateResponsePortTLV should not be nil")
			require.Equal(t, tt.expectedOffset, alternateResponsePortTlv.Offset, "Offset should match expected value")
		})
	}
}

func TestSimpleSelectedGMAsymmetric(t *testing.T) {
	bestAddr := netip.MustParseAddr("192.0.2.1")
	tests := []struct {
		name     string
		clients  map[netip.Addr]*Client
		results  map[netip.Addr]*RunResult
		config   AsymmetryConfig
		expected bool
	}{
		{
			name: "Selected GM is not asymmetric before max consecutive asymmetry",
			clients: map[netip.Addr]*Client{
				bestAddr: {asymmetryCounter: 0},
			},
			results: map[netip.Addr]*RunResult{
				bestAddr: {Measurement: &MeasurementResult{
					Announce: ptp.Announce{
						AnnounceBody: ptp.AnnounceBody{
							GrandmasterClockQuality: ptp.ClockQuality{
								ClockClass: ptp.ClockClass6,
							},
						},
					},
					Offset: 15 * time.Millisecond}},
				netip.MustParseAddr("192.0.2.2"): {Measurement: &MeasurementResult{
					Announce: ptp.Announce{
						AnnounceBody: ptp.AnnounceBody{
							GrandmasterClockQuality: ptp.ClockQuality{
								ClockClass: ptp.ClockClass6,
							},
						},
					},
					Offset: 20 * time.Millisecond}},
				netip.MustParseAddr("192.0.2.3"): {Measurement: &MeasurementResult{
					Announce: ptp.Announce{
						AnnounceBody: ptp.AnnounceBody{
							GrandmasterClockQuality: ptp.ClockQuality{
								ClockClass: ptp.ClockClass6,
							},
						},
					},
					Offset: 11 * time.Millisecond}},
			},
			config:   AsymmetryConfig{AsymmetryThreshold: 10 * time.Millisecond, MaxConsecutiveAsymmetry: 2},
			expected: false,
		},
		{
			name: "Selected GM is asymmetric if all others are",
			clients: map[netip.Addr]*Client{
				bestAddr: {asymmetryCounter: 3},
			},
			results: map[netip.Addr]*RunResult{
				bestAddr: {Measurement: &MeasurementResult{
					Announce: ptp.Announce{
						AnnounceBody: ptp.AnnounceBody{
							GrandmasterClockQuality: ptp.ClockQuality{
								ClockClass: ptp.ClockClass6,
							},
						},
					},
					Offset: 15 * time.Millisecond}},
				netip.MustParseAddr("192.0.2.2"): {Measurement: &MeasurementResult{
					Announce: ptp.Announce{
						AnnounceBody: ptp.AnnounceBody{
							GrandmasterClockQuality: ptp.ClockQuality{
								ClockClass: ptp.ClockClass6,
							},
						},
					},
					Offset: 20 * time.Millisecond}},
				netip.MustParseAddr("192.0.2.3"): {Measurement: &MeasurementResult{
					Announce: ptp.Announce{
						AnnounceBody: ptp.AnnounceBody{
							GrandmasterClockQuality: ptp.ClockQuality{
								ClockClass: ptp.ClockClass6,
							},
						},
					},
					Offset: 11 * time.Millisecond}},
			},
			config:   AsymmetryConfig{AsymmetryThreshold: 10 * time.Millisecond, MaxConsecutiveAsymmetry: 2},
			expected: true,
		},
		{
			name: "If at least one other GM is not asymmetric, selected GM is not asymmetric",
			clients: map[netip.Addr]*Client{
				bestAddr: {asymmetryCounter: 3},
			},
			results: map[netip.Addr]*RunResult{
				bestAddr: {Measurement: &MeasurementResult{
					Announce: ptp.Announce{
						AnnounceBody: ptp.AnnounceBody{
							GrandmasterClockQuality: ptp.ClockQuality{
								ClockClass: ptp.ClockClass6,
							},
						},
					},
					Offset: 5 * time.Millisecond}},
				netip.MustParseAddr("192.0.2.2"): {Measurement: &MeasurementResult{
					Announce: ptp.Announce{
						AnnounceBody: ptp.AnnounceBody{
							GrandmasterClockQuality: ptp.ClockQuality{
								ClockClass: ptp.ClockClass6,
							},
						},
					},
					Offset: 20 * time.Millisecond}},
				netip.MustParseAddr("192.0.2.3"): {Measurement: &MeasurementResult{
					Announce: ptp.Announce{
						AnnounceBody: ptp.AnnounceBody{
							GrandmasterClockQuality: ptp.ClockQuality{
								ClockClass: ptp.ClockClass6,
							},
						},
					},
					Offset: 9 * time.Millisecond}},
			},
			config:   AsymmetryConfig{AsymmetryThreshold: 10 * time.Millisecond, MaxConsecutiveAsymmetry: 2},
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := simpleSelectedGMAsymmetric(tt.clients, tt.results, bestAddr, tt.config)
			require.Equal(t, tt.expected, actual)
		})
	}
}
