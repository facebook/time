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

package servo

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStateString(t *testing.T) {
	testCases := []struct {
		state State
		want  string
	}{
		{StateInit, "INIT"},
		{StateJump, "JUMP"},
		{StateLocked, "LOCKED"},
		{StateFilter, "FILTER"},
		{StateHoldover, "HOLDOVER"},
		{State(99), "UNSUPPORTED"},
	}
	for _, tc := range testCases {
		t.Run(tc.want, func(t *testing.T) {
			require.Equal(t, tc.want, tc.state.String())
		})
	}
}

func TestGetState(t *testing.T) {
	pi := NewPiServo(DefaultServoConfig(), DefaultPiServoCfg(), -100000.0)
	pi.SyncInterval(1)

	require.Equal(t, StateInit, pi.GetState())

	pi.Sample(100, 1674148530000000000)
	require.Equal(t, StateJump, pi.GetState())

	pi.Sample(50, 1674148531000000000)
	require.Equal(t, StateLocked, pi.GetState())
}

func TestUnlock(t *testing.T) {
	pi := NewPiServo(DefaultServoConfig(), DefaultPiServoCfg(), -100000.0)
	pi.SyncInterval(1)
	// Unlock requires a filter attached to the servo
	filterCfg := DefaultPiServoFilterCfg()
	NewPiServoFilter(pi, filterCfg)

	pi.Sample(100, 1674148530000000000)
	pi.Sample(50, 1674148531000000000)
	require.Equal(t, StateLocked, pi.GetState())

	pi.Unlock()
	require.Equal(t, StateInit, pi.GetState())
}

func TestInitLastFreq(t *testing.T) {
	pi := NewPiServo(DefaultServoConfig(), DefaultPiServoCfg(), 0)
	pi.InitLastFreq(-50000.0)
	require.Equal(t, -50000.0, pi.lastFreq)
	require.Equal(t, -50000.0, pi.drift)
}

func TestSetMaxFreqClampsOutput(t *testing.T) {
	pi := NewPiServo(DefaultServoConfig(), DefaultPiServoCfg(), 0)
	pi.SetMaxFreq(100.0)
	pi.SyncInterval(1)

	pi.Sample(1000000, 1674148530000000000)
	freq, _ := pi.Sample(1000000, 1674148531000000000)
	// frequency should be clamped to maxFreq
	require.LessOrEqual(t, freq, 100.0)
	require.GreaterOrEqual(t, freq, -100.0)
}
