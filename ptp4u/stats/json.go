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

	ptp "github.com/facebookincubator/ptp/protocol"
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

	s.init()
	s.report.init()

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
	s.subscriptions.copy(&s.report.subscriptions)
	s.rx.copy(&s.report.rx)
	s.tx.copy(&s.report.tx)
	s.rxSignaling.copy(&s.report.rxSignaling)
	s.txSignaling.copy(&s.report.txSignaling)
	s.txtsattempts.copy(&s.report.txtsattempts)
	s.report.utcoffset = s.utcoffset
	s.report.workerQueue = s.workerQueue
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

// Reset atomically sets all the counters to 0
func (s *JSONStats) Reset() {
	s.reset()
}

// IncSubscription atomically add 1 to the counter
func (s *JSONStats) IncSubscription(t ptp.MessageType) {
	s.subscriptions.inc(int(t))
}

// IncRX atomically add 1 to the counter
func (s *JSONStats) IncRX(t ptp.MessageType) {
	s.rx.inc(int(t))
}

// IncTX atomically add 1 to the counter
func (s *JSONStats) IncTX(t ptp.MessageType) {
	s.tx.inc(int(t))
}

// IncRXSignaling atomically add 1 to the counter
func (s *JSONStats) IncRXSignaling(t ptp.MessageType) {
	s.rxSignaling.inc(int(t))
}

// IncTXSignaling atomically add 1 to the counter
func (s *JSONStats) IncTXSignaling(t ptp.MessageType) {
	s.txSignaling.inc(int(t))
}

// DecSubscription atomically removes 1 from the counter
func (s *JSONStats) DecSubscription(t ptp.MessageType) {
	s.subscriptions.dec(int(t))
}

// DecRX atomically removes 1 from the counter
func (s *JSONStats) DecRX(t ptp.MessageType) {
	s.rx.dec(int(t))
}

// DecTX atomically removes 1 from the counter
func (s *JSONStats) DecTX(t ptp.MessageType) {
	s.tx.dec(int(t))
}

// DecRXSignaling atomically removes 1 from the counter
func (s *JSONStats) DecRXSignaling(t ptp.MessageType) {
	s.rxSignaling.dec(int(t))
}

// DecTXSignaling atomically removes 1 from the counter
func (s *JSONStats) DecTXSignaling(t ptp.MessageType) {
	s.txSignaling.dec(int(t))
}

// SetWorkerQueue atomically sets worker queue len
func (s *JSONStats) SetWorkerQueue(queue int64) {
	atomic.StoreInt64(&s.workerQueue, queue)
}

// SetMaxTXTSAttempts atomically sets number of retries for get latest TX timestamp
func (s *JSONStats) SetMaxTXTSAttempts(workerid int, attempts int64) {
	if attempts > s.txtsattempts.load(workerid) {
		s.txtsattempts.store(workerid, attempts)
	}
}

// SetUTCOffset atomically sets the utcoffset
func (s *JSONStats) SetUTCOffset(utcoffset int64) {
	atomic.StoreInt64(&s.utcoffset, utcoffset)
}
