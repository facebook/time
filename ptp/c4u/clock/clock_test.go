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

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/stretchr/testify/require"
)

func TestWorst(t *testing.T) {
	expected := &ptp.ClockQuality{ClockClass: ptp.ClockClass13, ClockAccuracy: ptp.ClockAccuracyMicrosecond1}

	clocks := []*ptp.ClockQuality{
		&ptp.ClockQuality{ClockClass: ptp.ClockClass6, ClockAccuracy: ptp.ClockAccuracyNanosecond100},
		&ptp.ClockQuality{ClockClass: ptp.ClockClass7, ClockAccuracy: ptp.ClockAccuracyMicrosecond1},
		&ptp.ClockQuality{ClockClass: ptp.ClockClass13, ClockAccuracy: ptp.ClockAccuracyNanosecond250},
	}

	w := Worst(clocks)
	require.Equal(t, expected, w)

	clocks = []*ptp.ClockQuality{
		&ptp.ClockQuality{ClockClass: ptp.ClockClass13, ClockAccuracy: ptp.ClockAccuracyMicrosecond1},
		nil,
	}

	w = Worst(clocks)
	require.Equal(t, expected, w)

	clocks = []*ptp.ClockQuality{nil, nil}

	w = Worst(clocks)
	require.Nil(t, w)
}

func TestBufferRing(t *testing.T) {
	sample := 2
	rb := NewRingBuffer(sample)
	require.Equal(t, sample, rb.size)
	// Write 1
	rb.Write(&ptp.ClockQuality{ClockClass: ptp.ClockClass6, ClockAccuracy: ptp.ClockAccuracyNanosecond100})
	require.Equal(t, 1, rb.index)
	require.Equal(t, []*ptp.ClockQuality{&ptp.ClockQuality{ClockClass: ptp.ClockClass6, ClockAccuracy: ptp.ClockAccuracyNanosecond100}, nil}, rb.Data())

	// Write 2
	rb.Write(&ptp.ClockQuality{ClockClass: ptp.ClockClass7, ClockAccuracy: ptp.ClockAccuracyNanosecond250})
	require.Equal(t, 2, rb.index)
	require.Equal(t, []*ptp.ClockQuality{&ptp.ClockQuality{ClockClass: ptp.ClockClass6, ClockAccuracy: ptp.ClockAccuracyNanosecond100}, &ptp.ClockQuality{ClockClass: ptp.ClockClass7, ClockAccuracy: ptp.ClockAccuracyNanosecond250}}, rb.Data())

	// Write 3
	rb.Write(&ptp.ClockQuality{ClockClass: ptp.ClockClass13, ClockAccuracy: ptp.ClockAccuracyMicrosecond1})
	require.Equal(t, 1, rb.index)
	require.Equal(t, []*ptp.ClockQuality{&ptp.ClockQuality{ClockClass: ptp.ClockClass13, ClockAccuracy: ptp.ClockAccuracyMicrosecond1}, &ptp.ClockQuality{ClockClass: ptp.ClockClass7, ClockAccuracy: ptp.ClockAccuracyNanosecond250}}, rb.Data())

	// Write 4
	rb.Write(nil)
	require.Equal(t, 2, rb.index)
	require.Equal(t, []*ptp.ClockQuality{&ptp.ClockQuality{ClockClass: ptp.ClockClass13, ClockAccuracy: ptp.ClockAccuracyMicrosecond1}, nil}, rb.Data())

	// Write 5
	rb.Write(nil)
	require.Equal(t, 1, rb.index)
	require.Equal(t, []*ptp.ClockQuality{nil, nil}, rb.Data())
}
