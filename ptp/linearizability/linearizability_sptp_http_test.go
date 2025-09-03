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

package linearizability

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSPTPHTTPTestResultGood(t *testing.T) {
	testCases := []struct {
		name    string
		in      SPTPHTTPTestResult
		want    bool
		wantErr bool
	}{
		{
			name: "error",
			in: SPTPHTTPTestResult{
				Config: SPTPHTTPTestConfig{Server: "time01", LinearizabilityTestMaxGMOffset: 3 * time.Microsecond},
				Error:  fmt.Errorf("test error"),
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "fail",
			in: SPTPHTTPTestResult{
				Config:     SPTPHTTPTestConfig{Server: "time01", LinearizabilityTestMaxGMOffset: 3 * time.Microsecond},
				Offset:     float64(time.Millisecond.Nanoseconds()),
				ClockClass: 6,
				Error:      nil,
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "pass",
			in: SPTPHTTPTestResult{
				Config:     SPTPHTTPTestConfig{Server: "time01", LinearizabilityTestMaxGMOffset: 3 * time.Microsecond},
				Offset:     float64(time.Nanosecond),
				ClockClass: 6,
				Error:      nil,
			},
			want:    true,
			wantErr: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.in.Good()
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got, "Good() for %+v must return %v", tc.in, tc.want)
			}
		})
	}
}

func TestSPTPHTTPTestResultExplain(t *testing.T) {
	testCases := []struct {
		name string
		in   SPTPHTTPTestResult
		want string
	}{
		{
			name: "error",
			in: SPTPHTTPTestResult{
				Config: SPTPHTTPTestConfig{Server: "time01"},
				Error:  fmt.Errorf("test error"),
			},
			want: "linearizability test against \"time01\" couldn't be completed because of error: test error",
		},
		{
			name: "fail",
			in: SPTPHTTPTestResult{
				Config:     SPTPHTTPTestConfig{Server: "time01", LinearizabilityTestMaxGMOffset: 3 * time.Microsecond},
				Offset:     float64(time.Millisecond.Nanoseconds()),
				ClockClass: 6,
				Error:      nil,
			},
			want: "linearizability test against \"time01\" failed because the offset 1000000.00ns is > 3µs",
		},
		{
			name: "pass",
			in: SPTPHTTPTestResult{
				Config:     SPTPHTTPTestConfig{Server: "time01", LinearizabilityTestMaxGMOffset: 3 * time.Microsecond},
				Offset:     float64(time.Millisecond.Nanoseconds()),
				ClockClass: 52,
				Error:      nil,
			},
			want: "linearizability test against \"time01\" passed",
		},
		{
			name: "pass",
			in: SPTPHTTPTestResult{
				Config: SPTPHTTPTestConfig{Server: "time01", LinearizabilityTestMaxGMOffset: 3 * time.Microsecond},
				Offset: float64(time.Nanosecond),
				Error:  nil,
			},
			want: "linearizability test against \"time01\" passed",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.Explain()
			require.Equal(t, tc.want, got, "Explain() for %+v must return %v", tc.in, tc.want)
		})
	}
}

func TestSPTPRunTest(t *testing.T) {
	sampleResp := `
	[
		{"gm_address": "127.0.0.1", "selected": false, "port_identity": "oleg", "clock_quality": {"clock_class": 6, "clock_accuracy": 33, "offset_scaled_log_variance": 42}, "priority1": 2, "priority2": 3, "priority3": 4, "offset": -42.42, "mean_path_delay": 42.42, "steps_removed": 3, "cf_rx": 10, "cf_tx": 20, "gm_present": 1, "error": ""},
		{"gm_address": "::1", "selected": true, "port_identity": "oleg1", "clock_quality": {"clock_class": 7, "clock_accuracy": 34, "offset_scaled_log_variance": 42}, "priority1": 2, "priority2": 3, "priority3": 4, "offset": -43.43, "mean_path_delay": 43.43, "steps_removed": 3, "cf_rx": 100000, "cf_tx": 20000, "gm_present": 0, "error": "oops"}
	]
	`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	// Good
	lt, err := NewSPTPHTTPTester("127.0.0.1", ts.URL, 3*time.Microsecond)
	require.NoError(t, err)

	expectedGood := SPTPHTTPTestResult{
		Config:     SPTPHTTPTestConfig{Server: "127.0.0.1", sptpurl: ts.URL, LinearizabilityTestMaxGMOffset: 3 * time.Microsecond},
		Offset:     -42.42,
		ClockClass: 6,
		Error:      nil,
	}

	testResult := lt.RunTest(context.Background())
	require.Equal(t, expectedGood, testResult)
	require.Equal(t, &expectedGood, lt.result)
	require.NoError(t, testResult.Err())

	// Bad (error)
	lt, err = NewSPTPHTTPTester("::1", ts.URL, 3*time.Microsecond)
	require.NoError(t, err)

	expectedBad := SPTPHTTPTestResult{
		Config:     SPTPHTTPTestConfig{Server: "::1", sptpurl: ts.URL, LinearizabilityTestMaxGMOffset: 3 * time.Microsecond},
		Offset:     -43.43,
		ClockClass: 7,
		Error:      fmt.Errorf("oops"),
	}

	testResult = lt.RunTest(context.Background())
	require.Equal(t, expectedBad, testResult)
	require.Equal(t, &expectedBad, lt.result)
	require.Error(t, testResult.Err())

	// Bad (can't connect)
	lt, err = NewSPTPHTTPTester("::1", "blah", 3*time.Microsecond)
	require.NoError(t, err)

	testResult = lt.RunTest(context.Background())
	require.Error(t, testResult.Err())
}
