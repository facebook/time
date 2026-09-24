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

package pdelay

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func roundTrip(t *testing.T, in *Result) *Result {
	t.Helper()
	b, err := json.Marshal(in)
	require.NoError(t, err)
	out := &Result{}
	require.NoError(t, json.Unmarshal(b, out))
	return out
}

func TestResultRoundTrip(t *testing.T) {
	base := time.Unix(1700000000, 12345)
	in := &Result{
		Responder:           netip.MustParseAddr("2401:db00::1"),
		T1:                  base,
		T2:                  base.Add(100 * time.Microsecond),
		T3:                  base.Add(200 * time.Microsecond),
		T4:                  base.Add(310 * time.Microsecond),
		CorrectionFieldReq:  3 * time.Microsecond,
		CorrectionFieldResp: 4 * time.Microsecond,
		Timestamp:           base.Add(time.Millisecond),
		SWRTT:               500 * time.Microsecond,
	}

	got := roundTrip(t, in)

	require.Equal(t, in.Responder, got.Responder)
	require.True(t, in.T1.Equal(got.T1))
	require.True(t, in.T2.Equal(got.T2))
	require.True(t, in.T3.Equal(got.T3))
	require.True(t, in.T4.Equal(got.T4))
	require.Equal(t, in.CorrectionFieldReq, got.CorrectionFieldReq)
	require.Equal(t, in.CorrectionFieldResp, got.CorrectionFieldResp)
	require.True(t, in.Timestamp.Equal(got.Timestamp))
	require.Equal(t, in.SWRTT, got.SWRTT)
	require.NoError(t, got.Error)
	require.True(t, got.Valid())
	require.Equal(t, in.PathDelay(), got.PathDelay())
	require.Equal(t, in.Offset(), got.Offset())
}

// error is the only field that does not marshal as itself, so the sentinel has
// to be matched by identity on the far side, not just by message
func TestResultRoundTripError(t *testing.T) {
	in := &Result{
		Responder: netip.MustParseAddr("2401:db00::1"),
		Error:     ErrIncompleteResponse,
	}

	got := roundTrip(t, in)
	require.ErrorIs(t, got.Error, ErrIncompleteResponse)
	require.EqualError(t, got.Error, "incomplete response")
	require.False(t, got.Valid())
}

// every other message stays an opaque error, so the sentinel keeps meaning one
// specific failure rather than any failure whose text happens to arrive
func TestResultRoundTripOtherError(t *testing.T) {
	in := &Result{
		Responder: netip.MustParseAddr("2401:db00::1"),
		Error:     errors.New("connection timeout"),
	}

	got := roundTrip(t, in)
	require.EqualError(t, got.Error, "connection timeout")
	require.NotErrorIs(t, got.Error, ErrIncompleteResponse)
}

// a zero Responder and zero timestamps must survive the trip untouched
func TestResultRoundTripZeroValues(t *testing.T) {
	in := &Result{
		T1:    time.Unix(1700000000, 0),
		T4:    time.Unix(1700000000, 500),
		Error: errors.New("incomplete response"),
	}

	got := roundTrip(t, in)
	require.False(t, got.Responder.IsValid())
	require.True(t, got.T2.IsZero())
	require.True(t, got.T3.IsZero())
	require.True(t, got.T1.Equal(in.T1))
	require.True(t, got.T4.Equal(in.T4))
	require.False(t, got.Valid())
	require.EqualError(t, got.Error, "incomplete response")
}

func TestFetchPing(t *testing.T) {
	var gotPath, gotTarget string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotTarget = r.URL.Path, r.URL.Query().Get("target")
		fmt.Fprint(w, `[{"responder":"2401:db00::1","t1":"2023-11-14T22:13:20Z","t2":"2023-11-14T22:13:20.0001Z",
		                 "t3":"2023-11-14T22:13:20.0002Z","t4":"2023-11-14T22:13:20.00031Z","cf_req":3000,"cf_resp":4000}]`)
	}))
	defer srv.Close()

	res, err := FetchPing(t.Context(), srv.URL, "ff02::6b", time.Second)
	require.NoError(t, err)
	require.Equal(t, "/ping", gotPath)
	require.Equal(t, "ff02::6b", gotTarget)
	require.Len(t, res, 1)

	r := res[0]
	require.Equal(t, netip.MustParseAddr("2401:db00::1"), r.Responder)
	require.True(t, r.Valid())
	require.Equal(t, 3*time.Microsecond, r.CorrectionFieldReq)
	require.Equal(t, 4*time.Microsecond, r.CorrectionFieldResp)
}

func TestFetchPingError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "ping already in flight", http.StatusConflict)
	}))
	defer srv.Close()

	_, err := FetchPing(t.Context(), srv.URL, "ff02::6b", time.Second)
	require.ErrorContains(t, err, "ping already in flight")
}

func TestFetchPingRejectsNullEntry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[null]`)
	}))
	defer srv.Close()

	_, err := FetchPing(t.Context(), srv.URL, "ff02::6b", time.Second)
	require.ErrorContains(t, err, "null entry")
}

func TestFetchPingRejectsMalformedResponder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[{"responder":"not-an-address"}]`)
	}))
	defer srv.Close()

	_, err := FetchPing(t.Context(), srv.URL, "ff02::6b", time.Second)
	require.ErrorContains(t, err, "decoding sptp ping response")
}

// decoding into a reused Result must not inherit any field from the previous
// payload, or a partial object reports stale Valid/PathDelay/Offset
func TestResultUnmarshalClearsStaleFields(t *testing.T) {
	r := &Result{}
	full := `{"responder":"2401:db00::1","t1":"2023-11-14T22:13:20Z","t2":"2023-11-14T22:13:20.0001Z",
	          "t3":"2023-11-14T22:13:20.0002Z","t4":"2023-11-14T22:13:20.00031Z",
	          "cf_req":3000,"cf_resp":4000,"sw_rtt":500000,"timestamp":"2023-11-14T22:13:20Z",
	          "error":"incomplete response"}`
	require.NoError(t, json.Unmarshal([]byte(full), r))
	require.True(t, r.Responder.IsValid())
	require.EqualError(t, r.Error, "incomplete response")

	// a payload carrying only a responder must zero everything else
	require.NoError(t, json.Unmarshal([]byte(`{"responder":"2401:db00::2"}`), r))
	require.Equal(t, netip.MustParseAddr("2401:db00::2"), r.Responder)
	require.NoError(t, r.Error)
	require.True(t, r.T1.IsZero(), "stale T1")
	require.True(t, r.T2.IsZero(), "stale T2")
	require.True(t, r.T3.IsZero(), "stale T3")
	require.True(t, r.T4.IsZero(), "stale T4")
	require.Zero(t, r.CorrectionFieldReq, "stale cf_req")
	require.Zero(t, r.CorrectionFieldResp, "stale cf_resp")
	require.Zero(t, r.SWRTT, "stale sw_rtt")
	require.True(t, r.Timestamp.IsZero(), "stale timestamp")
	require.False(t, r.Valid(), "a partial decode must not look like a measurement")
}

func TestFetchPingRejectsTopLevelNull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `null`)
	}))
	defer srv.Close()

	_, err := FetchPing(t.Context(), srv.URL, "ff02::6b", time.Second)
	require.ErrorContains(t, err, "not a JSON array")
}

// an empty array is a successful probe with no responders
func TestFetchPingEmptyArrayIsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	res, err := FetchPing(t.Context(), srv.URL, "ff02::6b", time.Second)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Empty(t, res)
}
