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
	"net/netip"
	"testing"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/stretchr/testify/require"
)

var best = netip.MustParseAddr("1.1.1.1")
var worse = netip.MustParseAddr("4.4.4.4")

func TestBmcaProperlyUsesClockQuality(t *testing.T) {
	results := map[netip.Addr]*RunResult{
		best: {
			Measurement: &MeasurementResult{Announce: ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 1, GrandmasterClockQuality: ptp.ClockQuality{ClockClass: ptp.ClockClass7}}}},
		},
		worse: {
			Measurement: &MeasurementResult{Announce: ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 2, GrandmasterClockQuality: ptp.ClockQuality{ClockClass: ptp.ClockClass13}}}},
		},
	}
	selected := bmca(results, map[ptp.ClockIdentity]int{1: 2, 2: 1}, DefaultConfig())
	require.Equal(t, results[best].Measurement.Announce, *selected)
}

func TestBmcaProperlyUsesLocalPriority(t *testing.T) {
	results := map[netip.Addr]*RunResult{
		best: {
			Measurement: &MeasurementResult{Announce: ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 1, GrandmasterPriority1: 1}}}, // GrandMasterIdentity is ignored with TelcoDscmp
		},
		worse: {
			Measurement: &MeasurementResult{Announce: ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 2, GrandmasterPriority1: 2}}}, // GrandMasterIdentity is ignored with TelcoDscmp
		},
	}
	selected := bmca(results, map[ptp.ClockIdentity]int{1: 1, 2: 2}, DefaultConfig())
	require.Equal(t, results[best].Measurement.Announce, *selected)
}

func TestBmcaNoMasterForCalibrating(t *testing.T) {
	results := map[netip.Addr]*RunResult{
		best: {
			Measurement: &MeasurementResult{Announce: ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 1, GrandmasterClockQuality: ptp.ClockQuality{ClockClass: ptp.ClockClass13}}}},
		},
		worse: {
			Measurement: &MeasurementResult{Announce: ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 2, GrandmasterClockQuality: ptp.ClockQuality{ClockClass: ptp.ClockClass52}}}},
		},
	}
	selected := bmca(results, map[ptp.ClockIdentity]int{1: 2, 2: 1}, DefaultConfig())
	require.Empty(t, selected)
}

func TestBmcaNoMasterForLowAccuracy(t *testing.T) {
	results := map[netip.Addr]*RunResult{
		best: {
			Measurement: &MeasurementResult{Announce: ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 1, GrandmasterClockQuality: ptp.ClockQuality{ClockAccuracy: ptp.ClockAccuracyMicrosecond100}}}},
		},
		worse: {
			Measurement: &MeasurementResult{Announce: ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 2, GrandmasterClockQuality: ptp.ClockQuality{ClockAccuracy: ptp.ClockAccuracySecond10}}}},
		},
	}
	selected := bmca(results, map[ptp.ClockIdentity]int{1: 2, 2: 1}, DefaultConfig())
	require.Empty(t, selected)
}
