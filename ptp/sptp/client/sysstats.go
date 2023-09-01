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
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/process"
)

var procStartTime = time.Now()

// SysStats represents Sys Stats
type SysStats struct {
	memstats *runtime.MemStats
}

// setRate is a helper function to make a crude rate/diff
func setRate(name string, counts map[string]uint64, cur, prev uint64, interval time.Duration) {
	if prev > cur {
		return
	}
	secs := uint64(interval.Seconds())
	counts[fmt.Sprintf("%s.sum.%d", name, secs)] = cur - prev
	counts[fmt.Sprintf("%s.rate.%d", name, secs)] = (cur - prev) / secs
}

// CollectRuntimeStats gathers cpu, mem, gc statistics
func (s *SysStats) CollectRuntimeStats(interval time.Duration) (map[string]uint64, error) {
	stats := make(map[string]uint64)
	m := &runtime.MemStats{}
	runtime.ReadMemStats(m)
	lastStats := s.memstats

	// Process metrics
	proc, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		return nil, err
	}
	stats["process.uptime"] = uint64(time.Now().Unix() - procStartTime.Unix())

	if val, err := proc.Percent(0); err == nil {
		stats[fmt.Sprintf("process.cpu_pct.avg.%d", int(interval.Seconds()))] = uint64(val * 100)
	}

	if val, err := proc.MemoryInfo(); err == nil {
		stats["process.rss"] = val.RSS
		stats["process.vms"] = val.VMS
		stats["process.swap"] = val.Swap
	}

	if val, err := proc.NumFDs(); err == nil {
		stats["process.num_fds"] = uint64(val)
	}

	if val, err := proc.NumThreads(); err == nil {
		stats["process.num_threads"] = uint64(val)
	}

	// Go Runtime metrics
	stats["runtime.cpu.goroutines"] = uint64(runtime.NumGoroutine())
	stats["runtime.cpu.cgo_calls"] = uint64(runtime.NumCgoCall())
	stats["runtime.mem.alloc"] = m.Alloc
	stats["runtime.mem.total"] = m.TotalAlloc
	stats["runtime.mem.sys"] = m.Sys
	stats["runtime.mem.lookups"] = m.Lookups
	stats["runtime.mem.malloc"] = m.Mallocs
	stats["runtime.mem.frees"] = m.Frees

	stats["runtime.mem.heap.alloc"] = m.HeapAlloc
	stats["runtime.mem.heap.sys"] = m.HeapSys
	stats["runtime.mem.heap.idle"] = m.HeapIdle
	stats["runtime.mem.heap.inuse"] = m.HeapInuse
	stats["runtime.mem.heap.released"] = m.HeapReleased
	stats["runtime.mem.heap.objects"] = m.HeapObjects

	stats["runtime.mem.stack.inuse"] = m.StackInuse
	stats["runtime.mem.stack.sys"] = m.StackSys
	stats["runtime.mem.stack.mspan_inuse"] = m.MSpanInuse
	stats["runtime.mem.stack.mspan_sys"] = m.MSpanSys
	stats["runtime.mem.stack.mcache_inuse"] = m.MCacheInuse
	stats["runtime.mem.stack.mcache_sys"] = m.MCacheSys

	stats["runtime.mem.othersys"] = m.OtherSys
	stats["runtime.mem.gc.sys"] = m.GCSys
	stats["runtime.mem.gc.next"] = m.NextGC
	stats["runtime.mem.gc.last"] = m.LastGC
	stats["runtime.mem.gc.pause_total"] = m.PauseTotalNs
	stats["runtime.mem.gc.pause"] = m.PauseNs[(m.NumGC+255)%256]
	stats["runtime.mem.gc.count"] = uint64(m.NumGC)
	if lastStats != nil {
		setRate("runtime.lookups", stats, m.Lookups, lastStats.Lookups, interval)

		setRate("runtime.mem.total_alloc", stats, m.Mallocs, lastStats.Mallocs, interval)
		setRate("runtime.mem.mallocs", stats, m.Mallocs, lastStats.Mallocs, interval)
		setRate("runtime.mem.frees", stats, m.Frees, lastStats.Frees, interval)

		setRate("runtime.gc.pause_ns", stats, m.PauseTotalNs, lastStats.PauseTotalNs, interval)
		setRate("runtime.gc.count", stats, uint64(m.NumGC), uint64(lastStats.NumGC), interval)
	}
	s.memstats = m
	return stats, nil
}
