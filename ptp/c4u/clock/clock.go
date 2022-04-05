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

package clock

import (
	ptp "github.com/facebook/time/ptp/protocol"
)

const (
	ClockClassLocked       ptp.ClockClass = ptp.ClockClass6
	ClockClassHoldover     ptp.ClockClass = ptp.ClockClass7
	ClockClassCalibrating  ptp.ClockClass = ptp.ClockClass13
	ClockClassUncalibrated ptp.ClockClass = ptp.ClockClass52
)

func worst(clocks []*ptp.ClockQuality) *ptp.ClockQuality {
	w := &ptp.ClockQuality{}
	for _, c := range clocks {
		// Higher value of accuracy means worse
		if c.ClockAccuracy > w.ClockAccuracy {
			w.ClockAccuracy = c.ClockAccuracy
		}

		// Assuming higher class means worse
		if c.ClockClass > w.ClockClass {
			w.ClockClass = c.ClockClass
		}
	}
	return w
}

func Run() (*ptp.ClockQuality, error) {
	oscillatord, err := oscillatord()
	if err != nil {
		return nil, err
	}

	ts2phc, err := ts2phc()
	if err != nil {
		return nil, err
	}

	return worst([]*ptp.ClockQuality{oscillatord, ts2phc}), nil
}
