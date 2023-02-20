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

package bmc

import (
	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDscmp2(t *testing.T) {
	pi1 := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: 5212879185253000328,
	}
	pi2 := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: 0,
	}
	a1 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{StepsRemoved: 1}, Header: ptp.Header{SourcePortIdentity: pi1}}
	a2 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{StepsRemoved: 3}, Header: ptp.Header{SourcePortIdentity: pi1}}
	a3 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{StepsRemoved: 1}, Header: ptp.Header{SourcePortIdentity: pi2}}
	require.Equal(t, Dscmp2(&a1, &a1), Unknown)
	require.Equal(t, Dscmp2(&a1, &a2), ABetter)
	require.Equal(t, Dscmp2(&a1, &a3), BBetterTopo)
}

func TestDscmp(t *testing.T) {
	pi1 := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: 5212879185253000328,
	}
	pi2 := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: 0,
	}
	a1 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{StepsRemoved: 1}, Header: ptp.Header{SourcePortIdentity: pi1}}
	a2 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{StepsRemoved: 1}, Header: ptp.Header{SourcePortIdentity: pi2}}
	a3 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 1, GrandmasterPriority1: 1}}
	a4 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 2, GrandmasterPriority1: 2}}
	a5 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 1, GrandmasterClockQuality: ptp.ClockQuality{ClockClass: ptp.ClockClass7}}}
	a6 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 2, GrandmasterClockQuality: ptp.ClockQuality{ClockClass: ptp.ClockClass13}}}
	a7 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 1, GrandmasterClockQuality: ptp.ClockQuality{ClockAccuracy: 42}}}
	a8 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 2, GrandmasterClockQuality: ptp.ClockQuality{ClockAccuracy: 69}}}
	a9 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 1, GrandmasterClockQuality: ptp.ClockQuality{OffsetScaledLogVariance: 42}}}
	a10 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 2, GrandmasterClockQuality: ptp.ClockQuality{OffsetScaledLogVariance: 69}}}
	a11 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 1, GrandmasterPriority2: 1}}
	a12 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 2, GrandmasterPriority2: 2}}
	a13 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 1}}
	a14 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 2}}
	require.Equal(t, Dscmp(&a1, &a2), Unknown)
	require.Equal(t, Dscmp(&a3, &a4), ABetter)
	require.Equal(t, Dscmp(&a4, &a3), BBetter)
	require.Equal(t, Dscmp(&a5, &a6), ABetter)
	require.Equal(t, Dscmp(&a6, &a5), BBetter)
	require.Equal(t, Dscmp(&a7, &a8), ABetter)
	require.Equal(t, Dscmp(&a8, &a7), BBetter)
	require.Equal(t, Dscmp(&a9, &a10), ABetter)
	require.Equal(t, Dscmp(&a10, &a9), BBetter)
	require.Equal(t, Dscmp(&a11, &a12), ABetter)
	require.Equal(t, Dscmp(&a12, &a11), BBetter)
	require.Equal(t, Dscmp(&a13, &a14), ABetter)
	require.Equal(t, Dscmp(&a14, &a13), BBetter)
}

func TestTelcoDscmp(t *testing.T) {
	pi1 := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: 5212879185253000328,
	}
	pi2 := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: 0,
	}
	a1 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{StepsRemoved: 1}, Header: ptp.Header{SourcePortIdentity: pi1}}
	a2 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{StepsRemoved: 1}, Header: ptp.Header{SourcePortIdentity: pi2}}
	a3 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 1, GrandmasterClockQuality: ptp.ClockQuality{ClockClass: ptp.ClockClass7}}}
	a4 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 2, GrandmasterClockQuality: ptp.ClockQuality{ClockClass: ptp.ClockClass13}}}
	a5 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 1, GrandmasterClockQuality: ptp.ClockQuality{ClockAccuracy: 42}}}
	a6 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 2, GrandmasterClockQuality: ptp.ClockQuality{ClockAccuracy: 69}}}
	a7 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 1, GrandmasterClockQuality: ptp.ClockQuality{OffsetScaledLogVariance: 42}}}
	a8 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 2, GrandmasterClockQuality: ptp.ClockQuality{OffsetScaledLogVariance: 69}}}
	a9 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 1, GrandmasterPriority2: 1}}
	a10 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 2, GrandmasterPriority2: 2}}
	a11 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 1, GrandmasterClockQuality: ptp.ClockQuality{ClockClass: 128}}}
	a12 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 2, GrandmasterClockQuality: ptp.ClockQuality{ClockClass: 128}}}
	lp1 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 1, GrandmasterPriority1: 1}}
	lp2 := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 2, GrandmasterPriority1: 2}}
	require.Equal(t, TelcoDscmp(&a1, &a2, 1, 2), Unknown)
	require.Equal(t, TelcoDscmp(&a3, &a4, 1, 2), ABetter)
	require.Equal(t, TelcoDscmp(&a4, &a3, 1, 2), BBetter)
	require.Equal(t, TelcoDscmp(&a5, &a6, 1, 2), ABetter)
	require.Equal(t, TelcoDscmp(&a6, &a5, 1, 2), BBetter)
	require.Equal(t, TelcoDscmp(&a7, &a8, 1, 2), ABetter)
	require.Equal(t, TelcoDscmp(&a8, &a7, 1, 2), BBetter)
	require.Equal(t, TelcoDscmp(&a9, &a10, 1, 2), ABetter)
	require.Equal(t, TelcoDscmp(&a10, &a9, 1, 2), BBetter)
	require.Equal(t, TelcoDscmp(&a11, &a12, 1, 1), ABetter)
	require.Equal(t, TelcoDscmp(&a12, &a11, 1, 1), BBetter)
	require.Equal(t, TelcoDscmp(&lp1, &lp2, 1, 2), ABetter)
	require.Equal(t, TelcoDscmp(&lp1, &lp2, 2, 1), BBetter)
}
