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
	"time"

	log "github.com/sirupsen/logrus"
)

// JSONStats implements Stat interface
// This implementation reports JSON metrics via http interface
// This is a passive implementation. Only "Start" needs to be called
type JSONStats struct {
	invalidFormat       atomic.Int64
	requests            atomic.Int64
	responses           atomic.Int64
	listeners           atomic.Int64
	workers             atomic.Int64
	readError           atomic.Int64
	announce            atomic.Int64
	ntsAuthOK           atomic.Int64
	ntsAuthFailed       atomic.Int64
	ntsCookieOpenFailed atomic.Int64
	ntsCookieExpired    atomic.Int64
	ntsCookieFuture     atomic.Int64
}

// ToMap converts struct to a map
func (j *JSONStats) ToMap() (export map[string]int64) {
	export = make(map[string]int64)

	export["invalidformat"] = j.invalidFormat.Load()
	export["requests"] = j.requests.Load()
	export["responses"] = j.responses.Load()
	export["listeners"] = j.listeners.Load()
	export["workers"] = j.workers.Load()
	export["readError"] = j.readError.Load()
	export["announce"] = j.announce.Load()
	export["nts_auth_ok"] = j.ntsAuthOK.Load()
	export["nts_auth_failed"] = j.ntsAuthFailed.Load()
	export["nts_cookie_open_failed"] = j.ntsCookieOpenFailed.Load()
	export["nts_cookie_expired"] = j.ntsCookieExpired.Load()
	export["nts_cookie_future"] = j.ntsCookieFuture.Load()

	return export
}

// handleRequest is a handler used for all http monitoring requests
func (j *JSONStats) handleRequest(w http.ResponseWriter, _ *http.Request) {
	js, err := json.Marshal(j.ToMap())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(js); err != nil {
		log.Errorf("Failed to reply: %v", err)
	}
}

// Start serves the counters over http on the monitoring port. The mux must not
// be http.DefaultServeMux: the responder binary links net/http/pprof, whose
// init registers the profiling endpoints there.
func (j *JSONStats) Start(port int) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", j.handleRequest)
	addr := fmt.Sprintf(":%d", port)
	log.Debugf("Starting http json server on %s", addr)
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Errorf("Failed to start listener: %v", err)
	}
}

// IncInvalidFormat atomically add 1 to the counter
func (j *JSONStats) IncInvalidFormat() {
	j.invalidFormat.Add(1)
}

// IncRequests atomically add 1 to the counter
func (j *JSONStats) IncRequests() {
	j.requests.Add(1)
}

// IncResponses atomically add 1 to the counter
func (j *JSONStats) IncResponses() {
	j.responses.Add(1)
}

// IncListeners atomically add 1 to the counter
func (j *JSONStats) IncListeners() {
	j.listeners.Add(1)
}

// IncWorkers atomically add 1 to the counter
func (j *JSONStats) IncWorkers() {
	j.workers.Add(1)
}

// IncReadError atomically add 1 to the counter
func (j *JSONStats) IncReadError() {
	j.readError.Add(1)
}

// IncNTSAuthOK atomically add 1 to the counter
func (j *JSONStats) IncNTSAuthOK() {
	j.ntsAuthOK.Add(1)
}

// IncNTSAuthFailed atomically add 1 to the counter
func (j *JSONStats) IncNTSAuthFailed() {
	j.ntsAuthFailed.Add(1)
}

// IncNTSCookieOpenFailed atomically add 1 to the counter
func (j *JSONStats) IncNTSCookieOpenFailed() {
	j.ntsCookieOpenFailed.Add(1)
}

// IncNTSCookieExpired atomically add 1 to the counter
func (j *JSONStats) IncNTSCookieExpired() {
	j.ntsCookieExpired.Add(1)
}

// IncNTSCookieFuture atomically add 1 to the counter
func (j *JSONStats) IncNTSCookieFuture() {
	j.ntsCookieFuture.Add(1)
}

// DecListeners atomically removes 1 from the counter
func (j *JSONStats) DecListeners() {
	j.listeners.Add(-1)
}

// DecWorkers atomically removes 1 from the counter
func (j *JSONStats) DecWorkers() {
	j.workers.Add(-1)
}

// SetAnnounce atomically sets counter to 1
func (j *JSONStats) SetAnnounce() {
	j.announce.Store(1)
}

// ResetAnnounce atomically sets counter to 0
func (j *JSONStats) ResetAnnounce() {
	j.announce.Store(0)
}
