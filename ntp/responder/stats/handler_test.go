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
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandleRequest(t *testing.T) {
	j := &JSONStats{}
	atomic.AddInt64(&j.requests, 42)
	atomic.AddInt64(&j.responses, 10)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	j.handleRequest(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var result map[string]int64
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	require.Equal(t, int64(42), result["requests"])
	require.Equal(t, int64(10), result["responses"])
	require.Equal(t, int64(0), result["invalidformat"])
}

func TestHandleRequestEmpty(t *testing.T) {
	j := &JSONStats{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	j.handleRequest(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var result map[string]int64
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	require.Equal(t, int64(0), result["requests"])
}

func TestToMap(t *testing.T) {
	j := &JSONStats{}
	j.IncRequests()
	j.IncRequests()
	j.IncResponses()
	j.IncInvalidFormat()
	j.IncListeners()
	j.IncWorkers()
	j.IncReadError()
	j.SetAnnounce()

	m := j.toMap()
	require.Equal(t, int64(2), m["requests"])
	require.Equal(t, int64(1), m["responses"])
	require.Equal(t, int64(1), m["invalidformat"])
	require.Equal(t, int64(1), m["listeners"])
	require.Equal(t, int64(1), m["workers"])
	require.Equal(t, int64(1), m["readError"])
	require.Equal(t, int64(1), m["announce"])
}

func TestResetAnnounce(t *testing.T) {
	j := &JSONStats{}
	j.SetAnnounce()
	m := j.toMap()
	require.Equal(t, int64(1), m["announce"])

	j.ResetAnnounce()
	m = j.toMap()
	require.Equal(t, int64(0), m["announce"])
}
