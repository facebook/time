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
	"sync"
	"testing"

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
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.SetCounter("concurrent", int64(i))
			s.UpdateCounterBy("incr", 1)
			s.Get()
		}()
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

func TestHandleRequest(t *testing.T) {
	js := NewJSONStats()
	js.SetCounter("requests", 42)
	js.SetCounter("errors", 3)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	js.handleRequest(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var result map[string]int64
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	require.Equal(t, int64(42), result["requests"])
	require.Equal(t, int64(3), result["errors"])
}

func TestHandleRequestEmpty(t *testing.T) {
	js := NewJSONStats()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	js.handleRequest(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var result map[string]int64
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	require.Empty(t, result)
}
