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
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	gmstats "github.com/facebook/time/ptp/sptp/stats"
	"github.com/shirou/gopsutil/process"
)

// StatsServer is a stats server interface
type StatsServer interface {
	SetGmsTotal(gmsTotal int)
	SetGmsAvailable(gmsAvailable int)
	SetTickDuration(tickDuration time.Duration)
	IncRXSync()
	IncRXAnnounce()
	IncRXDelayReq()
	IncTXDelayReq()
	IncUnsupported()
	SetGMStats(stat *gmstats.Stat)
	CollectSysStats() error
}

// Stats is an implementation of
type Stats struct {
	sync.Mutex

	clientStats
	sysStats
	gmStats       gmstats.Stats
	procStartTime time.Time
	memstats      runtime.MemStats
}

// clientStats is just a grouping, don't use directly
type clientStats struct {
	gmsTotal     int64
	gmsAvailable int64
	tickDuration int64
	rxSync       int64
	rxAnnounce   int64
	rxDelayReq   int64
	txDelayReq   int64
	unsupported  int64
}

// sysStats is just a grouping, don't use directly
type sysStats struct {
	uptimeSec      int64
	cpuPCT         int64
	rss            int64
	goRoutines     int64
	gcPauseNs      int64
	gcPauseTotalNs int64
}

// NewStats created new instance of Stats
func NewStats() *Stats {
	return &Stats{
		gmStats:       gmstats.Stats{},
		procStartTime: time.Now(),
	}
}

// SetGmsTotal atomically sets the gmsTotal
func (s *Stats) SetGmsTotal(gmsTotal int) {
	atomic.StoreInt64(&s.gmsTotal, int64(gmsTotal))
}

// SetGmsAvailable atomically sets the gmsTotal
func (s *Stats) SetGmsAvailable(gmsAvailable int) {
	atomic.StoreInt64(&s.gmsAvailable, int64(gmsAvailable))
}

// SetTickDuration atomically sets the gmsTotal
func (s *Stats) SetTickDuration(tickDuration time.Duration) {
	atomic.StoreInt64(&s.tickDuration, tickDuration.Nanoseconds())
}

// IncRXSync atomically adds 1 to the rxsync
func (s *Stats) IncRXSync() {
	atomic.AddInt64(&s.rxSync, 1)
}

// IncRXAnnounce atomically adds 1 to the rxAnnounce
func (s *Stats) IncRXAnnounce() {
	atomic.AddInt64(&s.rxAnnounce, 1)
}

// IncRXDelayReq atomically adds 1 to the rxDelayReq
func (s *Stats) IncRXDelayReq() {
	atomic.AddInt64(&s.rxDelayReq, 1)
}

// IncTXDelayReq atomically adds 1 to the txDelayReq
func (s *Stats) IncTXDelayReq() {
	atomic.AddInt64(&s.txDelayReq, 1)
}

// IncUnsupported atomically adds 1 to the unsupported
func (s *Stats) IncUnsupported() {
	atomic.AddInt64(&s.unsupported, 1)
}

// GetCounters returns an map of counters
func (s *Stats) GetCounters() map[string]int64 {
	s.Lock()
	defer s.Unlock()

	return map[string]int64{
		// clientStats
		"ptp.sptp.gms.total":                s.gmsTotal,
		"ptp.sptp.gms.available_pct":        s.gmsAvailable,
		"ptp.sptp.tick_duration_ns":         s.tickDuration,
		"ptp.sptp.portstats.rx.sync":        s.rxSync,
		"ptp.sptp.portstats.rx.announce":    s.rxAnnounce,
		"ptp.sptp.portstats.rx.delay_req":   s.rxDelayReq,
		"ptp.sptp.portstats.tx.delay_req":   s.txDelayReq,
		"ptp.sptp.portstats.rx.unsupported": s.unsupported,
		// sysStats
		"ptp.sptp.runtime.gc.pause_ns.sum.60":    s.gcPauseNs,
		"ptp.sptp.runtime.mem.gc.pause_total_ns": s.gcPauseTotalNs,
		"ptp.sptp.runtime.cpu.goroutines":        s.goRoutines,
		"ptp.sptp.process.rss":                   s.rss,
		"ptp.sptp.process.cpu_pct.avg.60":        s.cpuPCT,
		"ptp.sptp.process.uptime":                s.uptimeSec,
	}
}

// GetGMStats returns an all gm stats
func (s *Stats) GetGMStats() gmstats.Stats {
	ret := make(gmstats.Stats, len(s.gmStats))
	s.Lock()
	copy(ret, s.gmStats)
	s.Unlock()
	return ret
}

// SetGMStats sets GM stats for particular gm
func (s *Stats) SetGMStats(stat *gmstats.Stat) {
	s.Lock()
	if i := s.gmStats.Index(stat); i != -1 {
		s.gmStats[i] = stat
	} else {
		s.gmStats = append(s.gmStats, stat)
	}
	s.Unlock()
}

// CollectSysStats gathers cpu, mem, gc statistics
func (s *Stats) CollectSysStats() error {
	s.Lock()
	defer s.Unlock()

	runtime.ReadMemStats(&s.memstats)

	// Process metrics
	proc, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		return err
	}
	s.uptimeSec = time.Now().Unix() - s.procStartTime.Unix()

	if val, err := proc.Percent(0); err == nil {
		s.cpuPCT = int64(val * 100)
	}

	if val, err := proc.MemoryInfo(); err == nil {
		s.rss = int64(val.RSS)
	}

	// Go Runtime metrics
	s.goRoutines = int64(runtime.NumGoroutine())
	// Diff between current and previous where s.gcPauseTotal acts as a previous
	s.gcPauseNs = int64(s.memstats.PauseTotalNs) - s.gcPauseTotalNs
	s.gcPauseTotalNs = int64(s.memstats.PauseTotalNs)

	return nil
}

func runResultToStats(address string, r *RunResult, p3 int, selected bool) *gmstats.Stat {
	s := &gmstats.Stat{
		GMAddress: address,
		Priority3: uint8(p3),
	}

	if r.Error != nil {
		s.GMPresent = 0
		s.Selected = false
		s.Error = r.Error.Error()
		return s
	}

	if r.Measurement == nil {
		s.Error = "Measurement is missing on RunResult"
		return s
	}
	s.GMPresent = 1
	s.PortIdentity = r.Measurement.Announce.GrandmasterIdentity.String()
	s.ClockQuality = r.Measurement.Announce.GrandmasterClockQuality
	s.Priority1 = r.Measurement.Announce.GrandmasterPriority1
	s.Priority2 = r.Measurement.Announce.GrandmasterPriority2
	s.Offset = float64(r.Measurement.Offset)
	s.MeanPathDelay = float64(r.Measurement.Delay)
	s.StepsRemoved = int(r.Measurement.Announce.StepsRemoved) + 1 // we are one step away from GM
	s.IngressTime = r.Measurement.Timestamp.UnixNano()
	s.CorrectionFieldRX = r.Measurement.CorrectionFieldRX.Nanoseconds()
	s.CorrectionFieldTX = r.Measurement.CorrectionFieldTX.Nanoseconds()
	s.C2SDelay = r.Measurement.C2SDelay.Nanoseconds()
	s.S2CDelay = r.Measurement.S2CDelay.Nanoseconds()
	if selected {
		s.Selected = true
	}
	return s
}
