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
	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBmcaProperlyUsesClockQuality(t *testing.T) {
	best := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 1, GrandmasterClockQuality: ptp.ClockQuality{ClockClass: ptp.ClockClass7}}}
	worse := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 2, GrandmasterClockQuality: ptp.ClockQuality{ClockClass: ptp.ClockClass13}}}
	selected := bmca([]*ptp.Announce{&best, &worse}, map[ptp.ClockIdentity]int{1: 2, 2: 1})
	require.Equal(t, best, *selected)
}

func TestBmcaProperlyUsesLocalPriority(t *testing.T) {
	best := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 1, GrandmasterPriority1: 1}}  // GrandMasterIdentity is ignored with TelcoDscmp
	worse := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 2, GrandmasterPriority1: 2}} // GrandMasterIdentity is ignored with TelcoDscmp
	selected := bmca([]*ptp.Announce{&best, &worse}, map[ptp.ClockIdentity]int{1: 1, 2: 2})
	require.Equal(t, best, *selected)
}

func TestBmcaNoMasterForCalibrating(t *testing.T) {
	best := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 1, GrandmasterClockQuality: ptp.ClockQuality{ClockClass: ptp.ClockClass52}}}
	worse := ptp.Announce{AnnounceBody: ptp.AnnounceBody{GrandmasterIdentity: 2, GrandmasterClockQuality: ptp.ClockQuality{ClockClass: ptp.ClockClass13}}}
	selected := bmca([]*ptp.Announce{&best, &worse}, map[ptp.ClockIdentity]int{1: 2, 2: 1})
	require.Empty(t, selected)
}
