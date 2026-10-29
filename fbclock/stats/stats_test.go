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
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewStats(t *testing.T) {
	s := NewStats()
	require.NotNil(t, s)
	require.Empty(t, s.Get())
}

func TestSetCounter(t *testing.T) {
	s := NewStats()
	s.SetCounter("test_key", 42)
	got := s.Get()
	require.Equal(t, int64(42), got["test_key"])
}

func TestSetCounterOverwrite(t *testing.T) {
	s := NewStats()
	s.SetCounter("key", 10)
	s.SetCounter("key", 20)
	got := s.Get()
	require.Equal(t, int64(20), got["key"])
}

func TestUpdateCounterBy(t *testing.T) {
	s := NewStats()
	s.UpdateCounterBy("counter", 5)
	s.UpdateCounterBy("counter", 3)
	s.UpdateCounterBy("counter", -1)
	got := s.Get()
	require.Equal(t, int64(7), got["counter"])
}

func TestUpdateCounterByNewKey(t *testing.T) {
	s := NewStats()
	s.UpdateCounterBy("new_key", 100)
	got := s.Get()
	require.Equal(t, int64(100), got["new_key"])
}

func TestGetReturnsCopy(t *testing.T) {
	s := NewStats()
	s.SetCounter("key", 42)
	got := s.Get()
	got["key"] = 999
	require.Equal(t, int64(42), s.Get()["key"])
}

func TestGetMultipleKeys(t *testing.T) {
	s := NewStats()
	s.SetCounter("a", 1)
	s.SetCounter("b", 2)
	s.SetCounter("c", 3)
	got := s.Get()
	require.Equal(t, int64(1), got["a"])
	require.Equal(t, int64(2), got["b"])
	require.Equal(t, int64(3), got["c"])
	require.Len(t, got, 3)
}

func TestCopy(t *testing.T) {
	src := NewStats()
	src.SetCounter("x", 10)
	src.SetCounter("y", 20)

	dst := NewStats()
	dst.SetCounter("z", 30)
	src.Copy(dst)

	got := dst.Get()
	require.Equal(t, int64(10), got["x"])
	require.Equal(t, int64(20), got["y"])
	require.Equal(t, int64(30), got["z"])
}

func TestReset(t *testing.T) {
	s := NewStats()
	s.SetCounter("a", 100)
	s.SetCounter("b", 200)
	s.Reset()
	got := s.Get()
	require.Equal(t, int64(0), got["a"])
	require.Equal(t, int64(0), got["b"])
	require.Len(t, got, 2)
}

func TestResetEmpty(t *testing.T) {
	s := NewStats()
	s.Reset()
	require.Empty(t, s.Get())
}

func TestConcurrentAccess(t *testing.T) {
	s := NewStats()
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Go(func() {
			s.SetCounter("concurrent", int64(i))
			s.UpdateCounterBy("incr", 1)
			s.Get()
		})
	}
	wg.Wait()
	got := s.Get()
	require.Equal(t, int64(100), got["incr"])
}

func TestNewJSONStats(t *testing.T) {
	js := NewJSONStats()
	require.NotNil(t, js)
	require.Empty(t, js.Get())
}

func TestHandleStatsRequest(t *testing.T) {
	js := NewJSONStats()
	js.SetCounter("requests", 42)
	js.SetCounter("errors", 3)

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	w := httptest.NewRecorder()
	js.handleStatsRequest(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var result map[string]int64
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	require.Equal(t, int64(42), result["requests"])
	require.Equal(t, int64(3), result["errors"])
}

func TestHandleStatsRequestDuringCounterUpdates(t *testing.T) {
	js := NewJSONStats()

	stop := make(chan struct{})
	started := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := 0; ; i++ {
			js.UpdateCounterBy("data_error", 1)
			js.SetCounter("processing_error", int64(i))
			if i == 0 {
				close(started)
			}
			select {
			case <-stop:
				return
			default:
			}
			runtime.Gosched()
		}
	})
	t.Cleanup(func() {
		close(stop)
		wg.Wait()
	})
	<-started

	for range 200 {
		req := httptest.NewRequest(http.MethodGet, "/stats", nil)
		w := httptest.NewRecorder()
		js.handleStatsRequest(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var result map[string]int64
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		require.GreaterOrEqual(t, result["data_error"], int64(1))
		require.Contains(t, result, "processing_error")
	}
}

func TestHandleStatsRequestEmpty(t *testing.T) {
	js := NewJSONStats()
	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	w := httptest.NewRecorder()
	js.handleStatsRequest(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var result map[string]int64
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestGetCounters(t *testing.T) {
	js := NewJSONStats()
	js.SetCounter("data_error", 7)
	js.SetCounter("m_ns", 120)

	got := js.GetCounters()
	require.Equal(t, []string{
		"clock.data_error",
		"clock.m_ns",
		"go.process.rss",
		"go.runtime.num_goroutines",
	}, slices.Sorted(maps.Keys(got)))
	require.Equal(t, int64(7), got["clock.data_error"])
	require.Equal(t, int64(120), got["clock.m_ns"])
	require.Positive(t, got["go.process.rss"])
	require.Positive(t, got["go.runtime.num_goroutines"])
	// process stats are not stored as counters
	require.Equal(t, map[string]int64{"data_error": 7, "m_ns": 120}, js.Get())
}

func TestHandleCountersRequest(t *testing.T) {
	js := NewJSONStats()
	js.SetCounter("data_error", 7)

	req := httptest.NewRequest(http.MethodGet, "/counters", nil)
	w := httptest.NewRecorder()
	js.handleCountersRequest(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))
	var result map[string]int64
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	require.Equal(t, int64(7), result["clock.data_error"])
	require.Contains(t, result, "go.process.rss")
}

func TestHandlerRoutes(t *testing.T) {
	h := NewJSONStats().handler()
	for path, want := range map[string]int{
		"/stats":    http.StatusOK,
		"/counters": http.StatusOK,
		"/":         http.StatusNotFound,
		"/foo":      http.StatusNotFound,
		"/stats/x":  http.StatusNotFound,
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, want, w.Code, path)
	}
}

// held stays referenced for the whole test: a listener nobody holds is closed
// by the runtime finalizer on its socket, which hands the port straight back
func TestListenFailsOnHeldPort(t *testing.T) {
	held, err := listen(0)
	require.NoError(t, err)
	defer held.Close()

	l, err := listen(held.Addr().(*net.TCPAddr).Port)
	if l != nil {
		l.Close()
	}
	require.Error(t, err, "listen must report a port it cannot bind")
}

func TestServe(t *testing.T) {
	l, err := listen(0)
	require.NoError(t, err)
	js := NewJSONStats()
	js.SetCounter("data_error", 7)
	go js.serve(l)

	url := fmt.Sprintf("http://[::1]:%d/stats", l.Addr().(*net.TCPAddr).Port)
	var got map[string]int64
	require.Eventually(t, func() bool {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&got) == nil
	}, 5*time.Second, 10*time.Millisecond)
	require.Equal(t, map[string]int64{"data_error": 7}, got)
}
