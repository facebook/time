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
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/facebook/time/ptp/sptp/client"

	"github.com/stretchr/testify/require"
)

// The fbagent collector's grep, verbatim from
// opsfiles/.../fb_fbagent/files/default/collectors/ptp/ptp_servicestats.sh.
var servicestatsCollectorFilter = regexp.MustCompile(
	`(num_threads|swap|cgo_calls|runtime\.lookups|runtime\.mem|gc.count|vms|num_fds)`)

// What `ptpcheck servicestats` prints for sptp, captured off the command's own
// stdout against a fake /counters endpoint.
func servicestatsStdout(t *testing.T, counters map[string]int64) []byte {
	t.Helper()
	body, err := json.Marshal(counters)
	require.NoError(t, err)

	pathCh := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathCh <- r.URL.Path
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	readStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() {
		os.Stdout = readStdout
		_ = r.Close()
	})
	os.Stdout = w

	runErr := serviceStatsRunSPTP(srv.URL)
	require.NoError(t, w.Close())
	os.Stdout = readStdout
	require.NoError(t, runErr)
	require.Equal(t, "/counters", <-pathCh)

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return out
}

// What fbagent stores: stdout through the collector's `jq . | grep -vE ...`. The
// final unmarshal is the guard -- a dropped last key leaves a trailing comma.
func servicestatsCollected(t *testing.T, stdout []byte) map[string]int64 {
	t.Helper()
	var printed map[string]int64
	require.NoError(t, json.Unmarshal(stdout, &printed))
	pretty, err := json.MarshalIndent(printed, "", "  ")
	require.NoError(t, err)

	var kept []string
	for line := range strings.SplitSeq(string(pretty), "\n") {
		if !servicestatsCollectorFilter.MatchString(line) {
			kept = append(kept, line)
		}
	}

	var got map[string]int64
	require.NoError(t, json.Unmarshal([]byte(strings.Join(kept, "\n")), &got))
	return got
}

func TestServiceStatsRunSPTPReportsPortStats(t *testing.T) {
	counters := map[string]int64{
		"ptp.sptp.gms.available_pct":        0,
		"ptp.sptp.portstats.rx.announce":    12,
		"ptp.sptp.portstats.rx.delay_req":   118,
		"ptp.sptp.portstats.rx.sync":        13,
		"ptp.sptp.portstats.rx.unsupported": 7,
		"ptp.sptp.portstats.tx.delay_req":   14,
	}

	var got map[string]int64
	require.NoError(t, json.Unmarshal(servicestatsStdout(t, counters), &got))
	require.Equal(t, counters, got)
}

// Each key is one ODS timeseries on every PTP client host, so adding one is a
// fleet-wide decision. Pinned so it happens in review.
func TestServiceStatsMetricKeys(t *testing.T) {
	s, err := client.NewStats()
	require.NoError(t, err)

	collected := servicestatsCollected(t, servicestatsStdout(t, s.GetCounters()))
	require.Equal(t, []string{
		"ptp.sptp.filtered",
		"ptp.sptp.gms.available_pct",
		"ptp.sptp.gms.total",
		"ptp.sptp.ping.errors",
		"ptp.sptp.ping.rejected",
		"ptp.sptp.ping.requests",
		"ptp.sptp.port_change_count",
		"ptp.sptp.portstats.rx.announce",
		"ptp.sptp.portstats.rx.delay_req",
		"ptp.sptp.portstats.rx.sync",
		"ptp.sptp.portstats.rx.unsupported",
		"ptp.sptp.portstats.tx.delay_req",
		"ptp.sptp.process.cpu_pct.avg.60",
		"ptp.sptp.process.rss",
		"ptp.sptp.process.uptime",
		"ptp.sptp.runtime.cpu.goroutines",
		"ptp.sptp.runtime.gc.pause_ns.sum.60",
		"ptp.sptp.servo.state",
	}, slices.Sorted(maps.Keys(collected)))
}
