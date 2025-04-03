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

// Handler is a handler for a metrics endpoint
type Handler struct {
	minOffset, maxOffset float64
	offsets              *list.List
	offsetsLock          sync.Mutex
}

// maxSamples is the maximum samples considered for calculating min/max offset
const maxSamples = 60

// RunMetricsServer starts a metrics server on the given port
func RunMetricsServer(monitoringPort uint, handler *Handler) error {
	log.Infof("Starting HTTP JSON metrics server - query at localhost:%d/metrics", monitoringPort)
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", monitoringPort),
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	}
	http.Handle("/metrics", handler)
	handler.offsets = list.New()
	return server.ListenAndServe()
}

// ObserveOffset sets the value of the ts2phc offset metrics
func (h *Handler) ObserveOffset(offset float64) {
	h.offsetsLock.Lock()
	tmpMinOffset, tmpMaxOffset := math.Inf(1), math.Inf(-1)
	if h.offsets.Len() >= maxSamples {
		for h.offsets.Len() >= maxSamples {
			h.offsets.Remove(h.offsets.Back())
		}
	}
	h.offsets.PushFront(offset)
	for elem := h.offsets.Front(); elem != nil; elem = elem.Next() {
		//nolint:unconvert
		tmpMinOffset = min(tmpMinOffset, elem.Value.(float64))
		//nolint:unconvert
		tmpMaxOffset = max(tmpMaxOffset, elem.Value.(float64))
	}
	h.minOffset = tmpMinOffset
	h.maxOffset = tmpMaxOffset
	h.offsetsLock.Unlock()
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	js, err := json.Marshal(h.getMetrics())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(js); err != nil {
		log.Errorf("Failed to reply to metrics request %v", err)
	}
}

func (h *Handler) getMetrics() map[string]float64 {
	return map[string]float64{
		"min_offset": h.minOffset,
		"max_offset": h.maxOffset,
	}
}
