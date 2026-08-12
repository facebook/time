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
	"testing"
	"time"

	"github.com/facebook/time/fbclock/stats"
	ptp "github.com/facebook/time/ptp/protocol"

	"github.com/stretchr/testify/require"
)

func healthyPTP4lReplies() (*ptp.TimeStatusNPTLV, *ptp.ParentDataSetTLV, *ptp.CurrentDataSetTLV) {
	return &ptp.TimeStatusNPTLV{
			MasterOffsetNS: 24536521,
			IngressTimeNS:  1615472167079671311,
			GMPresent:      1,
		},
		&ptp.ParentDataSetTLV{
			GrandmasterClockQuality: ptp.ClockQuality{ClockAccuracy: ptp.ClockAccuracyNanosecond100},
		},
		&ptp.CurrentDataSetTLV{MeanPathDelay: ptp.NewTimeInterval(213.0)}
}

func TestSockDataPointPassesSanityCheck(t *testing.T) {
	dp := sockDataPoint(healthyPTP4lReplies())
	// Run fills this in from the PHC before doWork runs SanityCheck.
	dp.FreqAdjustmentPPB = 212131

	require.True(t, dp.ServoStateUnavailable)
	require.NoError(t, dp.SanityCheck())
}

func TestSockDataPointFields(t *testing.T) {
	dp := sockDataPoint(healthyPTP4lReplies())

	require.Equal(t, int64(1615472167079671311), dp.IngressTimeNS)
	require.InDelta(t, 24536521.0, dp.MasterOffsetNS, 0)
	require.InDelta(t, 213.0, dp.PathDelayNS, 0)
	require.InDelta(t, 100.0, dp.ClockAccuracyNS, 0)
}

func TestPTP4lDataPointsReachSHM(t *testing.T) {
	cfg := &Config{
		RingSize: 30,
		Math: Math{
			M:     "mean(clockaccuracy, 30) + abs(mean(offset, 30)) + 1.0 * stddev(offset, 30)",
			W:     "mean(m, 30) + 4.0 * stddev(m, 30)",
			Drift: "1.5 * mean(freqchangeabs, 29)",
		},
	}
	require.NoError(t, cfg.Math.Prepare())
	st := stats.NewStats()
	s := newTestDaemon(cfg, st)

	status, pds, cds := healthyPTP4lReplies()
	baseIngressNS := status.IngressTimeNS
	adj := 212131.0
	for i := range cfg.RingSize {
		adj += float64(i)
		status.IngressTimeNS = baseIngressNS + int64(i)*int64(time.Second)
		dp := sockDataPoint(status, pds, cds)
		dp.FreqAdjustmentPPB = adj

		shmData, err := s.calculateSHMData(dp, nil)
		if i < cfg.RingSize-1 {
			require.ErrorIs(t, err, errNotEnoughData)
			continue
		}
		require.NoError(t, err)
		require.NotNil(t, shmData)
		require.NotZero(t, shmData.ErrorBoundNS)
	}
	require.Equal(t, int64(0), st.Get()["data_sanity_check_error"])
}

func TestSockDataPointNoGrandmaster(t *testing.T) {
	status, pds, cds := healthyPTP4lReplies()
	status.GMPresent = 0
	dp := sockDataPoint(status, pds, cds)
	dp.FreqAdjustmentPPB = 212131

	require.InDelta(t, 0.0, dp.ClockAccuracyNS, 0)
	require.EqualError(t, dp.SanityCheck(), "clock accuracy is 0")
}
