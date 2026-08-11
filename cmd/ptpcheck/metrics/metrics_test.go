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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

const concurrentScrapes = 20000

func TestGetMetrics(t *testing.T) {
	handler := &Handler{
		maxOffset:  -20.0,
		lastOffset: -5.0,
		lastUpdate: 1710000000,
	}
	metrics := handler.getMetrics()
	require.Equal(t, map[string]float64{
		"offset.ns":     -5.0,
		"offset.max.60": -20.0,
		"last_update":   1710000000.0,
	}, metrics)
}

func TestObserveOffset(t *testing.T) {
	tests := []struct {
		name           string
		offsets        []float64
		observedOffset float64
		expectedMax    float64
	}{
		{
			name:           "Negative observed value becomes signed max",
			offsets:        []float64{10.0},
			observedOffset: -100.0,
			expectedMax:    -100.0,
		},
		{
			name:           "Positive observed value becomes signed max",
			offsets:        []float64{10.0, 5.0, 15.0, -90.0},
			observedOffset: 100.0,
			expectedMax:    100.0,
		},
		{
			name:           "Keeps signed value of largest-magnitude sample",
			offsets:        []float64{10.0, 5.0, -15.0},
			observedOffset: 7.0,
			expectedMax:    -15.0,
		},
		{
			name:           "Exceed max samples (60 samples)",
			offsets:        append([]float64{-1000}, repeatNumber(maxSamples, 10.0)...),
			observedOffset: -999.0,
			expectedMax:    -999.0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{
				offsets: listify(tt.offsets),
			}
			handler.ObserveOffset(tt.observedOffset)
			require.Equal(t, tt.expectedMax, handler.maxOffset)
			require.Equal(t, tt.observedOffset, handler.lastOffset)
		})
	}
}

func TestObserveOffsetOnZeroValueHandler(t *testing.T) {
	handler := &Handler{}
	require.NotPanics(t, func() { handler.ObserveOffset(42.0) })
	require.Equal(t, 42.0, handler.getMetrics()["offset.ns"])
}

func TestServeHTTPDuringObserveOffset(t *testing.T) {
	handler := &Handler{}

	// Each offset outgrows the whole window before it, so ObserveOffset always leaves maxOffset == lastOffset.
	var torn, malformed, observed atomic.Int64
	var overlapped atomic.Bool
	var scrapers, observer sync.WaitGroup
	start, stop := make(chan struct{}), make(chan struct{})
	observer.Go(func() {
		<-start
		for i := int64(1); ; i++ {
			handler.ObserveOffset(float64(i))
			observed.Store(i)
			select {
			case <-stop:
				return
			default:
			}
		}
	})
	for range 4 {
		scrapers.Go(func() {
			<-start
			var prev float64
			for range concurrentScrapes {
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
				var served map[string]float64
				if err := json.Unmarshal(w.Body.Bytes(), &served); err != nil {
					malformed.Add(1)
					continue
				}
				if served["offset.ns"] != served["offset.max.60"] {
					torn.Add(1)
				}
				if prev != 0 && served["offset.ns"] != prev {
					overlapped.Store(true)
				}
				prev = served["offset.ns"]
			}
		})
	}
	close(start)
	scrapers.Wait()
	close(stop)
	observer.Wait()

	require.Zero(t, malformed.Load(), "handler served a body that is not valid JSON")
	require.Zero(t, torn.Load(), "served offset.ns and offset.max.60 from different updates")
	require.True(t, overlapped.Load(), "no scrape saw the offset change, so ServeHTTP never overlapped ObserveOffset")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var result map[string]float64
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	require.Equal(t, float64(observed.Load()), result["offset.ns"])
	require.Equal(t, float64(observed.Load()), result["offset.max.60"])
	require.Positive(t, result["last_update"])
}

// repeatNumber creates a slice with the given number repeated repetitionCount times
func repeatNumber(repetitionCount int, number float64) []float64 {
	slice := make([]float64, 0, repetitionCount)
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
