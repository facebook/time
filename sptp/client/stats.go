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
	"sync"

	gmstats "github.com/facebook/time/sptp/stats"
)

// StatsServer is a stats server interface
type StatsServer interface {
	// Reset atomically sets all the counters to 0
	Reset()
	SetCounter(key string, val int64)
	UpdateCounterBy(key string, count int64)
	SetGMStats(gm string, stats *gmstats.Stats)
}

// Stats is an implementation of
type Stats struct {
	mux      sync.Mutex
	counters map[string]int64
	gmStats  map[string]*gmstats.Stats
}

// NewStats created new instance of Stats
func NewStats() *Stats {
	return &Stats{
		counters: map[string]int64{},
		gmStats:  map[string]*gmstats.Stats{},
	}
}

// UpdateCounterBy will increment counter
func (s *Stats) UpdateCounterBy(key string, count int64) {
	s.mux.Lock()
	s.counters[key] += count
	s.mux.Unlock()
}

// SetCounter will set a counter to the provided value.
func (s *Stats) SetCounter(key string, val int64) {
	s.mux.Lock()
	s.counters[key] = val
	s.mux.Unlock()
}

// Get returns an map of counters
func (s *Stats) Get() map[string]int64 {
	ret := make(map[string]int64)
	s.mux.Lock()
	for key, val := range s.counters {
		ret[key] = val
	}
	s.mux.Unlock()
	return ret
}

// Copy all key-values between maps
func (s *Stats) Copy(dst *Stats) {
	s.mux.Lock()
	for k, v := range s.counters {
		dst.SetCounter(k, v)
	}
	s.mux.Unlock()
}

// Reset all the values of counters
func (s *Stats) Reset() {
	s.mux.Lock()
	for k := range s.counters {
		s.counters[k] = 0
	}
	s.mux.Unlock()
}

// SetGMStats sets GM stats for particular gm
func (s *Stats) SetGMStats(gm string, stats *gmstats.Stats) {
	s.mux.Lock()
	s.gmStats[gm] = stats
	s.mux.Unlock()
}

func runResultToStats(r *RunResult, p3 int, selected bool) *gmstats.Stats {
	s := &gmstats.Stats{}
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
	s.Priority3 = uint8(p3)
	s.Offset = float64(r.Measurement.Offset)
	s.MeanPathDelay = float64(r.Measurement.Delay)
	s.StepsRemoved = int(r.Measurement.Announce.StepsRemoved)
	s.IngressTime = r.Measurement.Timestamp.UnixNano()
	s.CorrectionFieldRX = r.Measurement.CorrectionFieldRX.Nanoseconds()
	s.CorrectionFieldTX = r.Measurement.CorrectionFieldTX.Nanoseconds()
	if selected {
		s.Selected = true
	}
	return s
}
