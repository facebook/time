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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/stretchr/testify/require"
)

func TestStats(t *testing.T) {
	s0 := &Stat{GMAddress: "::1", Priority3: 2}
	s1 := &Stat{GMAddress: "::1", Priority3: 3}
	s2 := &Stat{GMAddress: "127.0.0.1", Priority3: 1}
	s3 := &Stat{GMAddress: "127.0.0.2", Priority3: 1}

	s := Stats{s0, s1, s2, s3}
	require.Equal(t, 4, s.Len())
	require.True(t, s.Less(0, 1))
	require.False(t, s.Less(1, 2))
	require.True(t, s.Less(2, 3))
	require.True(t, s.Less(2, 0))

	require.Equal(t, 2, s.Index(s2))
	require.Equal(t, -1, s.Index(&Stat{}))
}

func TestFetchStats(t *testing.T) {
	sampleResp := `
[
	{"gm_address": "127.0.0.1", "selected": false, "port_identity": "oleg", "clock_quality": {"clock_class": 6, "clock_accuracy": 33, "offset_scaled_log_variance": 42}, "priority1": 2, "priority2": 3, "priority3": 4, "offset": -42.42, "mean_path_delay": 42.42, "steps_removed": 3, "cf_rx": 10, "cf_tx": 20, "client_server_delay": 42, "server_client_delay": 24, "gm_present": 1, "error": ""},
	{"gm_address": "::1", "selected": true, "port_identity": "oleg1", "clock_quality": {"clock_class": 7, "clock_accuracy": 34, "offset_scaled_log_variance": 42}, "priority1": 2, "priority2": 3, "priority3": 4, "offset": -43.43, "mean_path_delay": 43.43, "steps_removed": 3, "cf_rx": 100000, "cf_tx": 20000, "client_server_delay": 42, "server_client_delay": 24, "gm_present": 0, "error": "oops"}
]
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	expected := Stats{
		{
			GMAddress:    "127.0.0.1",
			Selected:     false,
			PortIdentity: "oleg",
			ClockQuality: ptp.ClockQuality{
				ClockClass:              ptp.ClockClass6,
				ClockAccuracy:           ptp.ClockAccuracyNanosecond100,
				OffsetScaledLogVariance: uint16(42),
			},
			Priority1:         2,
			Priority2:         3,
			Priority3:         4,
			Offset:            -42.42,
			MeanPathDelay:     42.42,
			StepsRemoved:      3,
			CorrectionFieldRX: 10,
			CorrectionFieldTX: 20,
			C2SDelay:          42,
			S2CDelay:          24,
			GMPresent:         1,
		},
		{
			GMAddress:    "::1",
			Selected:     true,
			PortIdentity: "oleg1",
			ClockQuality: ptp.ClockQuality{
				ClockClass:              ptp.ClockClass7,
				ClockAccuracy:           ptp.ClockAccuracyNanosecond250,
				OffsetScaledLogVariance: uint16(42),
			},
			Priority1:         2,
			Priority2:         3,
			Priority3:         4,
			Offset:            -43.43,
			MeanPathDelay:     43.43,
			StepsRemoved:      3,
			CorrectionFieldRX: 100000,
			CorrectionFieldTX: 20000,
			C2SDelay:          42,
			S2CDelay:          24,
			GMPresent:         0,
			Error:             "oops",
		},
	}

	actual, err := FetchStats(ts.URL)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func TestFetchCounters(t *testing.T) {
	sampleResp := `{"ptp.sptp.gms.available_pct":100,"ptp.sptp.gms.total":4,"ptp.sptp.portstats.rx.announce":4656,"ptp.sptp.portstats.rx.sync":4656,"ptp.sptp.portstats.tx.delay_req":4656,"ptp.sptp.process.alive":1,"ptp.sptp.process.alive_since":1676549472,"ptp.sptp.process.cpu_pct.avg.60":0,"ptp.sptp.process.cpu_permil.avg.60":0,"ptp.sptp.process.num_fds":12,"ptp.sptp.process.num_threads":16,"ptp.sptp.process.rss":13713408,"ptp.sptp.process.swap":0,"ptp.sptp.process.uptime":1140,"ptp.sptp.process.vms":1865134080,"ptp.sptp.runtime.cpu.cgo_calls":1,"ptp.sptp.runtime.cpu.goroutines":10,"ptp.sptp.runtime.gc.count.rate.60":0,"ptp.sptp.runtime.gc.count.sum.60":1,"ptp.sptp.runtime.gc.pause_ns.rate.60":1665,"ptp.sptp.runtime.gc.pause_ns.sum.60":99943,"ptp.sptp.runtime.lookups.rate.60":0,"ptp.sptp.runtime.lookups.sum.60":0,"ptp.sptp.runtime.mem.alloc":2487032,"ptp.sptp.runtime.mem.frees":566856,"ptp.sptp.runtime.mem.frees.rate.60":523,"ptp.sptp.runtime.mem.frees.sum.60":31418,"ptp.sptp.runtime.mem.gc.count":19,"ptp.sptp.runtime.mem.gc.last":1676550571374514171,"ptp.sptp.runtime.mem.gc.next":4194304,"ptp.sptp.runtime.mem.gc.pause":99943,"ptp.sptp.runtime.mem.gc.pause_total":1987074,"ptp.sptp.runtime.mem.gc.sys":4868072,"ptp.sptp.runtime.mem.heap.alloc":2487032,"ptp.sptp.runtime.mem.heap.idle":2891776,"ptp.sptp.runtime.mem.heap.inuse":4349952,"ptp.sptp.runtime.mem.heap.objects":21678,"ptp.sptp.runtime.mem.heap.released":1826816,"ptp.sptp.runtime.mem.heap.sys":7241728,"ptp.sptp.runtime.mem.lookups":0,"ptp.sptp.runtime.mem.malloc":588534,"ptp.sptp.runtime.mem.mallocs.rate.60":514,"ptp.sptp.runtime.mem.mallocs.sum.60":30848,"ptp.sptp.runtime.mem.othersys":2935262,"ptp.sptp.runtime.mem.stack.inuse":1146880,"ptp.sptp.runtime.mem.stack.mcache_inuse":62400,"ptp.sptp.runtime.mem.stack.mcache_sys":62400,"ptp.sptp.runtime.mem.stack.mspan_inuse":172312,"ptp.sptp.runtime.mem.stack.mspan_sys":195840,"ptp.sptp.runtime.mem.stack.sys":1146880,"ptp.sptp.runtime.mem.sys":17908744,"ptp.sptp.runtime.mem.total":41251472,"ptp.sptp.runtime.mem.total_alloc.rate.60":514,"ptp.sptp.runtime.mem.total_alloc.sum.60":30848}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	expected := Counters{
		"ptp.sptp.gms.available_pct":               100,
		"ptp.sptp.gms.total":                       4,
		"ptp.sptp.portstats.rx.announce":           4656,
		"ptp.sptp.portstats.rx.sync":               4656,
		"ptp.sptp.portstats.tx.delay_req":          4656,
		"ptp.sptp.process.alive":                   1,
		"ptp.sptp.process.alive_since":             1676549472,
		"ptp.sptp.process.cpu_pct.avg.60":          0,
		"ptp.sptp.process.cpu_permil.avg.60":       0,
		"ptp.sptp.process.num_fds":                 12,
		"ptp.sptp.process.num_threads":             16,
		"ptp.sptp.process.rss":                     13713408,
		"ptp.sptp.process.swap":                    0,
		"ptp.sptp.process.uptime":                  1140,
		"ptp.sptp.process.vms":                     1865134080,
		"ptp.sptp.runtime.cpu.cgo_calls":           1,
		"ptp.sptp.runtime.cpu.goroutines":          10,
		"ptp.sptp.runtime.gc.count.rate.60":        0,
		"ptp.sptp.runtime.gc.count.sum.60":         1,
		"ptp.sptp.runtime.gc.pause_ns.rate.60":     1665,
		"ptp.sptp.runtime.gc.pause_ns.sum.60":      99943,
		"ptp.sptp.runtime.lookups.rate.60":         0,
		"ptp.sptp.runtime.lookups.sum.60":          0,
		"ptp.sptp.runtime.mem.alloc":               2487032,
		"ptp.sptp.runtime.mem.frees":               566856,
		"ptp.sptp.runtime.mem.frees.rate.60":       523,
		"ptp.sptp.runtime.mem.frees.sum.60":        31418,
		"ptp.sptp.runtime.mem.gc.count":            19,
		"ptp.sptp.runtime.mem.gc.last":             1676550571374514171,
		"ptp.sptp.runtime.mem.gc.next":             4194304,
		"ptp.sptp.runtime.mem.gc.pause":            99943,
		"ptp.sptp.runtime.mem.gc.pause_total":      1987074,
		"ptp.sptp.runtime.mem.gc.sys":              4868072,
		"ptp.sptp.runtime.mem.heap.alloc":          2487032,
		"ptp.sptp.runtime.mem.heap.idle":           2891776,
		"ptp.sptp.runtime.mem.heap.inuse":          4349952,
		"ptp.sptp.runtime.mem.heap.objects":        21678,
		"ptp.sptp.runtime.mem.heap.released":       1826816,
		"ptp.sptp.runtime.mem.heap.sys":            7241728,
		"ptp.sptp.runtime.mem.lookups":             0,
		"ptp.sptp.runtime.mem.malloc":              588534,
		"ptp.sptp.runtime.mem.mallocs.rate.60":     514,
		"ptp.sptp.runtime.mem.mallocs.sum.60":      30848,
		"ptp.sptp.runtime.mem.othersys":            2935262,
		"ptp.sptp.runtime.mem.stack.inuse":         1146880,
		"ptp.sptp.runtime.mem.stack.mcache_inuse":  62400,
		"ptp.sptp.runtime.mem.stack.mcache_sys":    62400,
		"ptp.sptp.runtime.mem.stack.mspan_inuse":   172312,
		"ptp.sptp.runtime.mem.stack.mspan_sys":     195840,
		"ptp.sptp.runtime.mem.stack.sys":           1146880,
		"ptp.sptp.runtime.mem.sys":                 17908744,
		"ptp.sptp.runtime.mem.total":               41251472,
		"ptp.sptp.runtime.mem.total_alloc.rate.60": 514,
		"ptp.sptp.runtime.mem.total_alloc.sum.60":  30848,
	}

	actual, err := FetchCounters(ts.URL)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func TestFetchPortStats(t *testing.T) {
	sampleResp := `{"ptp.sptp.gms.available_pct":100,"ptp.sptp.gms.total":4,"ptp.sptp.portstats.rx.announce":4656,"ptp.sptp.portstats.rx.sync":4656,"ptp.sptp.portstats.tx.delay_req":4656,"ptp.sptp.process.alive":1,"ptp.sptp.process.alive_since":1676549472,"ptp.sptp.process.cpu_pct.avg.60":0,"ptp.sptp.process.cpu_permil.avg.60":0,"ptp.sptp.process.num_fds":12,"ptp.sptp.process.num_threads":16,"ptp.sptp.process.rss":13713408,"ptp.sptp.process.swap":0,"ptp.sptp.process.uptime":1140,"ptp.sptp.process.vms":1865134080,"ptp.sptp.runtime.cpu.cgo_calls":1,"ptp.sptp.runtime.cpu.goroutines":10,"ptp.sptp.runtime.gc.count.rate.60":0,"ptp.sptp.runtime.gc.count.sum.60":1,"ptp.sptp.runtime.gc.pause_ns.rate.60":1665,"ptp.sptp.runtime.gc.pause_ns.sum.60":99943,"ptp.sptp.runtime.lookups.rate.60":0,"ptp.sptp.runtime.lookups.sum.60":0,"ptp.sptp.runtime.mem.alloc":2487032,"ptp.sptp.runtime.mem.frees":566856,"ptp.sptp.runtime.mem.frees.rate.60":523,"ptp.sptp.runtime.mem.frees.sum.60":31418,"ptp.sptp.runtime.mem.gc.count":19,"ptp.sptp.runtime.mem.gc.last":1676550571374514171,"ptp.sptp.runtime.mem.gc.next":4194304,"ptp.sptp.runtime.mem.gc.pause":99943,"ptp.sptp.runtime.mem.gc.pause_total":1987074,"ptp.sptp.runtime.mem.gc.sys":4868072,"ptp.sptp.runtime.mem.heap.alloc":2487032,"ptp.sptp.runtime.mem.heap.idle":2891776,"ptp.sptp.runtime.mem.heap.inuse":4349952,"ptp.sptp.runtime.mem.heap.objects":21678,"ptp.sptp.runtime.mem.heap.released":1826816,"ptp.sptp.runtime.mem.heap.sys":7241728,"ptp.sptp.runtime.mem.lookups":0,"ptp.sptp.runtime.mem.malloc":588534,"ptp.sptp.runtime.mem.mallocs.rate.60":514,"ptp.sptp.runtime.mem.mallocs.sum.60":30848,"ptp.sptp.runtime.mem.othersys":2935262,"ptp.sptp.runtime.mem.stack.inuse":1146880,"ptp.sptp.runtime.mem.stack.mcache_inuse":62400,"ptp.sptp.runtime.mem.stack.mcache_sys":62400,"ptp.sptp.runtime.mem.stack.mspan_inuse":172312,"ptp.sptp.runtime.mem.stack.mspan_sys":195840,"ptp.sptp.runtime.mem.stack.sys":1146880,"ptp.sptp.runtime.mem.sys":17908744,"ptp.sptp.runtime.mem.total":41251472,"ptp.sptp.runtime.mem.total_alloc.rate.60":514,"ptp.sptp.runtime.mem.total_alloc.sum.60":30848}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	expectedTX := map[string]uint64{
		"delay_req": 4656,
	}
	expectedRX := map[string]uint64{
		"announce": 4656,
		"sync":     4656,
	}

	actualTX, actualRX, err := FetchPortStats(ts.URL)
	require.NoError(t, err)
	require.Equal(t, expectedTX, actualTX)
	require.Equal(t, expectedRX, actualRX)
}

func TestFetchSysStats(t *testing.T) {
	sampleResp := `{"ptp.sptp.gms.available_pct":100,"ptp.sptp.gms.total":4,"ptp.sptp.portstats.rx.announce":4656,"ptp.sptp.portstats.rx.sync":4656,"ptp.sptp.portstats.tx.delay_req":4656,"ptp.sptp.process.alive":1,"ptp.sptp.process.alive_since":1676549472,"ptp.sptp.process.cpu_pct.avg.60":0,"ptp.sptp.process.cpu_permil.avg.60":0,"ptp.sptp.process.num_fds":12,"ptp.sptp.process.num_threads":16,"ptp.sptp.process.rss":13713408,"ptp.sptp.process.swap":0,"ptp.sptp.process.uptime":1140,"ptp.sptp.process.vms":1865134080,"ptp.sptp.runtime.cpu.cgo_calls":1,"ptp.sptp.runtime.cpu.goroutines":10,"ptp.sptp.runtime.gc.count.rate.60":0,"ptp.sptp.runtime.gc.count.sum.60":1,"ptp.sptp.runtime.gc.pause_ns.rate.60":1665,"ptp.sptp.runtime.gc.pause_ns.sum.60":99943,"ptp.sptp.runtime.lookups.rate.60":0,"ptp.sptp.runtime.lookups.sum.60":0,"ptp.sptp.runtime.mem.alloc":2487032,"ptp.sptp.runtime.mem.frees":566856,"ptp.sptp.runtime.mem.frees.rate.60":523,"ptp.sptp.runtime.mem.frees.sum.60":31418,"ptp.sptp.runtime.mem.gc.count":19,"ptp.sptp.runtime.mem.gc.last":1676550571374514171,"ptp.sptp.runtime.mem.gc.next":4194304,"ptp.sptp.runtime.mem.gc.pause":99943,"ptp.sptp.runtime.mem.gc.pause_total":1987074,"ptp.sptp.runtime.mem.gc.sys":4868072,"ptp.sptp.runtime.mem.heap.alloc":2487032,"ptp.sptp.runtime.mem.heap.idle":2891776,"ptp.sptp.runtime.mem.heap.inuse":4349952,"ptp.sptp.runtime.mem.heap.objects":21678,"ptp.sptp.runtime.mem.heap.released":1826816,"ptp.sptp.runtime.mem.heap.sys":7241728,"ptp.sptp.runtime.mem.lookups":0,"ptp.sptp.runtime.mem.malloc":588534,"ptp.sptp.runtime.mem.mallocs.rate.60":514,"ptp.sptp.runtime.mem.mallocs.sum.60":30848,"ptp.sptp.runtime.mem.othersys":2935262,"ptp.sptp.runtime.mem.stack.inuse":1146880,"ptp.sptp.runtime.mem.stack.mcache_inuse":62400,"ptp.sptp.runtime.mem.stack.mcache_sys":62400,"ptp.sptp.runtime.mem.stack.mspan_inuse":172312,"ptp.sptp.runtime.mem.stack.mspan_sys":195840,"ptp.sptp.runtime.mem.stack.sys":1146880,"ptp.sptp.runtime.mem.sys":17908744,"ptp.sptp.runtime.mem.total":41251472,"ptp.sptp.runtime.mem.total_alloc.rate.60":514,"ptp.sptp.runtime.mem.total_alloc.sum.60":30848}`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	expected := map[string]int64{
		"ptp.sptp.gms.available_pct":               100,
		"ptp.sptp.gms.total":                       4,
		"ptp.sptp.process.alive":                   1,
		"ptp.sptp.process.alive_since":             1676549472,
		"ptp.sptp.process.cpu_pct.avg.60":          0,
		"ptp.sptp.process.cpu_permil.avg.60":       0,
		"ptp.sptp.process.num_fds":                 12,
		"ptp.sptp.process.num_threads":             16,
		"ptp.sptp.process.rss":                     13713408,
		"ptp.sptp.process.swap":                    0,
		"ptp.sptp.process.uptime":                  1140,
		"ptp.sptp.process.vms":                     1865134080,
		"ptp.sptp.runtime.cpu.cgo_calls":           1,
		"ptp.sptp.runtime.cpu.goroutines":          10,
		"ptp.sptp.runtime.gc.count.rate.60":        0,
		"ptp.sptp.runtime.gc.count.sum.60":         1,
		"ptp.sptp.runtime.gc.pause_ns.rate.60":     1665,
		"ptp.sptp.runtime.gc.pause_ns.sum.60":      99943,
		"ptp.sptp.runtime.lookups.rate.60":         0,
		"ptp.sptp.runtime.lookups.sum.60":          0,
		"ptp.sptp.runtime.mem.alloc":               2487032,
		"ptp.sptp.runtime.mem.frees":               566856,
		"ptp.sptp.runtime.mem.frees.rate.60":       523,
		"ptp.sptp.runtime.mem.frees.sum.60":        31418,
		"ptp.sptp.runtime.mem.gc.count":            19,
		"ptp.sptp.runtime.mem.gc.last":             1676550571374514171,
		"ptp.sptp.runtime.mem.gc.next":             4194304,
		"ptp.sptp.runtime.mem.gc.pause":            99943,
		"ptp.sptp.runtime.mem.gc.pause_total":      1987074,
		"ptp.sptp.runtime.mem.gc.sys":              4868072,
		"ptp.sptp.runtime.mem.heap.alloc":          2487032,
		"ptp.sptp.runtime.mem.heap.idle":           2891776,
		"ptp.sptp.runtime.mem.heap.inuse":          4349952,
		"ptp.sptp.runtime.mem.heap.objects":        21678,
		"ptp.sptp.runtime.mem.heap.released":       1826816,
		"ptp.sptp.runtime.mem.heap.sys":            7241728,
		"ptp.sptp.runtime.mem.lookups":             0,
		"ptp.sptp.runtime.mem.malloc":              588534,
		"ptp.sptp.runtime.mem.mallocs.rate.60":     514,
		"ptp.sptp.runtime.mem.mallocs.sum.60":      30848,
		"ptp.sptp.runtime.mem.othersys":            2935262,
		"ptp.sptp.runtime.mem.stack.inuse":         1146880,
		"ptp.sptp.runtime.mem.stack.mcache_inuse":  62400,
		"ptp.sptp.runtime.mem.stack.mcache_sys":    62400,
		"ptp.sptp.runtime.mem.stack.mspan_inuse":   172312,
		"ptp.sptp.runtime.mem.stack.mspan_sys":     195840,
		"ptp.sptp.runtime.mem.stack.sys":           1146880,
		"ptp.sptp.runtime.mem.sys":                 17908744,
		"ptp.sptp.runtime.mem.total":               41251472,
		"ptp.sptp.runtime.mem.total_alloc.rate.60": 514,
		"ptp.sptp.runtime.mem.total_alloc.sum.60":  30848,
	}

	actual, err := FetchSysStats(ts.URL)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}
