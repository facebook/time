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

package clock

import (
	"testing"
	"time"

	osc "github.com/facebook/time/oscillatord"
	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/stretchr/testify/require"
)

func TestOscillatorStateFromStatus(t *testing.T) {
	status := &osc.Status{
		Oscillator: osc.Oscillator{
			Lock: true,
		},
		Clock: osc.Clock{
			Class:  osc.ClockClass(ptp.ClockClass6),
			Offset: 42 * time.Nanosecond,
		},
	}
	expectedLock := &oscillatorState{
		ClockClass: ClockClassLock,
		Offset:     42 * time.Nanosecond,
	}
	expectedHoldover := &oscillatorState{
		ClockClass: ClockClassHoldover,
		Offset:     42 * time.Nanosecond,
	}
	expectedCalibrating := &oscillatorState{
		ClockClass: ClockClassCalibrating,
		Offset:     42 * time.Nanosecond,
	}
	expectedUncalibrated := &oscillatorState{
		ClockClass: ClockClassUncalibrated,
		Offset:     42 * time.Nanosecond,
	}
	expectedFailed := &oscillatorState{
		ClockClass: ClockClassUncalibrated,
		Offset:     0,
	}

	require.Equal(t, expectedLock, oscillatorStateFromStatus(status))

	status.Clock.Class = osc.ClockClass(ptp.ClockClass7)
	require.Equal(t, expectedHoldover, oscillatorStateFromStatus(status))

	status.Clock.Class = osc.ClockClass(ptp.ClockClass13)
	require.Equal(t, expectedCalibrating, oscillatorStateFromStatus(status))

	status.Clock.Class = osc.ClockClass(ptp.ClockClass52)
	require.Equal(t, expectedUncalibrated, oscillatorStateFromStatus(status))

	status.Oscillator.Lock = false
	require.Equal(t, expectedFailed, oscillatorStateFromStatus(status))
}

func TestOscillatord(t *testing.T) {
	status, err := oscillatord()
	require.Error(t, err)
	require.Nil(t, status)
}
