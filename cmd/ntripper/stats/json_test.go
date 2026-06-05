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

package stats

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJSONStatsCounters(t *testing.T) {
	st := NewJSONStats()

	st.IncFramesReceived()
	st.IncFramesReceived()
	st.SetConnected(1)

	require.Equal(t, int64(2), st.framesReceived.Load())
	require.Equal(t, int64(1), st.connected.Load())
}

func TestJSONStatsHTTPEndpoint(t *testing.T) {
	st := NewJSONStats()

	st.SetConnected(1)

	// Manually set the rate.
	st.framesPerSecond.Store(42)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	st.handleRequest(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var snap snapshot
	err := json.Unmarshal(rec.Body.Bytes(), &snap)
	require.NoError(t, err)

	require.Equal(t, int64(42), snap.FramesPerSecond)
	require.Equal(t, int64(1), snap.Connected)
}

func TestJSONStatsFramesPerSecond(t *testing.T) {
	st := NewJSONStats()

	for range 10 {
		st.IncFramesReceived()
	}

	cur := st.framesReceived.Load()
	prev := st.lastFrames.Swap(cur)
	st.framesPerSecond.Store(cur - prev)

	require.Equal(t, int64(10), st.framesPerSecond.Load())

	for range 5 {
		st.IncFramesReceived()
	}

	cur = st.framesReceived.Load()
	prev = st.lastFrames.Swap(cur)
	st.framesPerSecond.Store(cur - prev)

	require.Equal(t, int64(5), st.framesPerSecond.Load())
}

func TestComputeRatesIntegration(t *testing.T) {
	st := NewJSONStats()
	go st.computeRates()

	for range 20 {
		st.IncFramesReceived()
	}

	time.Sleep(1100 * time.Millisecond)

	require.Greater(t, st.framesPerSecond.Load(), int64(0))
}
