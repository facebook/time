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

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/stretchr/testify/require"
)

func TestWorst(t *testing.T) {
	aexpr := "max(abs(mean(phcoffset)) + 1 * stddev(phcoffset), abs(mean(oscillatoroffset)))"
	cexpr := "p99(oscillatorclass)"
	expected := &ptp.ClockQuality{ClockClass: ptp.ClockClass6, ClockAccuracy: ptp.ClockAccuracyMicrosecond1}

	clocks := []*DataPoint{
		&DataPoint{
			PHCOffset:            100 * time.Nanosecond,
			OscillatorOffset:     100 * time.Nanosecond,
			OscillatorClockClass: ClockClassLock,
		},
		&DataPoint{
			PHCOffset:            time.Microsecond,
			OscillatorOffset:     100 * time.Nanosecond,
			OscillatorClockClass: ClockClassLock,
		},
		&DataPoint{
			PHCOffset:            250 * time.Nanosecond,
			OscillatorOffset:     100 * time.Nanosecond,
			OscillatorClockClass: ClockClassLock,
		},
	}

	w, err := Worst(clocks, aexpr, cexpr)
	require.NoError(t, err)
	require.Equal(t, expected, w)

	expected = &ptp.ClockQuality{ClockClass: ptp.ClockClass7, ClockAccuracy: ptp.ClockAccuracyMicrosecond25}
	clocks = []*DataPoint{
		&DataPoint{
			PHCOffset:            12 * time.Microsecond,
			OscillatorOffset:     time.Microsecond,
			OscillatorClockClass: ClockClassHoldover,
		},
		&DataPoint{
			PHCOffset:            10 * time.Nanosecond,
			OscillatorOffset:     100 * time.Nanosecond,
			OscillatorClockClass: ClockClassLock,
		},
		nil,
	}

	w, err = Worst(clocks, aexpr, cexpr)
	require.NoError(t, err)
	require.Equal(t, expected, w)

	clocks = []*DataPoint{nil, nil}

	w, err = Worst(clocks, aexpr, cexpr)
	require.NoError(t, err)
	require.Nil(t, w)
}

func TestWorstBig(t *testing.T) {
	// p68 for normal distribution, see three-sigma rule of thumb
	aexpr := "abs(mean(phcoffset)) + stddev(phcoffset)"
	cexpr := "p99(oscillatorclass)"
	expected := &ptp.ClockQuality{ClockClass: ptp.ClockClass6, ClockAccuracy: ptp.ClockAccuracyNanosecond100}

	clocks := []*DataPoint{}
	for i := 0; i < 594; i++ {
		clocks = append(clocks, &DataPoint{OscillatorClockClass: ptp.ClockClass6, PHCOffset: 80 * time.Nanosecond})
	}
	for i := 0; i < 6; i++ {
		clocks = append(clocks, &DataPoint{OscillatorClockClass: ptp.ClockClass7, PHCOffset: 250 * time.Nanosecond})
	}

	w, err := Worst(clocks, aexpr, cexpr)
	require.NoError(t, err)
	require.Equal(t, expected, w)

	// Changing 1 element to sway over the border
	clocks[592] = &DataPoint{OscillatorClockClass: ptp.ClockClass7, PHCOffset: 250 * time.Nanosecond}
	expected = &ptp.ClockQuality{ClockClass: ptp.ClockClass7, ClockAccuracy: ptp.ClockAccuracyNanosecond100}
	w, err = Worst(clocks, aexpr, cexpr)
	require.NoError(t, err)
	require.Equal(t, expected, w)
}

func TestBufferRing(t *testing.T) {
	sample := 2
	rb := NewRingBuffer(sample)
	require.Equal(t, sample, rb.size)
	cc100 := &DataPoint{PHCOffset: 100 * time.Nanosecond, OscillatorOffset: 100 * time.Nanosecond, OscillatorClockClass: ClockClassLock}
	cc250 := &DataPoint{PHCOffset: 250 * time.Nanosecond, OscillatorOffset: 250 * time.Nanosecond, OscillatorClockClass: ClockClassCalibrating}
	cc1u := &DataPoint{PHCOffset: time.Microsecond, OscillatorOffset: time.Microsecond, OscillatorClockClass: ClockClassHoldover}
	// Write 1
	rb.Write(cc100)
	require.Equal(t, 1, rb.index)
	require.Equal(t, []*DataPoint{cc100, nil}, rb.Data())

	// Write 2
	rb.Write(cc250)
	require.Equal(t, 2, rb.index)
	require.Equal(t, []*DataPoint{cc100, cc250}, rb.Data())

	// Write 3
	rb.Write(cc1u)
	require.Equal(t, 1, rb.index)
	require.Equal(t, []*DataPoint{cc1u, cc250}, rb.Data())

	// Write 4
	rb.Write(nil)
	require.Equal(t, 2, rb.index)
	require.Equal(t, []*DataPoint{cc1u, nil}, rb.Data())

	// Write 5
	rb.Write(nil)
	require.Equal(t, 1, rb.index)
	require.Equal(t, []*DataPoint{nil, nil}, rb.Data())
}
