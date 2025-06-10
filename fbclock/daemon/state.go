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
	"container/ring"
	"math"
	"sync"

	"github.com/facebook/time/fbclock"
	"github.com/facebook/time/ptp/linearizability"
)

// state of the daemon, guarded by mutex
type daemonState struct {
	sync.Mutex

	DataPoints                 *ring.Ring // DataPoints we collected from ptp4l
	mmms                       *ring.Ring // M values we calculated
	linearizabilityTestResults *ring.Ring // linearizability test results

	lastIngressTimeNS int64
	lastStoredData    *fbclock.Data
}

func newDaemonState(ringSize int) *daemonState {
	s := &daemonState{
		DataPoints:                 ring.New(ringSize),
		mmms:                       ring.New(ringSize),
		linearizabilityTestResults: ring.New(ringSize),
	}
	// init ring buffers with nils
	for i := 0; i < ringSize; i++ {
		s.DataPoints.Value = nil
		s.DataPoints = s.DataPoints.Next()

		s.mmms.Value = nil
		s.mmms = s.mmms.Next()

		s.linearizabilityTestResults.Value = nil
		s.linearizabilityTestResults = s.linearizabilityTestResults.Next()
	}
	return s
}

func (s *daemonState) updateIngressTimeNS(it int64) {
	s.Lock()
	defer s.Unlock()
	s.lastIngressTimeNS = it
}

func (s *daemonState) ingressTimeNS() int64 {
	s.Lock()
	defer s.Unlock()
	return s.lastIngressTimeNS
}

func (s *daemonState) pushDataPoint(data *DataPoint) {
	s.Lock()
	defer s.Unlock()
	s.DataPoints.Value = data
	s.DataPoints = s.DataPoints.Next()
}

func (s *daemonState) takeDataPoint(n int) []*DataPoint {
	s.Lock()
	defer s.Unlock()
	result := []*DataPoint{}
	r := s.DataPoints.Prev()
	for j := 0; j < n; j++ {
		if r.Value == nil {
			continue
		}
		result = append(result, r.Value.(*DataPoint))
		r = r.Prev()
	}
	return result
}

func (s *daemonState) aggregateDataPointsMax(n int) *DataPoint {
	s.Lock()
	defer s.Unlock()
	d := &DataPoint{}
	r := s.DataPoints.Prev()
	for j := 0; j < n; j++ {
		if r.Value == nil {
			continue
		}
		dp := r.Value.(*DataPoint)
		if math.Abs(dp.MasterOffsetNS) > d.MasterOffsetNS {
			d.MasterOffsetNS = math.Abs(dp.MasterOffsetNS)
		}
		if math.Abs(dp.PathDelayNS) > d.PathDelayNS {
			d.PathDelayNS = math.Abs(dp.PathDelayNS)
		}
		if math.Abs(dp.FreqAdjustmentPPB) > d.FreqAdjustmentPPB {
			d.FreqAdjustmentPPB = math.Abs(dp.FreqAdjustmentPPB)
		}
		r = r.Prev()
	}
	return d
}

func (s *daemonState) pushM(data float64) {
	s.Lock()
	defer s.Unlock()
	s.mmms.Value = data
	s.mmms = s.mmms.Next()
}

func (s *daemonState) takeM(n int) []float64 {
	s.Lock()
	defer s.Unlock()
	result := []float64{}
	r := s.mmms.Prev()
	for j := 0; j < n; j++ {
		if r.Value == nil {
			continue
		}
		result = append(result, r.Value.(float64))
		r = r.Prev()
	}
	return result
}

func (s *daemonState) pushLinearizabilityTestResult(data linearizability.TestResult) {
	s.Lock()
	defer s.Unlock()
	s.linearizabilityTestResults.Value = data
	s.linearizabilityTestResults = s.linearizabilityTestResults.Next()
}

func (s *daemonState) takeLinearizabilityTestResult(n int) []linearizability.TestResult {
	s.Lock()
	defer s.Unlock()
	result := []linearizability.TestResult{}
	r := s.linearizabilityTestResults.Prev()
	for j := 0; j < n; j++ {
		if r.Value == nil {
			continue
		}
		result = append(result, r.Value.(linearizability.TestResult))
		r = r.Prev()
	}
	return result
}
