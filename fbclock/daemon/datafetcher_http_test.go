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

package daemon

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFetchGMs(t *testing.T) {
	sampleResp := `
[
	{"gm_address": "127.0.0.1", "selected": false, "port_identity": "oleg", "clock_quality": {"clock_class": 6, "clock_accuracy": 33, "offset_scaled_log_variance": 42}, "priority1": 2, "priority2": 3, "priority3": 4, "offset": -42.42, "mean_path_delay": 42.42, "steps_removed": 3, "gm_present": 1, "error": ""},
	{"gm_address": "::1", "selected": true, "port_identity": "oleg1", "clock_quality": {"clock_class": 7, "clock_accuracy": 34, "offset_scaled_log_variance": 42}, "priority1": 2, "priority2": 3, "priority3": 4, "offset": -43.43, "mean_path_delay": 43.43, "steps_removed": 3, "gm_present": 0, "error": "oops"}
]
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()
	surl, err := url.Parse(ts.URL)
	require.Nil(t, err)
	target := fmt.Sprintf("%s:%s", surl.Hostname(), surl.Port())
	cfg := &Config{
		PTPClientAddress: target,
	}
	fetcher := &HTTPFetcher{}
	hosts, err := fetcher.FetchGMs(cfg)
	require.Nil(t, err)
	require.Equal(t, []string{"127.0.0.1"}, hosts)
}

func TestFetchStats(t *testing.T) {
	sampleResp := `
[
	{"gm_address": "127.0.0.1", "selected": false, "port_identity": "oleg", "clock_quality": {"clock_class": 6, "clock_accuracy": 33, "offset_scaled_log_variance": 42}, "priority1": 2, "priority2": 3, "priority3": 4, "offset": -42.42, "mean_path_delay": 42.42, "steps_removed": 3, "gm_present": 1, "error": ""},
	{"gm_address": "::1", "selected": true, "port_identity": "oleg1", "clock_quality": {"clock_class": 7, "clock_accuracy": 34, "offset_scaled_log_variance": 42}, "priority1": 2, "priority2": 3, "priority3": 4, "offset": -43.43, "mean_path_delay": 43.43, "steps_removed": 3, "gm_present": 1, "servo_state": 2}
]
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()
	surl, err := url.Parse(ts.URL)
	require.Nil(t, err)
	target := fmt.Sprintf("%s:%s", surl.Hostname(), surl.Port())
	cfg := &Config{
		PTPClientAddress: target,
	}
	fetcher := &HTTPFetcher{}
	sstats, err := fetcher.FetchStats(cfg)
	require.Nil(t, err)
	expected := DataPoint{IngressTimeNS: 0, MasterOffsetNS: -43.43, PathDelayNS: 43.43, FreqAdjustmentPPB: 0, ClockAccuracyNS: 250, ServoState: 2}
	require.Equal(t, sstats, &expected)
}

