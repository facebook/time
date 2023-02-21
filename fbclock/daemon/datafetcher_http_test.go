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
{
	"127.0.0.1": {"selected": false, "port_identity": "oleg", "clock_quality": {"clock_class": 6, "clock_accuracy": 33, "offset_scaled_log_variance": 42}, "priority1": 2, "priority2": 3, "priority3": 4, "offset": -42.42, "mean_path_delay": 42.42, "steps_removed": 3, "gm_present": 1, "error": ""},
	"::1": {"selected": true, "port_identity": "oleg1", "clock_quality": {"clock_class": 7, "clock_accuracy": 34, "offset_scaled_log_variance": 42}, "priority1": 2, "priority2": 3, "priority3": 4, "offset": -43.43, "mean_path_delay": 43.43, "steps_removed": 3, "gm_present": 0, "error": "oops"}
}
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
{
	"127.0.0.1": {"selected": false, "port_identity": "oleg", "clock_quality": {"clock_class": 6, "clock_accuracy": 33, "offset_scaled_log_variance": 42}, "priority1": 2, "priority2": 3, "priority3": 4, "offset": -42.42, "mean_path_delay": 42.42, "steps_removed": 3, "gm_present": 1, "error": ""},
	"::1": {"selected": true, "port_identity": "oleg1", "clock_quality": {"clock_class": 7, "clock_accuracy": 34, "offset_scaled_log_variance": 42}, "priority1": 2, "priority2": 3, "priority3": 4, "offset": -43.43, "mean_path_delay": 43.43, "steps_removed": 3, "gm_present": 1}
}
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
	expected := DataPoint{IngressTimeNS: 0, MasterOffsetNS: -43.43, PathDelayNS: 43.43, FreqAdjustmentPPB: 0, ClockAccuracyNS: 250}
	require.Equal(t, sstats, &expected)
}
