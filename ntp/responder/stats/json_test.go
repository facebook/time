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
	require.Equal(t, int64(1), stats.invalidFormat.Load())
}

func TestJSONStatsRequests(t *testing.T) {
	stats := JSONStats{}

	stats.IncRequests()
	require.Equal(t, int64(1), stats.requests.Load())
}

func TestJSONStatsResponses(t *testing.T) {
	stats := JSONStats{}

	stats.IncResponses()
	require.Equal(t, int64(1), stats.responses.Load())
}

func TestJSONStatsListeners(t *testing.T) {
	stats := JSONStats{}

	stats.IncListeners()
	require.Equal(t, int64(1), stats.listeners.Load())

	stats.DecListeners()
	require.Equal(t, int64(0), stats.listeners.Load())
}

func TestJSONStatsWorkers(t *testing.T) {
	stats := JSONStats{}

	stats.IncWorkers()
	require.Equal(t, int64(1), stats.workers.Load())

	stats.DecWorkers()
	require.Equal(t, int64(0), stats.workers.Load())
}

func TestJSONStatsReadError(t *testing.T) {
	stats := JSONStats{}

	stats.IncReadError()
	require.Equal(t, int64(1), stats.readError.Load())
}

func TestJSONStatsNTSAuthOK(t *testing.T) {
	stats := JSONStats{}

	stats.IncNTSAuthOK()
	require.Equal(t, int64(1), stats.ntsAuthOK.Load())
}

func TestJSONStatsNTSAuthFailed(t *testing.T) {
	stats := JSONStats{}

	stats.IncNTSAuthFailed()
	require.Equal(t, int64(1), stats.ntsAuthFailed.Load())
}

func TestJSONStatsNTSCookieOpenFailed(t *testing.T) {
	stats := JSONStats{}

	stats.IncNTSCookieOpenFailed()
	require.Equal(t, int64(1), stats.ntsCookieOpenFailed.Load())
}

func TestJSONStatsAnnounce(t *testing.T) {
	stats := JSONStats{}

	stats.SetAnnounce()
	require.Equal(t, int64(1), stats.announce.Load())

	stats.ResetAnnounce()
	require.Equal(t, int64(0), stats.announce.Load())
}

func TestJSONStatsToMap(t *testing.T) {
	j := JSONStats{}
	j.invalidFormat.Store(1)
	j.requests.Store(2)
	j.responses.Store(3)
	j.listeners.Store(4)
	j.workers.Store(5)
	j.readError.Store(6)
	j.announce.Store(7)
	j.ntsAuthOK.Store(8)
	j.ntsAuthFailed.Store(9)
	j.ntsCookieOpenFailed.Store(10)
	result := j.ToMap()

	expectedMap := make(map[string]int64)
	expectedMap["invalidformat"] = 1
	expectedMap["requests"] = 2
	expectedMap["responses"] = 3
	expectedMap["listeners"] = 4
	expectedMap["workers"] = 5
	expectedMap["readError"] = 6
	expectedMap["announce"] = 7
	expectedMap["nts_auth_ok"] = 8
	expectedMap["nts_auth_failed"] = 9
	expectedMap["nts_cookie_open_failed"] = 10

	require.Equal(t, expectedMap, result)
}
