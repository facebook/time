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
	"fmt"
	"net"
	"testing"
	"time"

	gmstats "github.com/facebook/time/ptp/sptp/stats"

	"github.com/stretchr/testify/require"
)

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func TestJSONStats(t *testing.T) {
	stats := NewJSONStats()
	port, err := getFreePort()
	require.Nil(t, err, "Failed to allocate port")
	url := fmt.Sprintf("http://localhost:%d", port)
	go stats.Start(port)
	time.Sleep(time.Second)

	stats.SetCounter("some.counter", 1)
	stats.SetCounter("whatever", 42)

	gm0 := &gmstats.Stats{
		Error: "mymy",
	}
	stats.SetGMStats("192.168.0.10", gm0)

	gm1 := &gmstats.Stats{
		Error:             "",
		GMPresent:         1,
		IngressTime:       1676997604198536785,
		MeanPathDelay:     float64(299995 * time.Microsecond),
		Offset:            float64(-100001 * time.Microsecond),
		PortIdentity:      "000000.0086.09c621",
		Priority1:         1,
		Priority2:         2,
		Priority3:         3,
		Selected:          true,
		StepsRemoved:      1,
		CorrectionFieldRX: int64(6 * time.Microsecond),
		CorrectionFieldTX: int64(4 * time.Microsecond),
	}
	stats.SetGMStats("192.168.0.13", gm1)

	counters, err := gmstats.FetchCounters(url)
	require.NoError(t, err)
	expectedCounters := gmstats.Counters(map[string]int64{
		"some.counter": 1,
		"whatever":     42,
	})
	require.Equal(t, expectedCounters, counters)

	gms, err := gmstats.FetchStats(url)
	require.NoError(t, err)
	expectedStats := map[string]gmstats.Stats{
		"192.168.0.10": *gm0,
		"192.168.0.13": *gm1,
	}
	require.Equal(t, expectedStats, gms)
}
