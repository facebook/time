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

package measurement

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSlidingWindowEmpty(t *testing.T) {
	w := NewWindow(0) // defaults to size 1
	require.True(t, math.IsNaN(w.LastSample()))
	require.True(t, math.IsNaN(w.Mean()))
	require.True(t, math.IsNaN(w.Median()))
	require.Equal(t, 0, len(w.allSamples()))
}

func TestSlidingWindowOne(t *testing.T) {
	w := NewWindow(0) // defaults to size 1
	w.Add(3.14)
	require.InDelta(t, 3.14, w.LastSample(), 0.001)
	require.InDelta(t, 3.14, w.Mean(), 0.001)
	require.InDelta(t, 3.14, w.Median(), 0.001)
	require.Equal(t, 1, len(w.allSamples()))

	w.Add(5.32)
	require.InDelta(t, 5.32, w.LastSample(), 0.001)
	require.InDelta(t, 5.32, w.Mean(), 0.001)
	require.InDelta(t, 5.32, w.Median(), 0.001)
	require.Equal(t, 1, len(w.allSamples()))
}

func TestSlidingWindowMultiple(t *testing.T) {
	w := NewWindow(5)
	w.Add(3.14)
	require.InDelta(t, 3.14, w.LastSample(), 0.001)
	require.InDelta(t, 3.14, w.Mean(), 0.001)
	require.InDelta(t, 3.14, w.Median(), 0.001)
	require.Equal(t, 1, len(w.allSamples()))

	w.Add(5.32)
	require.InDelta(t, 5.32, w.LastSample(), 0.001)
	require.InDelta(t, 4.23, w.Mean(), 0.001)
	require.InDelta(t, 4.23, w.Median(), 0.001)
	require.Equal(t, 2, len(w.allSamples()))

	w.Add(3.17)
	require.InDelta(t, 3.17, w.LastSample(), 0.001)
	require.InDelta(t, 3.876, w.Mean(), 0.001)
	require.InDelta(t, 3.17, w.Median(), 0.001)
	require.Equal(t, 3, len(w.allSamples()))

	w.Add(3.52)
	require.InDelta(t, 3.52, w.LastSample(), 0.001)
	require.InDelta(t, 3.7875, w.Mean(), 0.001)
	require.InDelta(t, 3.3449, w.Median(), 0.001)
	require.Equal(t, 4, len(w.allSamples()))

	w.Add(3.90)
	require.InDelta(t, 3.90, w.LastSample(), 0.001)
	require.InDelta(t, 3.81, w.Mean(), 0.001)
	require.InDelta(t, 3.52, w.Median(), 0.001)
	require.Equal(t, 5, len(w.allSamples()))

	w.Add(3.14) // same as first value, which will be dropped from ring, so nothing changes in aggregates
	require.InDelta(t, 3.14, w.LastSample(), 0.001)
	require.InDelta(t, 3.81, w.Mean(), 0.001)
	require.InDelta(t, 3.52, w.Median(), 0.001)
	require.Equal(t, 5, len(w.allSamples()))

	w.Add(301.90) // crazy big number, median should remain stable
	require.InDelta(t, 301.90, w.LastSample(), 0.001)
	require.InDelta(t, 63.1259, w.Mean(), 0.001)
	require.InDelta(t, 3.52, w.Median(), 0.001)
	require.Equal(t, 5, len(w.allSamples()))
}

func TestSlidingWindowFull(t *testing.T) {
	w := NewWindow(5)
	w.Add(42)
	require.False(t, w.Full())
	w.Add(42)
	require.False(t, w.Full())
	w.Add(42)
	require.False(t, w.Full())
	w.Add(42)
	require.False(t, w.Full())
	w.Add(42)
	require.True(t, w.Full())
	w.Add(42)
	require.True(t, w.Full())
}
