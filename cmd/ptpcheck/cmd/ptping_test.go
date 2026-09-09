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
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/facebook/time/ptp/pdelay"
	"github.com/stretchr/testify/require"
)

func TestResultToTimestamps(t *testing.T) {
	base := time.Unix(1700000000, 0)
	r := &pdelay.Result{
		T1: base,
		T2: base.Add(100 * time.Microsecond),
		T3: base.Add(200 * time.Microsecond),
		T4: base.Add(310 * time.Microsecond),
	}

	ts := resultToTimestamps(r)
	require.Equal(t, r.T1, ts.t3)
	require.Equal(t, r.T2, ts.t4)
	require.Equal(t, r.T3, ts.t1)
	require.Equal(t, r.T4, ts.t2)
}

func TestPtpingRunFailsWhenEveryProbeFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := ptpingRun(t.Context(), srv.URL, "2401:db00::1", 2, DefaultPingTimeout)
	require.ErrorContains(t, err, "no successful probes")
}

func TestPtpingRunFailsOnIncompleteResult(t *testing.T) {
	// only t1/t2 set, so Valid() is false and fw/bk would render as garbage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[{"responder":"2401:db00::1","t1":"2023-11-14T22:13:20Z","t2":"2023-11-14T22:13:20.0001Z"}]`)
	}))
	defer srv.Close()

	err := ptpingRun(t.Context(), srv.URL, "2401:db00::1", 1, DefaultPingTimeout)
	require.ErrorContains(t, err, "no successful probes")
}

func TestPtpingRunSucceedsOnValidResult(t *testing.T) {
	var gotPath, gotTarget string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotTarget = r.URL.Path, r.URL.Query().Get("target")
		// t1..t4 100us apart, sw_rtt 500us
		fmt.Fprint(w, `[{"responder":"2401:db00::1","t1":"2023-11-14T22:13:20Z","t2":"2023-11-14T22:13:20.0001Z",
		                 "t3":"2023-11-14T22:13:20.0002Z","t4":"2023-11-14T22:13:20.00031Z","sw_rtt":500000}]`)
	}))
	defer srv.Close()

	readStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := ptpingRun(t.Context(), srv.URL, "2401:db00::1", 1, DefaultPingTimeout)
	w.Close()
	out, _ := io.ReadAll(r)
	os.Stdout = readStdout
	require.NoError(t, err)
	require.Equal(t, "/ping", gotPath)
	require.Equal(t, "2401:db00::1", gotTarget)

	// a nanos/micros scale bug in the wire path would change this line
	require.Equal(t, "2401:db00::1: seq=1 net=210\u00b5s (->100\u00b5s + <-110\u00b5s)\trtt=500\u00b5s\n", string(out))
}

func TestPtpingRunNoOpOnZeroCount(t *testing.T) {
	require.NoError(t, ptpingRun(t.Context(), "http://127.0.0.1:1", "2401:db00::1", 0, DefaultPingTimeout))
}

func TestPtpingRunFailsOnEmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	require.ErrorContains(t, ptpingRun(t.Context(), srv.URL, "2401:db00::1", 1, DefaultPingTimeout), "no successful probes")
}

func TestPtpingRunFailsOnWireError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[{"responder":"2401:db00::1","t1":"2023-11-14T22:13:20Z","t2":"2023-11-14T22:13:20.0001Z",
		                 "t3":"2023-11-14T22:13:20.0002Z","t4":"2023-11-14T22:13:20.00031Z","error":"bad tc"}]`)
	}))
	defer srv.Close()

	require.ErrorContains(t, ptpingRun(t.Context(), srv.URL, "2401:db00::1", 1, DefaultPingTimeout), "no successful probes")
}

// more than one result for a unicast target is unexpected; the first is used
func TestPtpingRunSelectsMatchingResponder(t *testing.T) {
	// the target is second, so a positional pick would take the wrong one
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[{"responder":"2401:db00::2","t1":"2023-11-14T22:13:20Z","t2":"2023-11-14T22:13:20.0009Z",
		                 "t3":"2023-11-14T22:13:20.0002Z","t4":"2023-11-14T22:13:20.00031Z","sw_rtt":900000},
		                {"responder":"2401:db00::1","t1":"2023-11-14T22:13:20Z","t2":"2023-11-14T22:13:20.0001Z",
		                 "t3":"2023-11-14T22:13:20.0002Z","t4":"2023-11-14T22:13:20.00031Z","sw_rtt":500000}]`)
	}))
	defer srv.Close()

	readStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := ptpingRun(t.Context(), srv.URL, "2401:db00::1", 1, DefaultPingTimeout)
	w.Close()
	out, _ := io.ReadAll(r)
	os.Stdout = readStdout

	require.NoError(t, err)
	require.Contains(t, string(out), "rtt=500\u00b5s", "the result matching the target is the one rendered")
}

func TestPtpingRunFailsOnMissingRTT(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// complete T1..T4 but no sw_rtt: rtt would print as 0s
		fmt.Fprint(w, `[{"responder":"2401:db00::1","t1":"2023-11-14T22:13:20Z","t2":"2023-11-14T22:13:20.0001Z",
		                 "t3":"2023-11-14T22:13:20.0002Z","t4":"2023-11-14T22:13:20.00031Z"}]`)
	}))
	defer srv.Close()

	err := ptpingRun(t.Context(), srv.URL, "2401:db00::1", 1, DefaultPingTimeout)
	require.ErrorContains(t, err, "no successful probes")
}

func TestPickResponder(t *testing.T) {
	want := netip.MustParseAddr("2401:db00::1")
	other := netip.MustParseAddr("2401:db00::2")

	res := pickResponder(pdelay.Results{{Responder: other}, {Responder: want}}, want)
	require.NotNil(t, res)
	require.Equal(t, want, res.Responder)

	require.Nil(t, pickResponder(pdelay.Results{{Responder: other}}, want),
		"a result from another host must not be printed as ours")
	require.Nil(t, pickResponder(pdelay.Results{}, want))

	// an unset responder from a single-result reply is still ours
	res = pickResponder(pdelay.Results{{}}, want)
	require.NotNil(t, res)
}

func TestPtpingRunRejectsShortTimeout(t *testing.T) {
	err := ptpingRun(t.Context(), "http://127.0.0.1:1", "2401:db00::1", 1, time.Second)
	require.ErrorContains(t, err, "must exceed")
}

// LookupNetIP yields ::ffff:a.b.c.d for an A record while sptp Unmap()s the
// responder, so exact equality would match nothing and every probe would fail
func TestPickResponderMatchesAcrossAddressForms(t *testing.T) {
	mapped := netip.MustParseAddr("::ffff:127.0.0.1")
	plain := netip.MustParseAddr("127.0.0.1")

	require.NotEqual(t, mapped, plain, "the two forms are not == equal")
	require.NotNil(t, pickResponder(pdelay.Results{{Responder: plain}}, mapped))
	require.NotNil(t, pickResponder(pdelay.Results{{Responder: mapped}}, plain))

	zoned := netip.MustParseAddr("fe80::1%eth0")
	unzoned := netip.MustParseAddr("fe80::1")
	require.NotNil(t, pickResponder(pdelay.Results{{Responder: unzoned}}, zoned))
}

// cancellation must exit promptly, not log one failure per remaining probe
func TestPtpingRunStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := ptpingRun(ctx, "http://127.0.0.1:1", "2401:db00::1", 5, DefaultPingTimeout)
	require.ErrorIs(t, err, context.Canceled)
}
