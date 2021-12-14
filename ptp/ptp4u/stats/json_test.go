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
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/stretchr/testify/require"
)

func TestJSONStatsReset(t *testing.T) {
	stats := JSONStats{}
	stats.subscriptions.init()
	stats.rxSignaling.init()
	stats.workerQueue.init()

	stats.IncSubscription(ptp.MessageAnnounce)
	stats.IncRXSignaling(ptp.MessageSync)
	stats.SetMaxWorkerQueue(10, 42)

	stats.Reset()
	require.Equal(t, int64(0), stats.subscriptions.load(int(ptp.MessageAnnounce)))
	require.Equal(t, int64(0), stats.rxSignaling.load(int(ptp.MessageSync)))
	require.Equal(t, int64(0), stats.workerQueue.load(10))
}

func TestJSONStatsAnnounceSubscription(t *testing.T) {
	stats := NewJSONStats()

	stats.IncSubscription(ptp.MessageAnnounce)
	require.Equal(t, int64(1), stats.subscriptions.load(int(ptp.MessageAnnounce)))

	stats.DecSubscription(ptp.MessageAnnounce)
	require.Equal(t, int64(0), stats.subscriptions.load(int(ptp.MessageAnnounce)))
}

func TestJSONStatsSyncSubscription(t *testing.T) {
	stats := NewJSONStats()

	stats.IncSubscription(ptp.MessageSync)
	require.Equal(t, int64(1), stats.subscriptions.load(int(ptp.MessageSync)))

	stats.DecSubscription(ptp.MessageSync)
	require.Equal(t, int64(0), stats.subscriptions.load(int(ptp.MessageSync)))
}

func TestJSONStatsRX(t *testing.T) {
	stats := NewJSONStats()

	stats.IncRX(ptp.MessageSync)
	require.Equal(t, int64(1), stats.rx.load(int(ptp.MessageSync)))

	stats.DecRX(ptp.MessageSync)
	require.Equal(t, int64(0), stats.rx.load(int(ptp.MessageSync)))
}

func TestJSONStatsTX(t *testing.T) {
	stats := NewJSONStats()

	stats.IncTX(ptp.MessageSync)
	require.Equal(t, int64(1), stats.tx.load(int(ptp.MessageSync)))

	stats.DecTX(ptp.MessageSync)
	require.Equal(t, int64(0), stats.tx.load(int(ptp.MessageSync)))
}

func TestJSONStatsRXSignaling(t *testing.T) {
	stats := NewJSONStats()

	stats.IncRXSignaling(ptp.MessageSync)
	require.Equal(t, int64(1), stats.rxSignaling.load(int(ptp.MessageSync)))

	stats.DecRXSignaling(ptp.MessageSync)
	require.Equal(t, int64(0), stats.rxSignaling.load(int(ptp.MessageSync)))
}

func TestJSONStatsTXSignaling(t *testing.T) {
	stats := NewJSONStats()

	stats.IncTXSignaling(ptp.MessageSync)
	require.Equal(t, int64(1), stats.txSignaling.load(int(ptp.MessageSync)))

	stats.DecTXSignaling(ptp.MessageSync)
	require.Equal(t, int64(0), stats.txSignaling.load(int(ptp.MessageSync)))
}

func TestJSONStatsSetMaxWorkerQueue(t *testing.T) {
	stats := NewJSONStats()

	stats.SetMaxWorkerQueue(10, 42)
	require.Equal(t, int64(42), stats.workerQueue.load(10))
}

func TestJSONStatsWorkerSubs(t *testing.T) {
	stats := NewJSONStats()

	stats.IncWorkerSubs(10)
	require.Equal(t, int64(1), stats.workerSubs.load(10))

	stats.DecWorkerSubs(10)
	require.Equal(t, int64(0), stats.tx.load(10))
}

func TestJSONStatsSetMaxTXTSAttempts(t *testing.T) {
	stats := NewJSONStats()

	stats.SetMaxTXTSAttempts(10, 42)
	require.Equal(t, int64(42), stats.txtsattempts.load(10))
}

func TestJSONStatsSetUTCOffset(t *testing.T) {
	stats := NewJSONStats()

	stats.SetUTCOffset(42)
	require.Equal(t, int64(42), stats.utcoffset)
}

func TestJSONStatsSnapshot(t *testing.T) {
	stats := NewJSONStats()

	go stats.Start(0)
	time.Sleep(time.Millisecond)

	stats.IncSubscription(ptp.MessageAnnounce)
	stats.IncTX(ptp.MessageSync)
	stats.IncTX(ptp.MessageSync)
	stats.IncRXSignaling(ptp.MessageDelayResp)
	stats.IncRXSignaling(ptp.MessageDelayResp)
	stats.IncRXSignaling(ptp.MessageDelayResp)
	stats.SetUTCOffset(1)

	stats.Snapshot()

	expectedStats := counters{}
	expectedStats.init()
	expectedStats.subscriptions.store(int(ptp.MessageAnnounce), 1)
	expectedStats.tx.store(int(ptp.MessageSync), 2)
	expectedStats.rxSignaling.store(int(ptp.MessageDelayResp), 3)
	expectedStats.utcoffset = 1

	require.Equal(t, expectedStats.subscriptions.m, stats.report.subscriptions.m)
	require.Equal(t, expectedStats.tx.m, stats.report.tx.m)
	require.Equal(t, expectedStats.rxSignaling.m, stats.report.rxSignaling.m)
	require.Equal(t, expectedStats.utcoffset, stats.report.utcoffset)
}

func TestJSONExport(t *testing.T) {
	stats := NewJSONStats()

	go stats.Start(8888)
	time.Sleep(time.Second)

	stats.IncSubscription(ptp.MessageAnnounce)
	stats.IncTX(ptp.MessageSync)
	stats.IncTX(ptp.MessageSync)
	stats.IncRXSignaling(ptp.MessageDelayResp)
	stats.IncRXSignaling(ptp.MessageDelayResp)
	stats.IncRXSignaling(ptp.MessageDelayResp)
	stats.SetUTCOffset(1)

	stats.Snapshot()

	resp, err := http.Get("http://localhost:8888")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	var data map[string]int64
	err = json.Unmarshal([]byte(body), &data)
	require.NoError(t, err)

	expectedMap := make(map[string]int64)
	expectedMap["subscriptions.announce"] = 1
	expectedMap["tx.sync"] = 2
	expectedMap["rx.signaling.delay_resp"] = 3
	expectedMap["utcoffset"] = 1

	require.Equal(t, expectedMap, data)
}
