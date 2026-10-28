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
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	log "github.com/sirupsen/logrus"
)

// an outgoing connection can hold the port as its source port until it closes
const listenRetryInterval = time.Second

// JSONStats is what we want to report as stats via http
type JSONStats struct {
	Stats
}

// NewJSONStats returns a new JSONStats
func NewJSONStats() *JSONStats {
	return &JSONStats{Stats: *NewStats()}
}

// Start runs http server, retrying until the port is free
func (s *JSONStats) Start(monitoringport int) {
	s.serve(fmt.Sprintf(":%d", monitoringport), listenRetryInterval)
}

func (s *JSONStats) serve(addr string, retryInterval time.Duration) {
	server := &http.Server{
		Addr:         addr,
		Handler:      s.handler(),
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	}
	log.Infof("Starting http json server on %s", addr)
	for {
		err := server.ListenAndServe()
		log.Errorf("Failed to start listener, retrying in %s: %v", retryInterval, err)
		time.Sleep(retryInterval)
	}
}

func (s *JSONStats) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/stats", s.handleStatsRequest)
	mux.HandleFunc("/counters", s.handleCountersRequest)
	return mux
}

// GetCounters returns all counters with process stats, named as fb303 exports them
func (s *JSONStats) GetCounters() map[string]int64 {
	res := map[string]int64{
		"go.runtime.num_goroutines": int64(runtime.NumGoroutine()),
	}
	if proc, err := process.NewProcess(int32(os.Getpid())); err == nil { //nolint:gosec
		if mem, err := proc.MemoryInfo(); err == nil {
			res["go.process.rss"] = int64(mem.RSS) //nolint:gosec
		}
	}
	for k, v := range s.Get() {
		res[fmt.Sprintf("clock.%s", k)] = v
	}
	return res
}

// handleStatsRequest serves every counter under its bare name
func (s *JSONStats) handleStatsRequest(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.Get())
}

// handleCountersRequest serves the counters to collect
func (s *JSONStats) handleCountersRequest(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.GetCounters())
}

func writeJSON(w http.ResponseWriter, counters map[string]int64) {
	js, err := json.Marshal(counters)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(js); err != nil {
		log.Errorf("Failed to reply: %v", err)
	}
}
