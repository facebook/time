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

func TestSPTPTestResultGood(t *testing.T) {
	testCases := []struct {
		name    string
		in      SPTPTestResult
		want    bool
		wantErr bool
	}{
		{
			name: "error",
			in: SPTPTestResult{
				Server: "time01",
				Error:  fmt.Errorf("test error"),
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "fail",
			in: SPTPTestResult{
				Server: "time01",
				Offset: float64(time.Millisecond.Nanoseconds()),
				Error:  nil,
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "pass",
			in: SPTPTestResult{
				Server: "time01",
				Offset: float64(time.Nanosecond),
				Error:  nil,
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

func TestSPTPTestResultExplain(t *testing.T) {
	testCases := []struct {
		name string
		in   SPTPTestResult
		want string
	}{
		{
			name: "error",
			in: SPTPTestResult{
				Server: "time01",
				Error:  fmt.Errorf("test error"),
			},
			want: "linearizability test against \"time01\" couldn't be completed because of error: test error",
		},
		{
			name: "fail",
			in: SPTPTestResult{
				Server: "time01",
				Offset: float64(time.Millisecond.Nanoseconds()),
				Error:  nil,
			},
			want: "linearizability test against \"time01\" failed because the offset 1000000.00ns is > 1µs",
		},
		{
			name: "pass",
			in: SPTPTestResult{
				Server: "time01",
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
	expectedGood := SPTPTestResult{
		Server: "127.0.0.1",
		Offset: -42.42,
		Error:  nil,
	}
	expectedBad := SPTPTestResult{
		Server: "::1",
		Offset: -43.43,
		Error:  fmt.Errorf("oops"),
	}
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
	lt, err := NewSPTPTester("127.0.0.1", ts.URL)
	require.NoError(t, err)

	testResult := lt.RunTest(context.Background())
	require.Equal(t, expectedGood, testResult)
	require.Equal(t, &expectedGood, lt.result)
	require.NoError(t, testResult.Err())

	// Bad (error)
	lt, err = NewSPTPTester("::1", ts.URL)
	require.NoError(t, err)

	testResult = lt.RunTest(context.Background())
	require.Equal(t, expectedBad, testResult)
	require.Equal(t, &expectedBad, lt.result)
	require.Error(t, testResult.Err())

	// Bad (can't connect)
	lt, err = NewSPTPTester("::1", "blah")
	require.NoError(t, err)

	testResult = lt.RunTest(context.Background())
	require.Error(t, testResult.Err())
}
