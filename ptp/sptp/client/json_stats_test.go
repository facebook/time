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
	"net/http"
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
	stats, err := NewJSONStats()
	require.NoError(t, err)
	port, err := getFreePort()
	require.Nil(t, err, "Failed to allocate port")
	url := fmt.Sprintf("http://localhost:%d", port)
	go stats.Start(port, time.Second)
	time.Sleep(time.Second)

	stats.SetTickDuration(time.Millisecond)

	gm0 := &gmstats.Stat{
		GMAddress: "192.168.0.10",
		Error:     "mymy",
	}
	stats.SetGMStats(gm0)

	gm1 := &gmstats.Stat{
		GMAddress:         "192.168.0.13",
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
	stats.SetGMStats(gm1)

	counters, err := gmstats.FetchCounters(url)
	require.NoError(t, err)
	require.Equal(t, int64(1000000), counters["ptp.sptp.tick_duration_ns"])

	gms, err := gmstats.FetchStats(url)
	require.NoError(t, err)
	expectedStats := gmstats.Stats{
		gm0,
		gm1,
	}
	require.Equal(t, expectedStats, gms)
}

func TestHeaders(t *testing.T) {
	stats, err := NewJSONStats()
	require.NoError(t, err)
	port, err := getFreePort()
	require.Nil(t, err, "Failed to allocate port")
	url := fmt.Sprintf("http://localhost:%d", port)
	go stats.Start(port, time.Second)
	time.Sleep(time.Second)

	c := http.Client{
		Timeout: time.Second * 2,
	}

	resp, err := c.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, applicationJSON, resp.Header.Get(contentType))
}
