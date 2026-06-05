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
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"
)

// JSONStats serves ntripper metrics as JSON over HTTP.
type JSONStats struct {
	counters

	// framesPerSecond is computed by a background ticker.
	framesPerSecond atomic.Int64
	lastFrames      atomic.Int64
}

// NewJSONStats creates a new JSONStats instance.
func NewJSONStats() *JSONStats {
	return &JSONStats{}
}

// Start launches the HTTP server and a background goroutine that computes
// per-second rates.
func (s *JSONStats) Start(port int) {
	go s.computeRates()

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)
	addr := fmt.Sprintf(":%d", port)
	slog.Info("starting JSON monitoring server", "addr", addr)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("monitoring server failed", "error", err)
	}
}

func (s *JSONStats) computeRates() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		cur := s.framesReceived.Load()
		prev := s.lastFrames.Swap(cur)
		s.framesPerSecond.Store(cur - prev)
	}
}

func (s *JSONStats) handleRequest(w http.ResponseWriter, _ *http.Request) {
	snap := snapshot{
		FramesPerSecond: s.framesPerSecond.Load(),
		Connected:       s.connected.Load(),
		Reconnects:      s.reconnects.Load(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(snap); err != nil {
		slog.Error("failed to write monitoring response", "error", err)
	}
}

// IncFramesReceived increments the received frame counter.
func (s *JSONStats) IncFramesReceived() { s.framesReceived.Add(1) }

// SetConnected sets connection status (1=connected, 0=disconnected).
func (s *JSONStats) SetConnected(v int64) { s.connected.Store(v) }

// IncReconnects increments the reconnect counter.
func (s *JSONStats) IncReconnects() { s.reconnects.Add(1) }
