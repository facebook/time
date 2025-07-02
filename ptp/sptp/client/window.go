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
)

type slidingWindow struct {
	size        int
	currentSize int
	head        int
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
		head:    size - 1,
		samples: make([]float64, size),
		sorted:  make([]float64, size),
	}
	for i := 0; i < w.size; i++ {
		w.samples[i] = math.NaN()
		w.sorted[i] = math.NaN()
	}
	return w
}

func (w *slidingWindow) add(sample float64) {
	w.head = (w.head + 1) % w.size
	if !w.Full() {
		w.currentSize++
	} else {
		w.sum -= w.samples[w.head]
	}
	w.samples[w.head] = sample
	w.sum += sample
}

func (w *slidingWindow) lastSample() float64 {
	return w.samples[w.head]
}

func (w *slidingWindow) allSamples() []float64 {
	for j, v := range w.samples {
		if !math.IsNaN(v) {
			w.sorted[j] = v
		}
	}
	return w.sorted[0:w.currentSize]
}

func medianOfThree(arr []float64, low, high int) {
	mid := low + (high-low)/2
	if (arr[low] > arr[mid]) != (arr[low] > arr[high]) {
		arr[low], arr[high] = arr[high], arr[low]
	} else if (arr[mid] > arr[low]) != (arr[mid] > arr[high]) {
		arr[mid], arr[high] = arr[high], arr[mid]
	}
}

func partition(arr []float64, low, high int) int {
	if high-low > 3 {
		medianOfThree(arr, low, high)
	}
	pivot := arr[high]
	i := low
	for j := low; j < high; j++ {
		if arr[j] < pivot {
			arr[i], arr[j] = arr[j], arr[i]
			i++
		}
	}
	arr[i], arr[high] = arr[high], arr[i]
	return i
}

func quickselect(arr []float64, low, high, k int) float64 {
	for low <= high {
		pi := partition(arr, low, high)
		if pi == k {
			return arr[pi]
		} else if pi < k {
			low = pi + 1
		} else {
			high = pi - 1
		}
	}
	return math.NaN()
}

func (w *slidingWindow) median() float64 {
	c := w.allSamples()
	l := len(c)
	if l == 0 {
		return math.NaN()
	} else if l%2 == 1 {
		return quickselect(c, 0, l-1, l/2)
	}
	mid1 := quickselect(c, 0, l-1, l/2-1)
	mid2 := c[l/2]
	for i := l/2 + 1; i < l; i++ {
		if c[i] < mid2 {
			mid2 = c[i]
		}
	}
	return (mid1 + mid2) / 2.0
}

func (w *slidingWindow) mean() float64 {
	return w.sum / float64(w.currentSize)
}

func (w *slidingWindow) Full() bool {
	return w.currentSize == w.size
}
