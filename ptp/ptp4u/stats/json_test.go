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
	"io"
	"net/http"
	"testing"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/stretchr/testify/require"
)

func TestJSONStatsReset(t *testing.T) {
	stats := JSONStats{}
	stats.subscriptions.init()
	stats.rxSignalingGrant.init()
	stats.rxSignalingCancel.init()
	stats.workerQueue.init()

	stats.IncSubscription(ptp.MessageAnnounce)
	stats.IncRXSignalingGrant(ptp.MessageSync)
	stats.IncRXSignalingCancel(ptp.MessageSync)
	stats.SetMaxWorkerQueue(10, 42)

	stats.Reset()
	require.Equal(t, int64(0), stats.subscriptions.load(int(ptp.MessageAnnounce)))
	require.Equal(t, int64(0), stats.rxSignalingGrant.load(int(ptp.MessageSync)))
	require.Equal(t, int64(0), stats.rxSignalingCancel.load(int(ptp.MessageSync)))
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

	stats.IncRXSignalingGrant(ptp.MessageSync)
	stats.IncRXSignalingCancel(ptp.MessageSync)
	require.Equal(t, int64(1), stats.rxSignalingGrant.load(int(ptp.MessageSync)))
	require.Equal(t, int64(1), stats.rxSignalingCancel.load(int(ptp.MessageSync)))

	stats.DecRXSignalingGrant(ptp.MessageSync)
	stats.DecRXSignalingCancel(ptp.MessageSync)
	require.Equal(t, int64(0), stats.rxSignalingGrant.load(int(ptp.MessageSync)))
	require.Equal(t, int64(0), stats.rxSignalingCancel.load(int(ptp.MessageSync)))
}

func TestJSONStatsTXSignaling(t *testing.T) {
	stats := NewJSONStats()

	stats.IncTXSignalingGrant(ptp.MessageSync)
	stats.IncTXSignalingCancel(ptp.MessageSync)
	require.Equal(t, int64(1), stats.txSignalingGrant.load(int(ptp.MessageSync)))
	require.Equal(t, int64(1), stats.txSignalingCancel.load(int(ptp.MessageSync)))

	stats.DecTXSignalingGrant(ptp.MessageSync)
	stats.DecTXSignalingCancel(ptp.MessageSync)
	require.Equal(t, int64(0), stats.txSignalingGrant.load(int(ptp.MessageSync)))
	require.Equal(t, int64(0), stats.txSignalingCancel.load(int(ptp.MessageSync)))
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

	stats.SetUTCOffsetSec(42)
	require.Equal(t, int64(42), stats.utcoffsetSec)
}

func TestJSONStatsSetClockAccuracy(t *testing.T) {
	stats := NewJSONStats()

	stats.SetClockAccuracy(42)
	require.Equal(t, int64(42), stats.clockaccuracy)
}

func TestJSONStatsSetClockCLass(t *testing.T) {
	stats := NewJSONStats()

	stats.SetClockClass(42)
	require.Equal(t, int64(42), stats.clockclass)
}

func TestJSONStatsSetDrain(t *testing.T) {
	stats := NewJSONStats()

	stats.SetDrain(1)
	require.Equal(t, int64(1), stats.drain)
}

func TestJSONStatsSnapshot(t *testing.T) {
	stats := NewJSONStats()

	go stats.Start(0)
	time.Sleep(time.Millisecond)

	stats.IncSubscription(ptp.MessageAnnounce)
	stats.IncTX(ptp.MessageSync)
	stats.IncTX(ptp.MessageSync)
	stats.IncRXSignalingGrant(ptp.MessageDelayResp)
	stats.IncRXSignalingGrant(ptp.MessageDelayResp)
	stats.IncRXSignalingGrant(ptp.MessageDelayResp)
	stats.SetClockAccuracy(1)
	stats.SetClockClass(1)
	stats.SetUTCOffsetSec(1)
	stats.SetDrain(1)
	stats.IncReload()

	stats.Snapshot()

	expectedStats := counters{}
	expectedStats.init()
	expectedStats.subscriptions.store(int(ptp.MessageAnnounce), 1)
	expectedStats.tx.store(int(ptp.MessageSync), 2)
	expectedStats.rxSignalingGrant.store(int(ptp.MessageDelayResp), 3)
	expectedStats.utcoffsetSec = 1
	expectedStats.clockaccuracy = 1
	expectedStats.clockclass = 1
	expectedStats.drain = 1
	expectedStats.reload = 1

	require.Equal(t, expectedStats.subscriptions.m, stats.report.subscriptions.m)
	require.Equal(t, expectedStats.tx.m, stats.report.tx.m)
	require.Equal(t, expectedStats.rxSignalingGrant.m, stats.report.rxSignalingGrant.m)
	require.Equal(t, expectedStats.utcoffsetSec, stats.report.utcoffsetSec)
	require.Equal(t, expectedStats.clockaccuracy, stats.report.clockaccuracy)
	require.Equal(t, expectedStats.clockclass, stats.report.clockclass)
	require.Equal(t, expectedStats.drain, stats.report.drain)
	require.Equal(t, expectedStats.reload, stats.report.reload)
}

func TestJSONExport(t *testing.T) {
	stats := NewJSONStats()

	go stats.Start(8888)
	time.Sleep(time.Second)

	stats.IncSubscription(ptp.MessageAnnounce)
	stats.IncTX(ptp.MessageSync)
	stats.IncTX(ptp.MessageSync)
	stats.IncRXSignalingGrant(ptp.MessageDelayResp)
	stats.IncRXSignalingGrant(ptp.MessageDelayResp)
	stats.IncRXSignalingGrant(ptp.MessageDelayResp)
	stats.IncRXSignalingCancel(ptp.MessageSync)
	stats.IncRXSignalingCancel(ptp.MessageSync)
	stats.SetUTCOffsetSec(1)
	stats.SetClockAccuracy(1)
	stats.SetClockClass(1)
	stats.SetDrain(1)
	stats.IncReload()

	stats.Snapshot()

	resp, err := http.Get("http://localhost:8888")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var data map[string]int64
	err = json.Unmarshal([]byte(body), &data)
	require.NoError(t, err)

	expectedMap := make(map[string]int64)
	expectedMap["subscriptions.announce"] = 1
	expectedMap["tx.sync"] = 2
	expectedMap["rx.signaling.grant.delay_resp"] = 3
	expectedMap["rx.signaling.cancel.sync"] = 2
	expectedMap["utcoffset_sec"] = 1
	expectedMap["clockaccuracy"] = 1
	expectedMap["clockclass"] = 1
	expectedMap["drain"] = 1
	expectedMap["reload"] = 1

	require.Equal(t, expectedMap, data)
}
