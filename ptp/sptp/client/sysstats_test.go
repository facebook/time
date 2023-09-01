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

package client

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/exp/maps"
)

var expectedNonAggregateKeys = []string{"runtime.cpu.cgo_calls", "runtime.cpu.goroutines", "process.cpu_pct.avg.1", "runtime.mem.alloc", "runtime.mem.frees", "runtime.mem.gc.count", "runtime.mem.gc.last", "runtime.mem.gc.next", "runtime.mem.gc.pause", "runtime.mem.gc.pause_total", "runtime.mem.gc.sys", "runtime.mem.heap.alloc", "runtime.mem.heap.idle", "runtime.mem.heap.inuse", "runtime.mem.heap.objects", "runtime.mem.heap.released", "runtime.mem.heap.sys", "runtime.mem.lookups", "runtime.mem.malloc", "runtime.mem.othersys", "runtime.mem.stack.inuse", "runtime.mem.stack.mcache_inuse", "runtime.mem.stack.mcache_sys", "runtime.mem.stack.mspan_inuse", "runtime.mem.stack.mspan_sys", "runtime.mem.stack.sys", "runtime.mem.sys", "runtime.mem.total", "process.num_fds", "process.num_threads", "process.rss", "process.swap", "process.uptime", "process.vms"}
var expectedAggKeys = []string{"runtime.mem.mallocs.rate.1", "runtime.mem.mallocs.sum.1", "runtime.mem.total_alloc.rate.1", "runtime.mem.total_alloc.sum.1", "runtime.gc.count.rate.1", "runtime.gc.count.sum.1", "runtime.gc.pause_ns.rate.1", "runtime.gc.pause_ns.sum.1", "runtime.lookups.rate.1", "runtime.lookups.sum.1", "runtime.mem.frees.rate.1", "runtime.mem.frees.sum.1"}

func TestSysStats(t *testing.T) {
	stats := SysStats{}
	interval := time.Second

	collected, err := stats.CollectRuntimeStats(interval)
	require.NoError(t, err)
	keys := maps.Keys(collected)
	require.ElementsMatch(t, keys, expectedNonAggregateKeys)

	// Run collection again to get aggregated metrics too
	collected, err = stats.CollectRuntimeStats(interval)
	require.NoError(t, err)
	keys = maps.Keys(collected)
	require.ElementsMatch(t, keys, append(expectedNonAggregateKeys, expectedAggKeys...))
}

func TestSetRate(t *testing.T) {
	stats := make(map[string]uint64)
	intervaltime := time.Second * time.Duration(5)
	setRate("test", stats, 20, 1, intervaltime)

	expected := map[string]uint64{
		"test.sum.5":  19,
		"test.rate.5": 3,
	}
	require.Equal(t, expected, stats)
}
