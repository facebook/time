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

	ptp "github.com/facebook/time/ptp/protocol"
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
	s.rxSignalingGrant.copy(&s.report.rxSignalingGrant)
	s.rxSignalingCancel.copy(&s.report.rxSignalingCancel)
	s.txSignalingGrant.copy(&s.report.txSignalingGrant)
	s.txSignalingCancel.copy(&s.report.txSignalingCancel)
	s.workerQueue.copy(&s.report.workerQueue)
	s.workerSubs.copy(&s.report.workerSubs)
	s.txtsattempts.copy(&s.report.txtsattempts)
	s.report.utcoffsetSec = s.utcoffsetSec
	s.report.clockaccuracy = s.clockaccuracy
	s.report.clockclass = s.clockclass
	s.report.drain = s.drain
	s.report.reload = s.reload
	s.report.txtsMissing = s.txtsMissing
	s.report.minMaxCF = s.minMaxCF
}

// handleRequest is a handler used for all http monitoring requests
func (s *JSONStats) handleRequest(w http.ResponseWriter, _ *http.Request) {
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

// IncRXSignalingGrant atomically add 1 to the counter
func (s *JSONStats) IncRXSignalingGrant(t ptp.MessageType) {
	s.rxSignalingGrant.inc(int(t))
}

// IncRXSignalingCancel atomically add 1 to the counter
func (s *JSONStats) IncRXSignalingCancel(t ptp.MessageType) {
	s.rxSignalingCancel.inc(int(t))
}

// IncTXSignalingGrant atomically add 1 to the counter
func (s *JSONStats) IncTXSignalingGrant(t ptp.MessageType) {
	s.txSignalingGrant.inc(int(t))
}

// IncTXSignalingCancel atomically add 1 to the counter
func (s *JSONStats) IncTXSignalingCancel(t ptp.MessageType) {
	s.txSignalingCancel.inc(int(t))
}

// IncWorkerSubs atomically add 1 to the counter
func (s *JSONStats) IncWorkerSubs(workerid int) {
	s.workerSubs.inc(workerid)
}

// IncReload atomically add 1 to the counter
func (s *JSONStats) IncReload() {
	atomic.AddInt64(&s.reload, 1)
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

// DecRXSignalingGrant atomically removes 1 from the counter
func (s *JSONStats) DecRXSignalingGrant(t ptp.MessageType) {
	s.rxSignalingGrant.dec(int(t))
}

// DecRXSignalingCancel atomically removes 1 from the counter
func (s *JSONStats) DecRXSignalingCancel(t ptp.MessageType) {
	s.rxSignalingCancel.dec(int(t))
}

// DecTXSignalingGrant atomically removes 1 from the counter
func (s *JSONStats) DecTXSignalingGrant(t ptp.MessageType) {
	s.txSignalingGrant.dec(int(t))
}

// DecTXSignalingCancel atomically removes 1 from the counter
func (s *JSONStats) DecTXSignalingCancel(t ptp.MessageType) {
	s.txSignalingCancel.dec(int(t))
}

// DecWorkerSubs atomically removes 1 from the counter
func (s *JSONStats) DecWorkerSubs(workerid int) {
	s.workerSubs.dec(workerid)
}

// SetMaxWorkerQueue atomically sets worker queue len
func (s *JSONStats) SetMaxWorkerQueue(workerid int, queue int64) {
	if queue > s.workerQueue.load(workerid) {
		s.workerQueue.store(workerid, queue)
	}
}

// SetMaxTXTSAttempts atomically sets number of retries for get latest TX timestamp
func (s *JSONStats) SetMaxTXTSAttempts(workerid int, attempts int64) {
	if attempts > s.txtsattempts.load(workerid) {
		s.txtsattempts.store(workerid, attempts)
	}
}

// IncTXTSMissing atomically increments the counter when all retries to get latest TX timestamp exceeded
func (s *JSONStats) IncTXTSMissing() {
	atomic.AddInt64(&s.txtsMissing, 1)
}

// SetMinMaxCF atomically sets max CF value observed (assuming all CF values are positive)
// or min CF (if any CF values are negative)
// CF values may be negative if PTP TCs are malfunctioning
func (s *JSONStats) SetMinMaxCF(cf int64) {
	for {
		mmCF := atomic.LoadInt64(&s.minMaxCF)
		var shouldUpdate bool

		if cf > 0 && mmCF >= 0 && cf > mmCF {
			shouldUpdate = true
		} else if cf <= 0 && cf < mmCF {
			shouldUpdate = true
		}

		if !shouldUpdate {
			return
		}

		// Atomically compare and swap - retry if another goroutine modified the value
		if atomic.CompareAndSwapInt64(&s.minMaxCF, mmCF, cf) {
			return
		}
		// If CompareAndSwap failed, another goroutine modified the value
		// Continue the loop to retry with the new value
	}
}

// SetUTCOffsetSec atomically sets the utcoffset
func (s *JSONStats) SetUTCOffsetSec(utcoffsetSec int64) {
	atomic.StoreInt64(&s.utcoffsetSec, utcoffsetSec)
}

// SetClockAccuracy atomically sets the clock accuracy
func (s *JSONStats) SetClockAccuracy(clockaccuracy int64) {
	atomic.StoreInt64(&s.clockaccuracy, clockaccuracy)
}

// SetClockClass atomically sets the clock class
func (s *JSONStats) SetClockClass(clockclass int64) {
	atomic.StoreInt64(&s.clockclass, clockclass)
}

// SetDrain atomically sets the drain status
func (s *JSONStats) SetDrain(drain int64) {
	atomic.StoreInt64(&s.drain, drain)
}
