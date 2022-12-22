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

package daemon

import (
	"sync"
)

// StatsServer is a stats server interface
type StatsServer interface {
	// Reset atomically sets all the counters to 0
	Reset()
	SetCounter(key string, val int64)
	UpdateCounterBy(key string, count int64)
}

// Stats is an implementation of
type Stats struct {
	mux      sync.Mutex
	counters map[string]int64
}

// NewStats created new instance of Stats
func NewStats() *Stats {
	return &Stats{
		counters: map[string]int64{},
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
