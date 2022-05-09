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
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"

	log "github.com/sirupsen/logrus"
)

// JSONStats is what we want to report as stats via http
type JSONStats struct {
	report counters

	counters
}

// NewJSONStats returns a new JSONStats
func NewJSONStats() *JSONStats {
	s := &JSONStats{}

	return s
}

// Start runs http server and initializes maps
func (s *JSONStats) Start(monitoringport int) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)
	addr := fmt.Sprintf(":%d", monitoringport)
	log.Infof("Starting http json server on %s", addr)
	err := http.ListenAndServe(addr, mux)
	if err != nil {
		log.Fatalf("Failed to start listener: %v", err)
	}
}

// Snapshot the values so they can be reported atomically
func (s *JSONStats) Snapshot() {
	s.report.utcOffset = s.utcOffset
	s.report.phcOffset = s.phcOffset
	s.report.oscillatorOffset = s.oscillatorOffset
	s.report.clockAccuracy = s.clockAccuracy
	s.report.clockClass = s.clockClass
	s.report.reload = s.reload
	s.report.dataError = s.dataError
}

// handleRequest is a handler used for all http monitoring requests
func (s *JSONStats) handleRequest(w http.ResponseWriter, r *http.Request) {
	js, err := json.Marshal(s.report.toMap())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(js); err != nil {
		log.Errorf("Failed to reply: %v", err)
	}
}

// IncReload atomically add 1 to the counter
func (s *JSONStats) IncReload() {
	atomic.AddInt64(&s.reload, 1)
}

// ResetReload atomically sets the counter to 0
func (s *JSONStats) ResetReload() {
	atomic.StoreInt64(&s.reload, 0)
}

// IncDataError atomically add 1 to the counter
func (s *JSONStats) IncDataError() {
	atomic.AddInt64(&s.dataError, 1)
}

// ResetDataError atomically sets the counter to 0
func (s *JSONStats) ResetDataError() {
	atomic.StoreInt64(&s.dataError, 0)
}

// SetUTCOffset atomically sets the utcoffset
func (s *JSONStats) SetUTCOffset(utcOffset int64) {
	atomic.StoreInt64(&s.utcOffset, utcOffset)
}

// SetPHCOffset atomically sets the phcoffset
func (s *JSONStats) SetPHCOffset(phcOffset int64) {
	atomic.StoreInt64(&s.phcOffset, phcOffset)
}

// SetOscillatorOffset atomically sets the oscillatoroffset
func (s *JSONStats) SetOscillatorOffset(oscillatorOffset int64) {
	atomic.StoreInt64(&s.oscillatorOffset, oscillatorOffset)
}

// SetClockAccuracy atomically sets the clock accuracy
func (s *JSONStats) SetClockAccuracy(clockAccuracy int64) {
	atomic.StoreInt64(&s.clockAccuracy, clockAccuracy)
}

// SetClockClass atomically sets the clock class
func (s *JSONStats) SetClockClass(clockClass int64) {
	atomic.StoreInt64(&s.clockClass, clockClass)
}
