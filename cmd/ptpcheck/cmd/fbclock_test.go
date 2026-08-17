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

package cmd

import (
	"bytes"
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/facebook/time/fbclock"

	"github.com/stretchr/testify/require"
)

// The fbagent collector's grep, verbatim from
// opsfiles/.../fb_fbagent/files/default/collectors/ptp/ptp_fbclock.sh.
var collectorFilter = regexp.MustCompile(`(earliest_ns|latest_ns|requests)`)

// Every key one run can print, built the way fbclockRun builds it.
func stdoutKeys(t *testing.T) []string {
	t.Helper()
	out := map[string]int64{}
	fbclockSample(&fbclock.TrueTime{
		Earliest: time.Unix(0, 1648137249050666302),
		Latest:   time.Unix(0, 1648137249050666313),
	}, "ptp.fbclock_synthetic.api.", out)
	fbclockMetrics(fbclock.Stats{Requests: 10000, Errors: 639}, "ptp.fbclock_synthetic.api.", ".60", out)
	return slices.Sorted(maps.Keys(out))
}

// Each key is one ODS timeseries on every PTP client host, so adding one is a
// fleet-wide decision. Pinned so it happens in review.
func TestFbclockMetricKeys(t *testing.T) {
	require.Equal(t, []string{
		"ptp.fbclock_synthetic.api.earliest_ns",
		"ptp.fbclock_synthetic.api.errors.sum.60",
		"ptp.fbclock_synthetic.api.latest_ns",
		"ptp.fbclock_synthetic.api.requests.sum.60",
		"ptp.fbclock_synthetic.api.wou_ge_1000us.sum.60",
		"ptp.fbclock_synthetic.api.wou_lt_1000us.sum.60",
		"ptp.fbclock_synthetic.api.wou_lt_100us.sum.60",
		"ptp.fbclock_synthetic.api.wou_lt_10us.sum.60",
		"ptp.fbclock_synthetic.api.wou_ns",
		"ptp.fbclock_synthetic.api.wou_ns.avg.60",
		"ptp.fbclock_synthetic.api.wou_ns.max.60",
	}, stdoutKeys(t))
}

// The collector greps whole lines out of a one-key-per-line JSON object. If the
// last key were one it drops, the line before keeps its trailing comma and
// fbagent discards every ptp.fbclock_synthetic.* metric for the host.
func TestFbclockStdoutSurvivesCollectorFilter(t *testing.T) {
	keys := stdoutKeys(t)
	require.NotEmpty(t, keys)
	require.NotRegexp(t, collectorFilter, keys[len(keys)-1])

	// That rests on json.Marshal sorting keys, so read the order off the wire.
	out := map[string]int64{}
	for i, k := range keys {
		out[k] = int64(i)
	}
	raw, err := json.Marshal(out)
	require.NoError(t, err)

	dec := json.NewDecoder(bytes.NewReader(raw))
	_, err = dec.Token()
	require.NoError(t, err)
	var order []string
	for dec.More() {
		name, err := dec.Token()
		require.NoError(t, err)
		order = append(order, name.(string))
		var v int64
		require.NoError(t, dec.Decode(&v))
	}
	require.Equal(t, keys, order)
}

func TestFormatErrorCauses(t *testing.T) {
	require.Empty(t, formatErrorCauses(nil))
	require.Empty(t, formatErrorCauses(map[string]int64{}))

	// Ordered so the same fleet condition prints byte-identically twice.
	got := formatErrorCauses(map[string]int64{
		"WOU is too big":                            7,
		"no data from daemon error":                 639,
		"shared memory data too old to extrapolate": 7,
	})
	require.Equal(t, "  639\tno data from daemon error\n  7\tWOU is too big\n  7\tshared memory data too old to extrapolate\n", got)
}
