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

package phc

import (
	"fmt"
	"time"

	"github.com/facebook/time/clock"
	"golang.org/x/sys/unix"
)

// errorClockState is an error type returned in a situation when the clock is found in bad state
type errorClockState struct {
	path string
	st   int
}

// Error implements the error interface
func (e *errorClockState) Error() string {
	return fmt.Sprintf("clock %q state %d is not TIME_OK", e.path, e.st)
}

// freqPPBFromDevice reads PHC device frequency in PPB
func freqPPBFromDevice(dev *Device) (freqPPB float64, err error) {
	freqPPB, state, err := clock.FrequencyPPB(dev.ClockID())
	if err == nil && state != unix.TIME_OK {
		return freqPPB, &errorClockState{path: dev.File().Name(), st: state}
	}
	return freqPPB, err
}

// clockAdjFreq adjusts PHC clock frequency in PPB
func clockAdjFreq(dev *Device, freqPPB float64) error {
	state, err := clock.AdjFreqPPB(dev.ClockID(), freqPPB)
	if err == nil && state != unix.TIME_OK {
		return &errorClockState{path: dev.File().Name(), st: state}
	}
	return err
}

// clockStep steps PHC clock by given step
func clockStep(dev *Device, step time.Duration) error {
	state, err := clock.Step(dev.ClockID(), step)
	if err == nil && state != unix.TIME_OK {
		return &errorClockState{path: dev.File().Name(), st: state}
	}
	return err
}

func clockSetTime(dev *Device, t time.Time) error {
	ts, err := unix.TimeToTimespec(t)
	if err != nil {
		return err
	}
	return clock.Settime(dev.ClockID(), &ts)
}
