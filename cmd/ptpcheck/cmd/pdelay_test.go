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

package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/facebook/time/ptp/pdelay"
	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/sptp/client"
	"github.com/stretchr/testify/require"
)

func TestCalculateJitter(t *testing.T) {
	tests := []struct {
		name      string
		maxJitter time.Duration
	}{
		{
			name:      "zero jitter",
			maxJitter: 0,
		},
		{
			name:      "negative jitter",
			maxJitter: -time.Second,
		},
		{
			name:      "positive jitter",
			maxJitter: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jitter := CalculateJitter(tt.maxJitter)

			if tt.maxJitter <= 0 {
				require.Zero(t, jitter)
			} else {
				require.GreaterOrEqual(t, jitter, time.Duration(0))
				require.Less(t, jitter, tt.maxJitter)
			}
		})
	}
}

func TestCalculateJitterDistribution(t *testing.T) {
	maxJitter := 30 * time.Second
	seen := make(map[time.Duration]bool)

	// Run multiple times to verify randomness produces different values
	for range 100 {
		jitter := CalculateJitter(maxJitter)
		require.GreaterOrEqual(t, jitter, time.Duration(0))
		require.Less(t, jitter, maxJitter)
		seen[jitter] = true
	}

	// Verify that different values are produced (randomness check)
	require.Greater(t, len(seen), 1, "CalculateJitter should produce different values across multiple calls")
}

func TestRunMulticastProbe(t *testing.T) {
	var gotPath, gotTarget string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotTarget = r.URL.Path, r.URL.Query().Get("target")
		fmt.Fprint(w, `[{"responder":"2401:db00::1","t1":"2023-11-14T22:13:20Z","t2":"2023-11-14T22:13:20.0001Z","t3":"2023-11-14T22:13:20.0002Z","t4":"2023-11-14T22:13:20.00031Z"},
		                {"responder":"2401:db00::2","error":"incomplete response"}]`)
	}))
	defer srv.Close()

	res, err := runMulticastProbe(t.Context(), ProbeConfig{Server: srv.URL, Timeout: DefaultPingTimeout})
	require.NoError(t, err)
	require.Equal(t, "/ping", gotPath)
	require.Equal(t, ptp.PDelayMulticastIPv6, gotTarget)
	require.Len(t, res, 2)
	require.True(t, res[0].Valid())
	require.False(t, res[1].Valid())
	require.ErrorContains(t, res[1].Error, "incomplete response")
}

func TestRunMulticastProbeSelectsIPv4Group(t *testing.T) {
	var gotTarget string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.URL.Query().Get("target")
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	res, err := runMulticastProbe(t.Context(), ProbeConfig{Server: srv.URL, Timeout: DefaultPingTimeout, IPv4: true})
	require.NoError(t, err)
	require.Equal(t, ptp.PDelayMulticastIPv4, gotTarget)
	require.Empty(t, res)
}

func TestRunMulticastProbeSurfacesTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "ping already in flight", http.StatusConflict)
	}))
	defer srv.Close()

	_, err := runMulticastProbe(t.Context(), ProbeConfig{Server: srv.URL, Timeout: DefaultPingTimeout})
	require.ErrorContains(t, err, "ping already in flight")
}

// an errored result with complete timestamps must not reach scuba as a real zero
func TestRunMulticastProbeErroredResultKeepsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[{"responder":"2401:db00::1","t1":"2023-11-14T22:13:20Z","t2":"2023-11-14T22:13:20.0001Z","t3":"2023-11-14T22:13:20.0002Z","t4":"2023-11-14T22:13:20.00031Z","error":"bad tc"}]`)
	}))
	defer srv.Close()

	res, err := runMulticastProbe(t.Context(), ProbeConfig{Server: srv.URL, Timeout: DefaultPingTimeout})
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.ErrorContains(t, res[0].Error, "bad tc")
}

// an incomplete result with no wire error must still be flagged locally
func TestRunMulticastProbeIncompleteGetsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[{"responder":"2401:db00::1","t1":"2023-11-14T22:13:20Z","t2":"2023-11-14T22:13:20.0001Z"}]`)
	}))
	defer srv.Close()

	res, err := runMulticastProbe(t.Context(), ProbeConfig{Server: srv.URL, Timeout: DefaultPingTimeout})
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.ErrorContains(t, res[0].Error, "incomplete response")
	require.False(t, res[0].Valid())
}

