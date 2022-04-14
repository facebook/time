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
	"github.com/stretchr/testify/require"
)

func TestClockQualityFromOscillatord(t *testing.T) {
	status := &osc.Status{
		GNSS: osc.GNSS{
			FixOK: true,
		},
		Oscillator: osc.Oscillator{
			Lock: true,
		},
	}
	expectedLock := &oscillatorState{
		ClockClass: ClockClassLocked,
		Offset:     100 * time.Nanosecond,
	}
	expectedHoldover := &oscillatorState{
		ClockClass: ClockClassHoldover,
		Offset:     time.Microsecond,
	}
	expectedFailed := &oscillatorState{
		ClockClass: ClockClassUncalibrated,
		Offset:     0,
	}
	require.Equal(t, expectedLock, clockQualityFromOscillatord(status))
	status.GNSS.FixOK = false
	require.Equal(t, expectedHoldover, clockQualityFromOscillatord(status))
	status.Oscillator.Lock = false
	require.Equal(t, expectedFailed, clockQualityFromOscillatord(status))
}

func TestOscillatord(t *testing.T) {
	status, err := oscillatord()
	require.Error(t, err)
	require.Nil(t, status)
}
