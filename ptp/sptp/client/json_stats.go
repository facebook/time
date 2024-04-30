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
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

// JSONStats is what we want to report as stats via http
type JSONStats struct {
	Stats
}

// NewJSONStats returns a new JSONStats
func NewJSONStats() *JSONStats {
	return &JSONStats{Stats: *NewStats()}
}

// Start runs http server and initializes maps
func (s *JSONStats) Start(monitoringport int, interval time.Duration) {
	// collect stats forever
	go func() {
		for range time.Tick(interval) {
			// update stats on every tick
			if err := s.CollectSysStats(); err != nil {
				log.Warningf("failed to get system metrics %s", err)
			}
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRootRequest)
	mux.HandleFunc("/counters", s.handleCountersRequest)
	addr := fmt.Sprintf(":%d", monitoringport)
	log.Infof("Starting http json server on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Failed to start listener: %v", err)
	}
}

// handleRootRequest is a handler used for all http monitoring requests
func (s *JSONStats) handleRootRequest(w http.ResponseWriter, _ *http.Request) {
	js, err := json.Marshal(s.GetGMStats())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(js); err != nil {
		log.Errorf("Failed to reply: %v", err)
	}
}

// handleCountersRequest is a handler used for all http monitoring requests
func (s *JSONStats) handleCountersRequest(w http.ResponseWriter, _ *http.Request) {
	js, err := json.Marshal(s.GetCounters())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(js); err != nil {
		log.Errorf("Failed to reply: %v", err)
	}
}