func TestFetchStatsServoNotLocked(t *testing.T) {
	sampleResp := `
[
	{"gm_address": "::1", "selected": true, "port_identity": "oleg1", "clock_quality": {"clock_class": 7, "clock_accuracy": 34, "offset_scaled_log_variance": 42}, "priority1": 2, "priority2": 3, "priority3": 4, "offset": -43.43, "mean_path_delay": 43.43, "steps_removed": 3, "gm_present": 1, "servo_state": 0}
]
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()
	surl, err := url.Parse(ts.URL)
	require.Nil(t, err)
	target := fmt.Sprintf("%s:%s", surl.Hostname(), surl.Port())
	cfg := &Config{
		PTPClientAddress: target,
	}
	fetcher := &HTTPFetcher{}
	sstats, err := fetcher.FetchStats(cfg)
	require.Nil(t, err)
	expected := DataPoint{IngressTimeNS: 0, MasterOffsetNS: -43.43, PathDelayNS: 43.43, FreqAdjustmentPPB: 0, ClockAccuracyNS: 250, ServoState: 0}
	require.Equal(t, sstats, &expected)
}

func TestFetchStatsNoSelectedGM(t *testing.T) {
	sampleResp := `
[
	{"gm_address": "127.0.0.1", "selected": false, "port_identity": "gm1", "clock_quality": {"clock_class": 6, "clock_accuracy": 33, "offset_scaled_log_variance": 42}, "offset": -42.42, "mean_path_delay": 42.42, "gm_present": 1, "error": ""},
	{"gm_address": "127.0.0.2", "selected": false, "port_identity": "gm2", "clock_quality": {"clock_class": 6, "clock_accuracy": 33, "offset_scaled_log_variance": 42}, "offset": -10.0, "mean_path_delay": 20.0, "gm_present": 1, "error": ""}
]
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()
	surl, err := url.Parse(ts.URL)
	require.NoError(t, err)
	cfg := &Config{PTPClientAddress: fmt.Sprintf("%s:%s", surl.Hostname(), surl.Port())}
	fetcher := &HTTPFetcher{}
	_, err = fetcher.FetchStats(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no selected grandmaster")
}

func TestFetchStatsGMNotPresent(t *testing.T) {
	sampleResp := `
[
	{"gm_address": "::1", "selected": true, "port_identity": "gm1", "clock_quality": {"clock_class": 6, "clock_accuracy": 33, "offset_scaled_log_variance": 42}, "offset": -50.5, "mean_path_delay": 100.0, "gm_present": 0, "servo_state": 2}
]
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()
	surl, err := url.Parse(ts.URL)
	require.NoError(t, err)
	cfg := &Config{PTPClientAddress: fmt.Sprintf("%s:%s", surl.Hostname(), surl.Port())}
	fetcher := &HTTPFetcher{}
	dp, err := fetcher.FetchStats(cfg)
	require.NoError(t, err)
	// gm_present=0 means ClockAccuracyNS should be 0
	require.Equal(t, float64(0), dp.ClockAccuracyNS)
	require.Equal(t, -50.5, dp.MasterOffsetNS)
	require.Equal(t, 100.0, dp.PathDelayNS)
}

func TestFetchStatsWithIngressTime(t *testing.T) {
	sampleResp := `
[
	{"gm_address": "::1", "selected": true, "port_identity": "gm1", "clock_quality": {"clock_class": 6, "clock_accuracy": 33, "offset_scaled_log_variance": 42}, "offset": -23.5, "mean_path_delay": 150.0, "gm_present": 1, "ingress_time": 1674148530671467104, "servo_state": 2}
]
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()
	surl, err := url.Parse(ts.URL)
	require.NoError(t, err)
	cfg := &Config{PTPClientAddress: fmt.Sprintf("%s:%s", surl.Hostname(), surl.Port())}
	fetcher := &HTTPFetcher{}
	dp, err := fetcher.FetchStats(cfg)
	require.NoError(t, err)
	require.Equal(t, int64(1674148530671467104), dp.IngressTimeNS)
	require.Equal(t, -23.5, dp.MasterOffsetNS)
	require.Equal(t, 150.0, dp.PathDelayNS)
	require.Equal(t, 2, dp.ServoState)
}

func TestFetchGMsMultipleWithFiltering(t *testing.T) {
	sampleResp := `
[
	{"gm_address": "10.0.0.1", "selected": true, "port_identity": "best", "clock_quality": {"clock_class": 6, "clock_accuracy": 33, "offset_scaled_log_variance": 42}, "offset": -1.0, "mean_path_delay": 5.0, "gm_present": 1, "error": ""},
	{"gm_address": "10.0.0.2", "selected": false, "port_identity": "gm2", "clock_quality": {"clock_class": 6, "clock_accuracy": 33, "offset_scaled_log_variance": 42}, "offset": -2.0, "mean_path_delay": 10.0, "gm_present": 1, "error": ""},
	{"gm_address": "10.0.0.3", "selected": false, "port_identity": "gm3", "clock_quality": {"clock_class": 6, "clock_accuracy": 33, "offset_scaled_log_variance": 42}, "offset": -3.0, "mean_path_delay": 15.0, "gm_present": 1, "error": "timeout"},
	{"gm_address": "10.0.0.4", "selected": false, "port_identity": "gm4", "clock_quality": {"clock_class": 6, "clock_accuracy": 33, "offset_scaled_log_variance": 42}, "offset": -4.0, "mean_path_delay": 20.0, "gm_present": 1, "error": ""}
]
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()
	surl, err := url.Parse(ts.URL)
	require.NoError(t, err)
	cfg := &Config{PTPClientAddress: fmt.Sprintf("%s:%s", surl.Hostname(), surl.Port())}
	fetcher := &HTTPFetcher{}
	hosts, err := fetcher.FetchGMs(cfg)
	require.NoError(t, err)
	// 10.0.0.1 skipped (selected), 10.0.0.3 skipped (has error)
	require.Equal(t, []string{"10.0.0.2", "10.0.0.4"}, hosts)
}

func TestFetchStatsHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer ts.Close()
	surl, err := url.Parse(ts.URL)
	require.NoError(t, err)
	cfg := &Config{PTPClientAddress: fmt.Sprintf("%s:%s", surl.Hostname(), surl.Port())}
	fetcher := &HTTPFetcher{}
	_, err = fetcher.FetchStats(cfg)
	require.Error(t, err)
}

func TestFetchGMsHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer ts.Close()
	surl, err := url.Parse(ts.URL)
	require.NoError(t, err)
	cfg := &Config{PTPClientAddress: fmt.Sprintf("%s:%s", surl.Hostname(), surl.Port())}
	fetcher := &HTTPFetcher{}
	_, err = fetcher.FetchGMs(cfg)
	require.Error(t, err)
}
