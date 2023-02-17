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
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSlidingWindowEmpty(t *testing.T) {
	w := newSlidingWindow(0) // defaults to size 1
	require.True(t, math.IsNaN(w.lastSample()))
	require.True(t, math.IsNaN(w.mean()))
	require.True(t, math.IsNaN(w.median()))
	require.Equal(t, 0, len(w.allSamples()))
}

func TestSlidingWindowOne(t *testing.T) {
	w := newSlidingWindow(0) // defaults to size 1
	w.add(3.14)
	require.InDelta(t, 3.14, w.lastSample(), 0.001)
	require.InDelta(t, 3.14, w.mean(), 0.001)
	require.InDelta(t, 3.14, w.median(), 0.001)
	require.Equal(t, 1, len(w.allSamples()))

	w.add(5.32)
	require.InDelta(t, 5.32, w.lastSample(), 0.001)
	require.InDelta(t, 5.32, w.mean(), 0.001)
	require.InDelta(t, 5.32, w.median(), 0.001)
	require.Equal(t, 1, len(w.allSamples()))
}

func TestSlidingWindowMultiple(t *testing.T) {
	w := newSlidingWindow(5)
	w.add(3.14)
	require.InDelta(t, 3.14, w.lastSample(), 0.001)
	require.InDelta(t, 3.14, w.mean(), 0.001)
	require.InDelta(t, 3.14, w.median(), 0.001)
	require.Equal(t, 1, len(w.allSamples()))

	w.add(5.32)
	require.InDelta(t, 5.32, w.lastSample(), 0.001)
	require.InDelta(t, 4.23, w.mean(), 0.001)
	require.InDelta(t, 4.23, w.median(), 0.001)
	require.Equal(t, 2, len(w.allSamples()))

	w.add(3.17)
	require.InDelta(t, 3.17, w.lastSample(), 0.001)
	require.InDelta(t, 3.876, w.mean(), 0.001)
	require.InDelta(t, 3.17, w.median(), 0.001)
	require.Equal(t, 3, len(w.allSamples()))

	w.add(3.52)
	require.InDelta(t, 3.52, w.lastSample(), 0.001)
	require.InDelta(t, 3.7875, w.mean(), 0.001)
	require.InDelta(t, 3.3449, w.median(), 0.001)
	require.Equal(t, 4, len(w.allSamples()))

	w.add(3.90)
	require.InDelta(t, 3.90, w.lastSample(), 0.001)
	require.InDelta(t, 3.81, w.mean(), 0.001)
	require.InDelta(t, 3.52, w.median(), 0.001)
	require.Equal(t, 5, len(w.allSamples()))

	w.add(3.14) // same as first value, which will be dropped from ring, so nothing changes in aggregates
	require.InDelta(t, 3.14, w.lastSample(), 0.001)
	require.InDelta(t, 3.81, w.mean(), 0.001)
	require.InDelta(t, 3.52, w.median(), 0.001)
	require.Equal(t, 5, len(w.allSamples()))

	w.add(301.90) // crazy big number, median should remain stable
	require.InDelta(t, 301.90, w.lastSample(), 0.001)
	require.InDelta(t, 63.1259, w.mean(), 0.001)
	require.InDelta(t, 3.52, w.median(), 0.001)
	require.Equal(t, 5, len(w.allSamples()))
}

func TestSlidingWindowFull(t *testing.T) {
	w := newSlidingWindow(5)
	w.add(42)
	require.False(t, w.Full())
	w.add(42)
	require.False(t, w.Full())
	w.add(42)
	require.False(t, w.Full())
	w.add(42)
	require.False(t, w.Full())
	w.add(42)
	require.True(t, w.Full())
	w.add(42)
	require.True(t, w.Full())
}
