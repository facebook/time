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

/*
Package stats implements statistics collection and reporting.
It is used by server to report internal statistics, such as number of
requests and responses.
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
type JSONStats struct {
	// keep these aligned to 64-bit for sync/atomic
	invalidFormat int64
	requests      int64
	responses     int64
	listeners     int64
	workers       int64
	readError     int64
	announce      int64
}

// toMap converts struct to a map
func (j *JSONStats) toMap() (export map[string]int64) {
	export = make(map[string]int64)

	export["invalidformat"] = j.invalidFormat
	export["requests"] = j.requests
	export["responses"] = j.responses
	export["listeners"] = j.listeners
	export["workers"] = j.workers
	export["readError"] = j.readError
	export["announce"] = j.announce

	return export
}

// handleRequest is a handler used for all http monitoring requests
func (j *JSONStats) handleRequest(w http.ResponseWriter, _ *http.Request) {
	js, err := json.Marshal(j.toMap())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(js); err != nil {
		log.Errorf("Failed to reply: %v", err)
	}
}

// Start with launch 303 thrift and report ODS metrics periodically
func (j *JSONStats) Start(port int) {
	http.HandleFunc("/", j.handleRequest)
	addr := fmt.Sprintf(":%d", port)
	log.Debugf("Starting http json server on %s", addr)
	err := http.ListenAndServe(addr, nil)
	if err != nil {
		log.Errorf("Failed to start listener: %v", err)
	}
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

// IncReadError atomically add 1 to the counter
func (j *JSONStats) IncReadError() {
	atomic.AddInt64(&j.readError, 1)
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
