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
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	// Registers the profiling endpoints on http.DefaultServeMux, exactly as the
	// ntpresponder binary's own blank import does. Without it the process the
	// test runs in would not have the exposure the test is looking for.
	_ "net/http/pprof" //nolint:gosec // G108 is the bug under test, reproduced on purpose

	"github.com/stretchr/testify/require"
)

// toMapReads is how many times each reader in TestToMapDuringCounterUpdates
// calls ToMap.
const toMapReads = 2000

func TestHandleRequest(t *testing.T) {
	j := &JSONStats{}
	j.requests.Add(42)
	j.responses.Add(10)

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

func TestMonitoringPortDoesNotServePprof(t *testing.T) {
	_, pprofPattern := http.DefaultServeMux.Handler(
		httptest.NewRequest(http.MethodGet, "/debug/pprof/cmdline", nil),
	)
	require.NotEmpty(t, pprofPattern, "net/http/pprof is no longer linked, this test proves nothing")

	j := &JSONStats{}
	j.requests.Add(7)

	port := freePort(t)
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	go j.Start(port)
	require.Eventually(t, func() bool {
		resp, err := http.Get(base + "/")
		if err != nil {
			return false
		}
		return resp.Body.Close() == nil
	}, 30*time.Second, 10*time.Millisecond, "monitoring server never came up on %s", base)

	for _, path := range []string{"/", "/debug/pprof/", "/debug/pprof/cmdline", "/debug/pprof/symbol"} {
		resp, err := http.Get(base + path)
		require.NoError(t, err, path)
		body, readErr := io.ReadAll(resp.Body)
		require.NoError(t, readErr, path)
		require.NoError(t, resp.Body.Close(), path)

		require.Equal(t, http.StatusOK, resp.StatusCode, path)
		require.Equal(t, "application/json", resp.Header.Get("Content-Type"), path)

		var counters map[string]int64
		require.NoError(t, json.Unmarshal(body, &counters), path)
		require.Equal(t, int64(7), counters["requests"], path)
	}
}

// freePort returns a port nothing is listening on, for Start to bind.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	return port
}

func TestToMapDuringCounterUpdates(t *testing.T) {
	j := &JSONStats{}

	stop := make(chan struct{})
	counted := make(chan struct{})
	var writer sync.WaitGroup
	writer.Go(func() {
		for i := 0; ; i++ {
			j.IncRequests()
			j.IncListeners()
			j.SetAnnounce()
			if i == 0 {
				close(counted)
			}
			select {
			case <-stop:
				return
			default:
			}
		}
	})
	// Wait for the first update so the readers below overlap the writer
	// instead of racing it to start.
	<-counted

	var negative atomic.Bool
	var readers sync.WaitGroup
	for range 4 {
		readers.Go(func() {
			for range toMapReads {
				for _, v := range j.ToMap() {
					if v < 0 {
						negative.Store(true)
					}
				}
			}
		})
	}
	readers.Wait()
	close(stop)
	writer.Wait()

	require.False(t, negative.Load())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	j.handleRequest(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var result map[string]int64
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	require.Positive(t, result["requests"])
	require.Equal(t, int64(1), result["announce"])
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

	m := j.ToMap()
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
	m := j.ToMap()
	require.Equal(t, int64(1), m["announce"])

	j.ResetAnnounce()
	m = j.ToMap()
	require.Equal(t, int64(0), m["announce"])
}
