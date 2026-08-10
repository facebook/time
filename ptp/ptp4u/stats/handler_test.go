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
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSnapshotDuringHandleRequest(t *testing.T) {
	js := NewJSONStats(1)
	const iterations = 20000

	// The writer sets drain and clockclass together, so every published report
	// satisfies drain == clockclass+1. drain is 0 only before the first
	// Snapshot. Any other pairing means the handler served a report that two
	// Snapshot calls were interleaved into. mode/dev-gorace reports the race
	// itself and is the deterministic detector; this counter is what catches
	// it in the default mode.
	var mixed atomic.Int64
	var wg sync.WaitGroup
	// start makes the reporter and the scrapers runnable at the same instant,
	// so the reporter cannot finish before a scraper is scheduled.
	start := make(chan struct{})
	wg.Go(func() {
		<-start
		for i := range iterations {
			js.SetClockClass(int64(i))
			js.SetDrain(int64(i) + 1)
			js.Snapshot()
		}
	})
	for range 4 {
		wg.Go(func() {
			<-start
			for range iterations {
				w := httptest.NewRecorder()
				js.handleRequest(w, httptest.NewRequest(http.MethodGet, "/", nil))
				var served map[string]int64
				if err := json.Unmarshal(w.Body.Bytes(), &served); err != nil {
					mixed.Add(1)
					continue
				}
				if served["drain"] != 0 && served["drain"] != served["clockclass"]+1 {
					mixed.Add(1)
				}
			}
		})
	}
	close(start)
	wg.Wait()
	require.Zero(t, mixed.Load(), "handler served a report mixing two Snapshot calls")

	w := httptest.NewRecorder()
	js.handleRequest(w, httptest.NewRequest(http.MethodGet, "/", nil))

	require.Equal(t, http.StatusOK, w.Code)
	var result map[string]int64
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	require.Equal(t, int64(iterations-1), result["clockclass"])
	require.Equal(t, int64(iterations), result["drain"])
}
