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
	"os"
	"time"

	"github.com/facebook/time/clock"
	"golang.org/x/sys/unix"
)

// FrequencyPPBFromDevice reads PHC device frequency in PPB
func FrequencyPPBFromDevice(phcDevice *os.File) (freqPPB float64, err error) {
	var state int
	freqPPB, state, err = clock.FrequencyPPB(FDToClockID(phcDevice.Fd()))
	if err == nil && state != unix.TIME_OK {
		return freqPPB, fmt.Errorf("clock %q state %d is not TIME_OK", phcDevice.Name(), state)
	}
	return freqPPB, err
}

// ClockAdjFreq adjusts PHC clock frequency in PPB
func ClockAdjFreq(phcDevice *os.File, freqPPB float64) error {
	state, err := clock.AdjFreqPPB(FDToClockID(phcDevice.Fd()), freqPPB)
	if err == nil && state != unix.TIME_OK {
		return fmt.Errorf("clock %q state %d is not TIME_OK", phcDevice.Name(), state)
	}
	return err
}

// ClockStep steps PHC clock by given step
func ClockStep(phcDevice *os.File, step time.Duration) error {
	state, err := clock.Step(FDToClockID(phcDevice.Fd()), step)
	if err == nil && state != unix.TIME_OK {
		return fmt.Errorf("clock %q state %d is not TIME_OK", phcDevice.Name(), state)
	}
	return err
}
