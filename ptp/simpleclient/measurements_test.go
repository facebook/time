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

package simpleclient

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeasurementsFullRun(t *testing.T) {
	m := newMeasurements()
	var syncSeq uint16 = 1
	var delaySeq uint16 = 28
	t.Run("symmetrical delay, no offset", func(t *testing.T) {
		netDelay := 100 * time.Millisecond
		netDelayBack := netDelay

		timeSync, err := time.Parse(time.RFC3339, "2021-05-21T13:32:05+01:00")
		require.Nil(t, err)

		// time when we received SYNC
		m.addSync(syncSeq, timeSync, 0)
		// time when SYNC was actually sent by GM
		m.addFollowUp(syncSeq, timeSync.Add(-netDelay), 0)
		// time when we sent out DELAY_REQ
		timeDelaySent := timeSync.Add(10 * time.Millisecond)
		m.addDelayReq(delaySeq, timeDelaySent)
		// time when DELAY_REQ was received by GM
		timeLastPacket := timeDelaySent.Add(netDelayBack)
		m.addDelayResp(delaySeq, timeLastPacket, 0)

		got, err := m.latest()
		require.Nil(t, err)
		want := &MeasurementResult{
			Delay:              netDelay,
			ServerToClientDiff: netDelay,
			ClientToServerDiff: netDelayBack,
			Offset:             0,
			Timestamp:          timeLastPacket,
		}
		assert.Equal(t, want, got)
	})

	t.Run("asymmetrical delay, some offset", func(t *testing.T) {
		netDelay := 200 * time.Millisecond
		netDelayBack := 2 * netDelay

		timeSync, err := time.Parse(time.RFC3339, "2021-05-21T13:32:05+01:00")
		require.Nil(t, err)

		// time when we received SYNC
		m.addSync(syncSeq, timeSync, 0)
		// time when SYNC was actually sent by GM
		m.addFollowUp(syncSeq, timeSync.Add(-netDelay), 0)
		// time when we sent out DELAY_REQ
		timeDelaySent := timeSync.Add(10 * time.Millisecond)
		m.addDelayReq(delaySeq, timeDelaySent)
		// time when DELAY_REQ was received by GM
		timeLastPacket := timeDelaySent.Add(netDelayBack)
		m.addDelayResp(delaySeq, timeLastPacket, 0)

		got, err := m.latest()
		require.Nil(t, err)
		want := &MeasurementResult{
			Delay:              300 * time.Millisecond,
			ServerToClientDiff: netDelay,
			ClientToServerDiff: netDelayBack,
			Offset:             -100 * time.Millisecond,
			Timestamp:          timeLastPacket,
		}
		assert.Equal(t, want, got)
	})

	t.Run("asymmetrical delay, some offset and correction", func(t *testing.T) {
		netDelay := 200 * time.Millisecond
		netDelayBack := 2 * netDelay
		netCorrection := 6 * time.Microsecond
		netCorrectionBack := 4 * time.Microsecond

		timeSync, err := time.Parse(time.RFC3339, "2021-05-21T13:32:05+01:00")
		require.Nil(t, err)

		// time when we received SYNC
		m.addSync(syncSeq, timeSync, netCorrection)
		// time when SYNC was actually sent by GM
		m.addFollowUp(syncSeq, timeSync.Add(-netDelay), 0)
		// time when we sent out DELAY_REQ
		timeDelaySent := timeSync.Add(10 * time.Millisecond)
		m.addDelayReq(delaySeq, timeDelaySent)
		// time when DELAY_REQ was received by GM
		timeLastPacket := timeDelaySent.Add(netDelayBack)
		m.addDelayResp(delaySeq, timeLastPacket, netCorrectionBack)

		got, err := m.latest()
		require.Nil(t, err)
		want := &MeasurementResult{
			Delay:              299995 * time.Microsecond,
			ServerToClientDiff: netDelay - netCorrection,
			ClientToServerDiff: netDelayBack - netCorrectionBack,
			Offset:             -100001 * time.Microsecond,
			Timestamp:          timeLastPacket,
		}
		assert.Equal(t, want, got)
	})
}
