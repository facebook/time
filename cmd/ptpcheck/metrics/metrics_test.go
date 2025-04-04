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

package metrics

import (
	"container/list"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetMetrics(t *testing.T) {
	handler := &Handler{
		maxOffsetAbs: 20.0,
	}
	metrics := handler.getMetrics()
	require.Equal(t, map[string]float64{
		"offset.abs_max": 20.0,
	}, metrics)
}

func TestObserveOffset(t *testing.T) {
	tests := []struct {
		name              string
		offsets           []float64
		observedOffset    float64
		expectedOffsetAbs float64
	}{
		{
			name:              "Max absolute offset updated when given negative value",
			offsets:           []float64{10.0},
			observedOffset:    -100.0,
			expectedOffsetAbs: 100.0,
		},
		{
			name:              "Max offset updated when given positive value",
			offsets:           []float64{10.0, 5.0, 15.0, -90.0},
			observedOffset:    100.0,
			expectedOffsetAbs: 100.0,
		},
		{
			name:              "Does not update when |observedOffset| is less than |max| from offsets",
			offsets:           []float64{10.0, 5.0, 15.0},
			observedOffset:    7.0,
			expectedOffsetAbs: 15.0,
		},
		{
			name:              "Exceed max samples (60 samples)",
			offsets:           append([]float64{-1000}, repeatNumber(maxSamples, 10.0)...),
			observedOffset:    -999.0,
			expectedOffsetAbs: 999.0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{
				offsets: listify(tt.offsets),
			}
			handler.ObserveOffset(tt.observedOffset)
			require.Equal(t, tt.expectedOffsetAbs, handler.maxOffsetAbs)
		})
	}
}

// repeatNumber creates a slice with the given number repeated repetitionCount times
func repeatNumber(repetitionCount int, number float64) []float64 {
	slice := []float64{}
	for range repetitionCount {
		slice = append(slice, number)
	}
	return slice
}

func listify(numbers []float64) *list.List {
	list := list.List{}
	for _, number := range numbers {
		list.PushFront(number)
	}
	return &list
}
