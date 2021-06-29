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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJSONStatsInvalidFormat(t *testing.T) {
	stats := JSONStats{}

	stats.IncInvalidFormat()
	require.Equal(t, int64(1), stats.invalidFormat)
}

func TestJSONStatsRequests(t *testing.T) {
	stats := JSONStats{}

	stats.IncRequests()
	require.Equal(t, int64(1), stats.requests)
}

func TestJSONStatsResponses(t *testing.T) {
	stats := JSONStats{}

	stats.IncResponses()
	require.Equal(t, int64(1), stats.responses)
}

func TestJSONStatsListeners(t *testing.T) {
	stats := JSONStats{}

	stats.IncListeners()
	require.Equal(t, int64(1), stats.listeners)

	stats.DecListeners()
	require.Equal(t, int64(0), stats.listeners)
}

func TestJSONStatsWorkers(t *testing.T) {
	stats := JSONStats{}

	stats.IncWorkers()
	require.Equal(t, int64(1), stats.workers)

	stats.DecWorkers()
	require.Equal(t, int64(0), stats.workers)
}

func TestJSONStatsReadError(t *testing.T) {
	stats := JSONStats{}

	stats.IncReadError()
	require.Equal(t, int64(1), stats.readError)
}

func TestJSONStatsAnnounce(t *testing.T) {
	stats := JSONStats{}

	stats.SetAnnounce()
	require.Equal(t, int64(1), stats.announce)

	stats.ResetAnnounce()
	require.Equal(t, int64(0), stats.announce)
}

func TestJSONStatsToMap(t *testing.T) {
	j := JSONStats{
		invalidFormat: 1,
		requests:      2,
		responses:     3,
		listeners:     4,
		workers:       5,
		readError:     6,
		announce:      7,
	}
	result := j.toMap()

	expectedMap := make(map[string]int64)
	expectedMap["invalidformat"] = 1
	expectedMap["requests"] = 2
	expectedMap["responses"] = 3
	expectedMap["listeners"] = 4
	expectedMap["workers"] = 5
	expectedMap["readError"] = 6
	expectedMap["announce"] = 7

	require.Equal(t, expectedMap, result)
}