// periodic mode would otherwise loop forever on an unusable timeout
func TestRunPeriodicProbeRejectsShortTimeout(t *testing.T) {
	err := RunPeriodicProbe(t.Context(), ProbeConfig{Server: "http://127.0.0.1:1", Timeout: time.Second})
	require.ErrorContains(t, err, "must exceed")
}

// RunPeriodicProbe is what ptp_pdelay_cron.sh drives, and OnProbeResult is how
// results reach Scuba, so exercise the entrypoint rather than only the helper
func TestRunPeriodicProbeCountOne(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[{"responder":"2401:db00::1","t1":"2023-11-14T22:13:20Z","t2":"2023-11-14T22:13:20.0001Z",
		                 "t3":"2023-11-14T22:13:20.0002Z","t4":"2023-11-14T22:13:20.00031Z"},
		                {"responder":"2401:db00::2","t1":"2023-11-14T22:13:20Z","t2":"2023-11-14T22:13:20.0001Z"}]`)
	}))
	defer srv.Close()

	var got []*pdelay.Result
	var gotServer string
	OnProbeResult = func(results []*pdelay.Result, server string) {
		got, gotServer = results, server
	}
	t.Cleanup(func() { OnProbeResult = nil })

	err := RunPeriodicProbe(t.Context(), ProbeConfig{Server: srv.URL, Count: 1, Timeout: DefaultPingTimeout})
	require.NoError(t, err)
	require.Len(t, got, 2, "the callback must receive every responder")
	// anything the callback reads back must describe the sptp that was probed,
	// not whichever one a fresh lookup happens to resolve
	require.Equal(t, srv.URL, gotServer)

	require.True(t, got[0].Valid())
	require.NoError(t, got[0].Error)
	// an incomplete responder must carry an error so Scuba does not log a real zero
	require.False(t, got[1].Valid())
	require.ErrorContains(t, got[1].Error, "incomplete response")
}

func TestRunPeriodicProbeCountOneTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	called := false
	OnProbeResult = func([]*pdelay.Result, string) { called = true }
	t.Cleanup(func() { OnProbeResult = nil })

	// cron reads the exit code, so a probe that never landed must not look healthy
	err := RunPeriodicProbe(t.Context(), ProbeConfig{Server: srv.URL, Count: 1, Timeout: DefaultPingTimeout})
	require.ErrorContains(t, err, "probes failed")
	require.False(t, called, "a failed probe must not invoke the scuba callback")
}

func TestRunPeriodicProbeCountedExitsOnTransportError(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		http.Error(w, "sptp is down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	done := make(chan error, 1)
	go func() {
		done <- RunPeriodicProbe(t.Context(), ProbeConfig{Server: srv.URL, Count: 3, Timeout: DefaultPingTimeout})
	}()

	select {
	case err := <-done:
		require.ErrorContains(t, err, "probes failed")
		require.Equal(t, 3, hits, "each failure must consume one counted attempt")
	case <-time.After(30 * time.Second):
		t.Fatal("counted mode looped instead of finishing")
	}
}

// the default has to survive a change to sptp's collection window, which a
// hardcoded value would not
func TestDefaultPingTimeoutSatisfiesValidator(t *testing.T) {
	require.Greater(t, DefaultPingTimeout, client.PingTimeout)
	require.NoError(t, checkPingTimeout(DefaultPingTimeout))
}

// ptp_pdelay_cron.sh wraps this in `timeout 5`, so counted mode must not spend a
// second sleeping before it has probed anything
func TestRunPeriodicProbeCountedProbesImmediately(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	start := time.Now()
	require.NoError(t, RunPeriodicProbe(t.Context(), ProbeConfig{Server: srv.URL, Count: 1, Timeout: DefaultPingTimeout}))
	require.Less(t, time.Since(start), 500*time.Millisecond, "counted mode must not pre-sleep")
}

// one success in a run means the dataset got a row, so the run is not a failure
func TestRunPeriodicProbeCountedMixedSuccess(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		if hits == 1 {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[]`)
			return
		}
		http.Error(w, "sptp is down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	require.NoError(t, RunPeriodicProbe(t.Context(),
		ProbeConfig{Server: srv.URL, Count: 3, Timeout: DefaultPingTimeout}))
	require.Equal(t, 3, hits)
}

// counted mode has no wait, so cancellation must be checked rather than raced
// against time.After(0)
func TestRunPeriodicProbeCountedHonoursCancelledContext(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := RunPeriodicProbe(ctx, ProbeConfig{Server: srv.URL, Count: 3, Timeout: DefaultPingTimeout})
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, hits, "no probe may be sent after cancellation")
}
