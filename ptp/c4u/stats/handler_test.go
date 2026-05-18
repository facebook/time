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

	"github.com/stretchr/testify/require"
)

func TestSnapshotAndHandleRequest(t *testing.T) {
	js := NewJSONStats()
	js.SetPHCOffsetNS(12345)
	js.SetOscillatorOffsetNS(-9876)
	js.SetUTCOffsetSec(37)
	js.SetClockAccuracy(100)
	js.SetClockAccuracyWorst(250)
	js.SetClockClass(6)
	js.IncReload()
	js.IncReload()
	js.IncDataError()

	js.Snapshot()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	js.handleRequest(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var result map[string]int64
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	require.Equal(t, int64(12345), result["phc_offset_ns"])
	require.Equal(t, int64(-9876), result["oscillator_offset_ns"])
	require.Equal(t, int64(37), result["utc_offset_sec"])
	require.Equal(t, int64(100), result["clock_accuracy"])
	require.Equal(t, int64(250), result["clock_accuracy_worst"])
	require.Equal(t, int64(6), result["clock_class"])
	require.Equal(t, int64(2), result["reload"])
	require.Equal(t, int64(1), result["data_error"])
}

func TestSnapshotIsolation(t *testing.T) {
	js := NewJSONStats()
	js.SetPHCOffsetNS(100)
	js.Snapshot()

	// change after snapshot — should not affect report
	js.SetPHCOffsetNS(999)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	js.handleRequest(w, req)

	var result map[string]int64
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	require.Equal(t, int64(100), result["phc_offset_ns"])
}

func TestResetCounters(t *testing.T) {
	js := NewJSONStats()
	js.IncReload()
	js.IncReload()
	js.IncDataError()

	js.ResetReload()
	js.ResetDataError()
	js.Snapshot()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	js.handleRequest(w, req)

	var result map[string]int64
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	require.Equal(t, int64(0), result["reload"])
	require.Equal(t, int64(0), result["data_error"])
}
