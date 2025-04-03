package metrics

import (
	"container/list"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetMetrics(t *testing.T) {
	handler := &Handler{}
	handler.minOffset = 10.0
	handler.maxOffset = 20.0
	metrics := handler.getMetrics()
	require.Equal(t, map[string]float64{
		"min_offset": 10.0,
		"max_offset": 20.0,
	}, metrics)
	if metrics["min_offset"] != 10.0 {
		t.Errorf("Expected min_offset to be 10.0, got %f", metrics["min_offset"])
	}
	if metrics["max_offset"] != 20.0 {
		t.Errorf("Expected max_offset to be 20.0, got %f", metrics["max_offset"])
	}
}

func TestObserveOffset(t *testing.T) {
	tests := []struct {
		name           string
		offsets        []float64
		observedOffset float64
		expectedMin    float64
		expectedMax    float64
	}{
		{
			name:           "Min offset updated",
			offsets:        []float64{10.0},
			observedOffset: -10.0,
			expectedMin:    -10.0,
			expectedMax:    10.0,
		},
		{
			name:           "Max offset updated",
			offsets:        []float64{10.0, 5.0, 15.0},
			observedOffset: 100.0,
			expectedMin:    5.0,
			expectedMax:    100.0,
		},
		{
			name:           "None updated",
			offsets:        []float64{10.0, 5.0, 15.0},
			observedOffset: 7.0,
			expectedMin:    5.0,
			expectedMax:    15.0,
		},
		{
			name:           "Exceed max samples (60 samples)",
			offsets:        append([]float64{-1000}, repeatNumber(maxSamples, 10.0)...),
			observedOffset: -999.0,
			expectedMin:    -999.0,
			expectedMax:    10.0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{
				minOffset: math.Inf(1),
				maxOffset: math.Inf(-1),
				offsets:   listify(tt.offsets),
			}
			handler.ObserveOffset(tt.observedOffset)
			require.Equal(t, tt.expectedMin, handler.minOffset)
			require.Equal(t, tt.expectedMax, handler.maxOffset)
		})
	}
}

// create a slize with the given number repeated repetitionCount times
func repeatNumber(repetitionCount int, number float64) []float64 {
	slice := []float64{}
	for i := 0; i < repetitionCount; i++ {
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
