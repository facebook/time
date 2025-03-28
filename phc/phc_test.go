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

package phc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMaxAdjFreq(t *testing.T) {
	var testcases = []struct {
		in   int32
		targ float64
	}{
		{in: 1000000000, targ: 1000000000.0},
		{in: 0, targ: 500000.0},
	}

	for _, tc := range testcases {
		caps := &PtpClockCaps{Max_adj: tc.in}
		got := maxAdj(caps)
		require.InEpsilon(t, tc.targ, got, 0.00001)
	}
}

func TestIfaceToPHCDeviceNotSupported(t *testing.T) {
	dev, err := IfaceToPHCDevice("lo")
	require.Error(t, err)
	require.Equal(t, "", dev)
}

func TestIfaceToPHCDeviceNotFound(t *testing.T) {
	dev, err := IfaceToPHCDevice("lol-does-not-exist")
	require.Error(t, err)
	require.Equal(t, "", dev)
}
