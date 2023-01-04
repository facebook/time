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
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJSONStatsReset(t *testing.T) {
	stats := JSONStats{}

	stats.IncDataError()
	stats.IncReload()

	stats.ResetDataError()
	stats.ResetReload()
	require.Equal(t, int64(0), stats.reload)
	require.Equal(t, int64(0), stats.dataError)
}

func TestJSONStatsSetClockAccuracy(t *testing.T) {
	stats := NewJSONStats()

	stats.SetClockAccuracy(42)
	require.Equal(t, int64(42), stats.clockAccuracy)
}

func TestJSONStatsSetClockCLass(t *testing.T) {
	stats := NewJSONStats()

	stats.SetClockClass(42)
	require.Equal(t, int64(42), stats.clockClass)
}

func TestJSONStatsSnapshot(t *testing.T) {
	stats := NewJSONStats()

	go stats.Start(0)
	time.Sleep(time.Millisecond)

	stats.SetClockAccuracyWorst(1)
	stats.SetClockAccuracy(1)
	stats.SetClockClass(1)
	stats.SetUTCOffsetSec(1)
	stats.IncReload()

	stats.Snapshot()

	expectedStats := counters{}
	expectedStats.utcOffsetSec = 1
	expectedStats.clockAccuracyWorst = 1
	expectedStats.clockAccuracy = 1
	expectedStats.clockClass = 1
	expectedStats.reload = 1

	require.Equal(t, expectedStats.utcOffsetSec, stats.report.utcOffsetSec)
	require.Equal(t, expectedStats.clockAccuracyWorst, stats.report.clockAccuracyWorst)
	require.Equal(t, expectedStats.clockAccuracy, stats.report.clockAccuracy)
	require.Equal(t, expectedStats.clockClass, stats.report.clockClass)
	require.Equal(t, expectedStats.reload, stats.report.reload)
}

func TestJSONExport(t *testing.T) {
	stats := NewJSONStats()

	go stats.Start(8889)
	time.Sleep(time.Second)

	stats.SetUTCOffsetSec(1)
	stats.SetClockAccuracy(1)
	stats.SetClockAccuracyWorst(1)
	stats.SetClockClass(1)
	stats.IncReload()

	stats.Snapshot()

	resp, err := http.Get("http://localhost:8889")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var data map[string]int64
	err = json.Unmarshal(body, &data)
	require.NoError(t, err)

	expectedMap := map[string]int64{
		"phc_offset_ns":        0,
		"oscillator_offset_ns": 0,
		"utc_offset_sec":       1,
		"clock_accuracy_worst": 1,
		"clock_accuracy":       1,
		"clock_class":          1,
		"data_error":           0,
		"reload":               1,
	}

	require.Equal(t, expectedMap, data)
}
