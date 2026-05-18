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

	"github.com/facebook/time/ptp/linearizability"
	"github.com/stretchr/testify/require"
)

func TestIngressTimeNS(t *testing.T) {
	s := newDaemonState(3)
	require.Equal(t, int64(0), s.ingressTimeNS())

	s.updateIngressTimeNS(123456789)
	require.Equal(t, int64(123456789), s.ingressTimeNS())

	s.updateIngressTimeNS(987654321)
	require.Equal(t, int64(987654321), s.ingressTimeNS())
}

func TestPushAndTakeDataPoint(t *testing.T) {
	s := newDaemonState(3)

	dp1 := &DataPoint{MasterOffsetNS: 1.0, PathDelayNS: 10.0, FreqAdjustmentPPB: 100.0}
	dp2 := &DataPoint{MasterOffsetNS: 2.0, PathDelayNS: 20.0, FreqAdjustmentPPB: 200.0}
	dp3 := &DataPoint{MasterOffsetNS: 3.0, PathDelayNS: 30.0, FreqAdjustmentPPB: 300.0}

	s.pushDataPoint(dp1)
	s.pushDataPoint(dp2)
	s.pushDataPoint(dp3)

	got := s.takeDataPoint(3)
	require.Len(t, got, 3)
	require.Equal(t, dp3, got[0])
	require.Equal(t, dp2, got[1])
	require.Equal(t, dp1, got[2])
}

func TestTakeDataPointPartial(t *testing.T) {
	s := newDaemonState(5)

	dp1 := &DataPoint{MasterOffsetNS: 1.0}
	dp2 := &DataPoint{MasterOffsetNS: 2.0}
	s.pushDataPoint(dp1)
	s.pushDataPoint(dp2)

	got := s.takeDataPoint(5)
	require.Len(t, got, 2)
}

func TestTakeDataPointOverflow(t *testing.T) {
	s := newDaemonState(3)

	dp1 := &DataPoint{MasterOffsetNS: 1.0}
	dp2 := &DataPoint{MasterOffsetNS: 2.0}
	dp3 := &DataPoint{MasterOffsetNS: 3.0}
	dp4 := &DataPoint{MasterOffsetNS: 4.0}

	s.pushDataPoint(dp1)
	s.pushDataPoint(dp2)
	s.pushDataPoint(dp3)
	s.pushDataPoint(dp4)

	got := s.takeDataPoint(3)
	require.Len(t, got, 3)
	require.Equal(t, dp4, got[0])
	require.Equal(t, dp3, got[1])
	require.Equal(t, dp2, got[2])
}

func TestPushAndTakeM(t *testing.T) {
	s := newDaemonState(3)

	s.pushM(1.5)
	s.pushM(2.5)
	s.pushM(3.5)

	got := s.takeM(3)
	require.Len(t, got, 3)
	require.Equal(t, 3.5, got[0])
	require.Equal(t, 2.5, got[1])
	require.Equal(t, 1.5, got[2])
}

func TestTakeMPartial(t *testing.T) {
	s := newDaemonState(5)
	s.pushM(10.0)
	s.pushM(20.0)

	got := s.takeM(5)
	require.Len(t, got, 2)
}

func TestTakeMOverflow(t *testing.T) {
	s := newDaemonState(3)
	s.pushM(1.0)
	s.pushM(2.0)
	s.pushM(3.0)
	s.pushM(4.0)

	got := s.takeM(3)
	require.Len(t, got, 3)
	require.Equal(t, 4.0, got[0])
	require.Equal(t, 3.0, got[1])
	require.Equal(t, 2.0, got[2])
}

func TestPushAndTakeLinearizabilityTestResult(t *testing.T) {
	s := newDaemonState(3)

	r1 := linearizability.PTPTestResult{
		Server:      "server01",
		TXTimestamp: time.Unix(0, 100),
		RXTimestamp: time.Unix(0, 200),
	}
	r2 := linearizability.PTPTestResult{
		Server:      "server02",
		TXTimestamp: time.Unix(0, 300),
		RXTimestamp: time.Unix(0, 400),
	}

	s.pushLinearizabilityTestResult(r1)
	s.pushLinearizabilityTestResult(r2)

	got := s.takeLinearizabilityTestResult(3)
	require.Len(t, got, 2)
	require.Contains(t, got, linearizability.TestResult(r1))
	require.Contains(t, got, linearizability.TestResult(r2))
}

func TestTakeLinearizabilityTestResultOverflow(t *testing.T) {
	s := newDaemonState(2)

	r1 := linearizability.PTPTestResult{Server: "s1", TXTimestamp: time.Unix(0, 1), RXTimestamp: time.Unix(0, 2)}
	r2 := linearizability.PTPTestResult{Server: "s2", TXTimestamp: time.Unix(0, 3), RXTimestamp: time.Unix(0, 4)}
	r3 := linearizability.PTPTestResult{Server: "s3", TXTimestamp: time.Unix(0, 5), RXTimestamp: time.Unix(0, 6)}

	s.pushLinearizabilityTestResult(r1)
	s.pushLinearizabilityTestResult(r2)
	s.pushLinearizabilityTestResult(r3)

	got := s.takeLinearizabilityTestResult(2)
	require.Len(t, got, 2)
	require.Contains(t, got, linearizability.TestResult(r3))
	require.Contains(t, got, linearizability.TestResult(r2))
}

func TestAggregateDataPointsMaxEmpty(t *testing.T) {
	s := newDaemonState(5)
	got := s.aggregateDataPointsMax(5)
	require.Equal(t, &DataPoint{}, got)
}

func TestAggregateDataPointsMaxSingleEntry(t *testing.T) {
	s := newDaemonState(5)
	dp := &DataPoint{
		MasterOffsetNS:    -500.0,
		PathDelayNS:       -200.0,
		FreqAdjustmentPPB: 300.0,
	}
	s.pushDataPoint(dp)
	got := s.aggregateDataPointsMax(5)
	require.Equal(t, 500.0, got.MasterOffsetNS)
	require.Equal(t, 200.0, got.PathDelayNS)
	require.Equal(t, 300.0, got.FreqAdjustmentPPB)
}

func TestAggregateDataPointsMaxNegativeValues(t *testing.T) {
	s := newDaemonState(5)
	s.pushDataPoint(&DataPoint{MasterOffsetNS: -100, PathDelayNS: -50, FreqAdjustmentPPB: -25})
	s.pushDataPoint(&DataPoint{MasterOffsetNS: 80, PathDelayNS: 40, FreqAdjustmentPPB: 20})
	s.pushDataPoint(&DataPoint{MasterOffsetNS: -200, PathDelayNS: -150, FreqAdjustmentPPB: -300})

	got := s.aggregateDataPointsMax(5)
	require.Equal(t, 200.0, got.MasterOffsetNS)
	require.Equal(t, 150.0, got.PathDelayNS)
	require.Equal(t, 300.0, got.FreqAdjustmentPPB)
}

func TestTakeDataPointEmpty(t *testing.T) {
	s := newDaemonState(5)
	got := s.takeDataPoint(5)
	require.Empty(t, got)
}

func TestTakeMEmpty(t *testing.T) {
	s := newDaemonState(5)
	got := s.takeM(5)
	require.Empty(t, got)
}

func TestTakeLinearizabilityTestResultEmpty(t *testing.T) {
	s := newDaemonState(5)
	got := s.takeLinearizabilityTestResult(5)
	require.Empty(t, got)
}
