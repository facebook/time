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

package metrics

import (
	"container/list"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// maxSamples is the maximum samples considered for calculating min/max offset
const maxSamples = 60

var (
	minOffset, maxOffset = 0.0, 0.0
	offsets              list.List
	offsetsLock          sync.Mutex
)

// RunMetricsServer starts a metrics server on the given port
func RunMetricsServer(monitoringPort uint) error {
	log.Infof("Starting HTTP JSON metrics server - query at localhost:%d/metrics", monitoringPort)
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", monitoringPort),
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	}
	http.Handle("/metrics", &metricsHandler{})
	return server.ListenAndServe()
}

// ObserveOffset sets the value of the ts2phc offset metrics
func ObserveOffset(offset float64) {
	offsetsLock.Lock()
	tmpMinOffset, tmpMaxOffset := math.Inf(1), math.Inf(-1)
	if offsets.Len() >= maxSamples {
		offsets.Remove(offsets.Back())
	}
	offsets.PushFront(offset)
	for elem := offsets.Front(); elem != nil; elem = elem.Next() {
		tmpMinOffset = min(tmpMinOffset, elem.Value.(float64))
		tmpMaxOffset = max(tmpMaxOffset, elem.Value.(float64))
	}
	minOffset, maxOffset = tmpMinOffset, tmpMaxOffset
	offsetsLock.Unlock()
}

type metricsHandler struct{}

func (h *metricsHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	js, err := json.Marshal(getMetrics())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(js); err != nil {
		log.Errorf("Failed to reply to metrics request %v", err)
	}
}

func getMetrics() map[string]float64 {
	return map[string]float64{
		"min_offset": minOffset,
		"max_offset": maxOffset,
	}
}
