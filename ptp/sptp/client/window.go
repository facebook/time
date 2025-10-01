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
	"sort"
)

type slidingWindow struct {
	size        int
	currentSize int
	sum         float64
	samples     []float64
	sorted      []float64
}

func newSlidingWindow(size int) *slidingWindow {
	if size < 1 {
		size = 1
	}
	w := &slidingWindow{
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

func (w *slidingWindow) add(sample float64) {
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

func (w *slidingWindow) lastSample() float64 {
	return w.samples[0]
}

func (w *slidingWindow) allSamples() []float64 {
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

func (w *slidingWindow) median() float64 {
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

func (w *slidingWindow) mean() float64 {
	return w.sum / float64(w.currentSize)
}

func (w *slidingWindow) Full() bool {
	return w.currentSize == w.size
}
