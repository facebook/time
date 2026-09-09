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
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/facebook/time/ptp/pdelay"
	ptp "github.com/facebook/time/ptp/protocol"
	log "github.com/sirupsen/logrus"
)

const (
	contentType     = "Content-Type"
	applicationJSON = "application/json"
)

// JSONStats is what we want to report as stats via http
type JSONStats struct {
	*Stats
}

// NewJSONStats returns a new JSONStats
func NewJSONStats() (*JSONStats, error) {
	stats, err := NewStats()
	return &JSONStats{Stats: stats}, err
}

// Start runs http server and initializes maps
func (s JSONStats) Start(monitoringhost string, monitoringport int, interval time.Duration, pinger Pinger) {
	// collect stats forever
	go func() {
		for range time.Tick(interval) {
			// update stats on every tick
			s.CollectSysStats()
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRootRequest)
	mux.HandleFunc("/counters", s.handleCountersRequest)
	mux.HandleFunc("/ping", s.pingHandler(pinger))
	addr := net.JoinHostPort(cmp.Or(monitoringhost, DefaultConfig().MonitoringHost), strconv.Itoa(monitoringport))
	log.Infof("Starting http json server on %s", addr)
	server := &http.Server{
		Addr:         addr,
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
		Handler:      mux,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Failed to start listener: %v", err)
	}
}

// checkPingTarget allows the peer delay groups and ordinary unicast hosts; an
// arbitrary group would fan out PTP traffic from the event port
func checkPingTarget(addr netip.Addr) error {
	switch {
	case isPDelayGroup(addr):
		return nil
	case addr.IsMulticast():
		return fmt.Errorf("multicast target %s must be a PTP peer delay group", addr)
	case addr.IsUnspecified():
		return fmt.Errorf("target %s is unspecified", addr)
	case addr.Is4() && addr.As4() == [4]byte{255, 255, 255, 255}:
		return fmt.Errorf("target %s is a broadcast address", addr)
	default:
		return nil
	}
}

// isPDelayGroup ignores the zone, which selects an interface rather than a group
func isPDelayGroup(addr netip.Addr) bool {
	group := addr.WithZone("")
	return group == netip.MustParseAddr(ptp.PDelayMulticastIPv4) ||
		group == netip.MustParseAddr(ptp.PDelayMulticastIPv6)
}

// pingHandler measures network latency to the requested target via the PTP event port
func (s JSONStats) pingHandler(pinger Pinger) http.HandlerFunc {
	// every path that refuses before a packet leaves counts and logs the same way
	reject := func(w http.ResponseWriter, code int, msg string, args ...any) {
		s.IncPingRejected()
		text := fmt.Sprintf(msg, args...)
		log.Warningf("[ping] rejected: %s", text)
		http.Error(w, text, code)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// a GET pattern also matches HEAD, which must not send PTP traffic
		if r.Method != http.MethodGet {
			reject(w, http.StatusMethodNotAllowed, "method %s is not supported, use GET", r.Method)
			return
		}
		if pinger == nil {
			reject(w, http.StatusServiceUnavailable, "ping is not available")
			return
		}
		rawTarget := r.URL.Query().Get("target")
		target, err := LookupNetIP(rawTarget)
		if err != nil {
			reject(w, http.StatusBadRequest, "bad target %q: %v", rawTarget, err)
			return
		}
		// unmap so the address validated is the address sent; the zone stays because
		// it selects the egress interface
		target = target.Unmap()
		if err := checkPingTarget(target); err != nil {
			reject(w, http.StatusBadRequest, "%v", err)
			return
		}
		// the handler outlives the server's write timeout by design, it waits for responses
		if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(PingTimeout + time.Second)); err != nil {
			log.Warningf("Failed to extend write deadline: %v", err)
		}

		res, err := pinger.Ping(r.Context(), target)
		switch {
		case errors.Is(err, ErrPingInFlight):
			// contention refused the request, no packet was sent
			reject(w, http.StatusConflict, "%v", err)
			return
		case err != nil:
			s.IncPingErrors()
			log.Warningf("[ping] probe to %s failed: %v", target, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if res == nil {
			// no responders is a successful empty array, and nil marshals to null
			res = pdelay.Results{}
		}
		js, err := json.Marshal(res)
		if err != nil {
			// counted as an error, not a completed probe, so the three counters
			// stay a partition of every request
			s.IncPingErrors()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.IncPingRequests()
		w.Header().Set(contentType, applicationJSON)
		if _, err = w.Write(js); err != nil {
			log.Errorf("Failed to reply: %v", err)
		}
	}
}

// handleRootRequest is a handler used for all http monitoring requests
func (s JSONStats) handleRootRequest(w http.ResponseWriter, _ *http.Request) {
	js, err := json.Marshal(s.GetGMStats())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(contentType, applicationJSON)
	if _, err = w.Write(js); err != nil {
		log.Errorf("Failed to reply: %v", err)
	}
}

// handleCountersRequest is a handler used for all http monitoring requests
func (s JSONStats) handleCountersRequest(w http.ResponseWriter, _ *http.Request) {
	js, err := json.Marshal(s.GetCounters())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set(contentType, applicationJSON)
	if _, err = w.Write(js); err != nil {
		log.Errorf("Failed to reply: %v", err)
	}
}
