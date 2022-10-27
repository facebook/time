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

package fbclock

import "C"

import (
	"time"
)

// Stats aggregate stats for fbclock GetTime results
type Stats struct {
	Requests    int64
	Errors      int64
	WOUAvg      int64
	WOUMax      int64
	WOUlt10us   int64
	WOUlt100us  int64
	WOUlt1000us int64
	WOUge1000us int64
}

// StatsCollector collects stats based on GetTime results
type StatsCollector struct {
	stats Stats
	// internal stuff
	wouSum int64
}

// Update processes result of GetTime call and updates Stats
func (s *StatsCollector) Update(tt *TrueTime, err error) {
	s.stats.Requests++
	if err != nil {
		s.stats.Errors++
		return
	}
	wou := tt.Latest.Sub(tt.Earliest)
	s.wouSum += int64(wou)
	s.stats.WOUAvg = s.wouSum / (s.stats.Requests - s.stats.Errors)
	if wou < 10*time.Microsecond {
		s.stats.WOUlt10us++
	} else if wou < 100*time.Microsecond {
		s.stats.WOUlt100us++
	} else if wou < 1000*time.Microsecond {
		s.stats.WOUlt1000us++
	} else {
		s.stats.WOUge1000us++
	}
	if int64(wou) > s.stats.WOUMax {
		s.stats.WOUMax = int64(wou)
	}
}

// Stats returns collected stats
func (s *StatsCollector) Stats() Stats {
	return s.stats
}
