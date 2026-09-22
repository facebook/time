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

// Package measurements holds the path delay window and the screen applied to
// it, shared by the grandmaster path, which learns its estimate across time,
// and the in-rack peer path, which learns it across peers.
package measurement

import (
	"math"
	"sort"
	"time"
)

type Window struct {
	size        int
	currentSize int
	sum         float64
	samples     []float64
	sorted      []float64
}

func NewWindow(size int) *Window {
	if size < 1 {
		size = 1
	}
	w := &Window{
		size:    size,
		samples: make([]float64, size),
		sorted:  make([]float64, size),
	}
	for i := range w.size {
		w.samples[i] = math.NaN()
		w.sorted[i] = math.NaN()
	}
	return w
}

func (w *Window) Add(sample float64) {
	if !w.Full() {
		w.currentSize++
	} else {
		w.sum -= w.samples[w.size-1]
	}
	for i := w.currentSize - 1; i > 0; i-- {
		w.samples[i] = w.samples[i-1]
	}

	w.samples[0] = sample
	w.sum += sample
}

func (w *Window) LastSample() float64 {
	return w.samples[0]
}

func (w *Window) allSamples() []float64 {
	for j, v := range w.samples {
		if !math.IsNaN(v) {
			w.sorted[j] = v
		}
	}
	return w.sorted[0:w.currentSize]
}

func mean(data []float64) float64 {
	sum := 0.0
	for _, v := range data {
		sum += v
	}
	return sum / float64(len(data))
}

func (w *Window) Median() float64 {
	c := w.allSamples()
	sort.Float64s(c)
	l := len(c)
	if l == 0 {
		return math.NaN()
	} else if l%2 == 0 {
		return mean(c[l/2-1 : l/2+1])
	}
	return c[l/2]
}

func (w *Window) Mean() float64 {
	return w.sum / float64(w.currentSize)
}

func (w *Window) Full() bool {
	return w.currentSize == w.size
}

// Percentile returns the pth (0..1) smallest sample, NaN when empty.
func (w *Window) Percentile(p float64) float64 {
	c := w.allSamples()
	if len(c) == 0 {
		return math.NaN()
	}
	sort.Float64s(c)
	return c[min(max(int(p*float64(len(c))), 0), len(c)-1)]
}

// PathDelayInRange reports whether a path delay is usable: at or above the
// floor, and not a spike over the running estimate once there is enough history
// to judge. Negative falls out of the floor rather than needing its own case.
func PathDelayInRange(delay, ceiling, below, from time.Duration, full bool) bool {
	if delay < below {
		return false
	}
	return delay <= from || delay <= ceiling || ceiling <= below || !full
}
