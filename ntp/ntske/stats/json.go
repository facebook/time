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
Package stats implements statistics collection and reporting for the NTS-KE
server. It reports the per-connection counters emitted by ntske.Server
(completed handshakes and errors) as JSON over an HTTP interface.
*/
package stats

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
)

// JSONStats reports NTS-KE server metrics as JSON over an HTTP interface and
// satisfies the ntske.Stats interface. It is a passive implementation: only
// Start needs to be called to expose the counters.
type JSONStats struct {
	// keep these aligned to 64-bit for sync/atomic
	handshakes atomic.Int64
	errors     atomic.Int64
}

// toMap converts the counters to a map for JSON export.
func (j *JSONStats) toMap() map[string]int64 {
	return map[string]int64{
		"nts.ke.handshakes": j.handshakes.Load(),
		"nts.ke.errors":     j.errors.Load(),
	}
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
		slog.Error("ntske stats: failed to reply", "err", err)
	}
}

// Start launches the HTTP JSON metrics server on the given port. It blocks and
// is intended to be run in its own goroutine.
func (j *JSONStats) Start(port int) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", j.handleRequest)
	addr := fmt.Sprintf(":%d", port)
	slog.Debug("starting ntske stats http server", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil { //nolint:gosec // local interop testing server, no timeouts needed
		slog.Error("ntske stats: failed to start listener", "err", err)
	}
}

// IncHandshakes atomically adds 1 to the NTS-KE handshake counter.
func (j *JSONStats) IncHandshakes() {
	j.handshakes.Add(1)
}

// IncErrors atomically adds 1 to the NTS-KE error counter.
func (j *JSONStats) IncErrors() {
	j.errors.Add(1)
}
