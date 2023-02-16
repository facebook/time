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

package stats

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/stretchr/testify/require"
)

func TestFetchStats(t *testing.T) {
	sampleResp := `
{
	"127.0.0.1": {"selected": false, "port_identity": "oleg", "clock_quality": {"clock_class": 6, "clock_accuracy": 33, "offset_scaled_log_variance": 42}, "priority1": 2, "priority2": 3, "priority3": 4, "offset": -42.42, "mean_path_delay": 42.42, "steps_removed": 3, "cf_rx": 10, "cf_tx": 20, "gm_present": 1, "error": ""},
	"::1": {"selected": true, "port_identity": "oleg1", "clock_quality": {"clock_class": 7, "clock_accuracy": 34, "offset_scaled_log_variance": 42}, "priority1": 2, "priority2": 3, "priority3": 4, "offset": -43.43, "mean_path_delay": 43.43, "steps_removed": 3, "cf_rx": 100000, "cf_tx": 20000, "gm_present": 0, "error": "oops"}
}
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, sampleResp)
	}))
	defer ts.Close()

	expected := map[string]Stats{
		"127.0.0.1": {
			Selected:     false,
			PortIdentity: "oleg",
			ClockQuality: ptp.ClockQuality{
				ClockClass:              ptp.ClockClass6,
				ClockAccuracy:           ptp.ClockAccuracyNanosecond100,
				OffsetScaledLogVariance: uint16(42),
			},
			Priority1:         2,
			Priority2:         3,
			Priority3:         4,
			Offset:            -42.42,
			MeanPathDelay:     42.42,
			StepsRemoved:      3,
			CorrectionFieldRX: 10,
			CorrectionFieldTX: 20,
			GMPresent:         1,
		},
		"::1": {
			Selected:     true,
			PortIdentity: "oleg1",
			ClockQuality: ptp.ClockQuality{
				ClockClass:              ptp.ClockClass7,
				ClockAccuracy:           ptp.ClockAccuracyNanosecond250,
				OffsetScaledLogVariance: uint16(42),
			},
			Priority1:         2,
			Priority2:         3,
			Priority3:         4,
			Offset:            -43.43,
			MeanPathDelay:     43.43,
			StepsRemoved:      3,
			CorrectionFieldRX: 100000,
			CorrectionFieldTX: 20000,
			GMPresent:         0,
			Error:             "oops",
		},
	}

	actual, err := FetchStats(ts.URL)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}
