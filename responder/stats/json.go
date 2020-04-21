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

// JSONStats implements Stat interface
// This implementation reports JSON metrics via http interface
// This is a passive implementation. Only "Start" needs to be called
// Report will do nothing
type JSONStats struct {
	// keep these aligned to 64-bit for sync/atomic
	invalidFormat int64
	requests      int64
	responses     int64
	listeners     int64
	workers       int64
	announce      int64

	prefix string
}

func (j *JSONStats) toMap() (export map[string]int64) {
	export = make(map[string]int64)

	export[fmt.Sprintf("%sinvalidformat", j.prefix)] = j.invalidFormat
	export[fmt.Sprintf("%srequests", j.prefix)] = j.requests
	export[fmt.Sprintf("%sresponses", j.prefix)] = j.responses
	export[fmt.Sprintf("%slisteners", j.prefix)] = j.listeners
	export[fmt.Sprintf("%sworkers", j.prefix)] = j.workers
	export[fmt.Sprintf("%sannounce", j.prefix)] = j.announce

	return export
}

func (j *JSONStats) handleRequest(w http.ResponseWriter, r *http.Request) {
	js, err := json.Marshal(j.toMap())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

// Start with launch 303 thrift and report ODS metrics periodically
func (j *JSONStats) Start(port int) {
	http.HandleFunc("/", j.handleRequest)
	addr := fmt.Sprintf(":%d", port)
	log.Debugf("Starting http json server on %s", addr)
	http.ListenAndServe(addr, nil)
}

// SetPrefix is implementing SetPrefix function of interface
func (j *JSONStats) SetPrefix(prefix string) {
	j.prefix = prefix
}

// Report is implementing Report function of interface
// As JSONStats a passive reporter, Report will do nothing
func (j *JSONStats) Report() error {
	return nil
}

// IncInvalidFormat atomically add 1 to the counter
func (j *JSONStats) IncInvalidFormat() {
	atomic.AddInt64(&j.invalidFormat, 1)
}

// IncRequests atomically add 1 to the counter
func (j *JSONStats) IncRequests() {
	atomic.AddInt64(&j.requests, 1)
}

// IncResponses atomically add 1 to the counter
func (j *JSONStats) IncResponses() {
	atomic.AddInt64(&j.responses, 1)
}

// IncListeners atomically add 1 to the counter
func (j *JSONStats) IncListeners() {
	atomic.AddInt64(&j.listeners, 1)
}

// IncWorkers atomically add 1 to the counter
func (j *JSONStats) IncWorkers() {
	atomic.AddInt64(&j.workers, 1)
}

// DecListeners atomically removes 1 from the counter
func (j *JSONStats) DecListeners() {
	atomic.AddInt64(&j.listeners, -1)
}

// DecWorkers atomically removes 1 from the counter
func (j *JSONStats) DecWorkers() {
	atomic.AddInt64(&j.workers, -1)
}

// SetAnnounce atomically sets counter to 1
func (j *JSONStats) SetAnnounce() {
	atomic.StoreInt64(&j.announce, 1)
}

// ResetAnnounce atomically sets counter to 0
func (j *JSONStats) ResetAnnounce() {
	atomic.StoreInt64(&j.announce, 0)
}
