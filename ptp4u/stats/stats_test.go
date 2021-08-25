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
	"testing"

	ptp "github.com/facebookincubator/ptp/protocol"
	"github.com/stretchr/testify/require"
)

func TestSyncMapInt64Keys(t *testing.T) {
	s := syncMapInt64{}
	s.init()

	expected := []int{24, 42}
	for _, i := range expected {
		s.inc(i)
	}

	found := 0
	for _, k := range s.keys() {
		for _, i := range expected {
			if i == k {
				found++
				break
			}
		}
	}

	require.Equal(t, len(expected), found)
}

func TestSyncMapInt64Copy(t *testing.T) {
	s := syncMapInt64{}
	s.init()

	s.store(1, 1)
	require.Equal(t, int64(1), s.load(1))

	dst := syncMapInt64{}
	dst.init()

	s.copy(&dst)
	require.Equal(t, s.m, dst.m)
	require.Equal(t, int64(1), dst.load(1))
}

func TestSyncMapInt64Counters(t *testing.T) {
	c := counters{}
	c.init()

	c.subscriptions.store(1, 1)
	c.rx.store(1, 1)
	c.tx.store(1, 1)
	c.rxSignaling.store(1, 1)
	c.txSignaling.store(1, 1)
	c.workerQueue.store(1, 1)
	c.txtsattempts.store(1, 1)
	c.utcoffset = 1

	require.Equal(t, int64(1), c.subscriptions.load(1))
	require.Equal(t, int64(1), c.rx.load(1))
	require.Equal(t, int64(1), c.tx.load(1))
	require.Equal(t, int64(1), c.rxSignaling.load(1))
	require.Equal(t, int64(1), c.txSignaling.load(1))
	require.Equal(t, int64(1), c.workerQueue.load(1))
	require.Equal(t, int64(1), c.txtsattempts.load(1))
	require.Equal(t, int64(1), c.utcoffset)

	c.reset()

	require.Equal(t, int64(0), c.subscriptions.load(1))
	require.Equal(t, int64(0), c.rx.load(1))
	require.Equal(t, int64(0), c.tx.load(1))
	require.Equal(t, int64(0), c.rxSignaling.load(1))
	require.Equal(t, int64(0), c.txSignaling.load(1))
	require.Equal(t, int64(0), c.workerQueue.load(1))
	require.Equal(t, int64(0), c.txtsattempts.load(1))
	require.Equal(t, int64(0), c.utcoffset)
}

func TestCountersToMap(t *testing.T) {
	c := counters{}
	c.init()

	c.subscriptions.store(int(ptp.MessageAnnounce), 1)
	c.tx.store(int(ptp.MessageSync), 2)
	c.rxSignaling.store(int(ptp.MessageDelayResp), 3)
	c.utcoffset = 1

	result := c.toMap()

	expectedMap := make(map[string]int64)
	expectedMap["subscriptions.announce"] = 1
	expectedMap["tx.sync"] = 2
	expectedMap["rx.signaling.delay_resp"] = 3
	expectedMap["utcoffset"] = 1

	require.Equal(t, expectedMap, result)
}
