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
	"fmt"
	"testing"
	"time"

	"github.com/facebook/time/fbclock/stats"
	ptp "github.com/facebook/time/ptp/protocol"

	"github.com/stretchr/testify/require"
)

func TestProcessMonitoringResults(t *testing.T) {
	s := stats.NewStats()
	results := map[string]TestResult{
		"server01.nha1": PTPTestResult{ // tests pass
			Server:      "192.168.0.10",
			Error:       nil,
			TXTimestamp: time.Unix(0, 1653574589806127700),
			RXTimestamp: time.Unix(0, 1653574589806127800),
		},
		"server02.nha1": PTPTestResult{ // tests failed - TX after RX
			Server:      "192.168.0.11",
			Error:       nil,
			TXTimestamp: time.Unix(0, 1653574589806127730),
			RXTimestamp: time.Unix(0, 1653574589806127600),
		},
		"server03.nha1": PTPTestResult{ // test failed - drained server/grant denied
			Server:      "192.168.0.12",
			Error:       ErrGrantDenied,
			TXTimestamp: time.Time{},
			RXTimestamp: time.Time{},
		},
		"server04.nha1": PTPTestResult{ // tests pass
			Server:      "192.168.0.13",
			Error:       nil,
			TXTimestamp: time.Unix(0, 1653574589806127900),
			RXTimestamp: time.Unix(0, 1653574589806127930),
		},
		"server05.nha1": PTPTestResult{ // test failed - err != nil
			Server:      "192.168.0.14",
			Error:       fmt.Errorf("ooops"),
			TXTimestamp: time.Time{},
			RXTimestamp: time.Time{},
		},
	}

	ProcessMonitoringResults("ptp.linearizability.", results, s)

	c := s.Get()
	require.Equal(t, int64(0), c["ptp.linearizability.failed"])
}

func TestProcessMonitoringResultsSPTP(t *testing.T) {
	s := stats.NewStats()
	results := map[string]TestResult{ // test failed - offset > max
		"server01.nha1": SPTPHTTPTestResult{
			Config: SPTPHTTPTestConfig{
				Server:                         "192.168.0.11",
				LinearizabilityTestMaxGMOffset: time.Nanosecond,
			},
			Offset:     42.0,
			Error:      nil,
			ClockClass: ptp.ClockClass6,
		},
		"server02.nha1": SPTPHTTPTestResult{ // test failed - error
			Config: SPTPHTTPTestConfig{
				Server:                         "192.168.0.12",
				LinearizabilityTestMaxGMOffset: time.Second,
			},
			Offset:     42.0,
			Error:      fmt.Errorf("ooops"),
			ClockClass: ptp.ClockClass6,
		},
	}

	ProcessMonitoringResults("ptp.linearizability.", results, s)

	c := s.Get()
	require.Equal(t, int64(1), c["ptp.linearizability.failed"])
}

func TestB2I(t *testing.T) {
	require.Equal(t, int64(0), b2i[false])
	require.Equal(t, int64(1), b2i[true])
}
