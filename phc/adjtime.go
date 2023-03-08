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
func FrequencyPPBFromDevice(phcDevice string) (freqPPB float64, err error) {
	// we need RW permissions to issue CLOCK_ADJTIME on the device, even with empty struct
	f, err := os.OpenFile(phcDevice, os.O_RDWR, 0)
	if err != nil {
		return freqPPB, fmt.Errorf("opening device %q to read frequency: %w", phcDevice, err)
	}
	defer f.Close()
	var state int
	freqPPB, state, err = clock.FrequencyPPB(FDToClockID(f.Fd()))
	if err == nil && state != unix.TIME_OK {
		return freqPPB, fmt.Errorf("clock %q state %d is not TIME_OK", phcDevice, state)
	}
	return freqPPB, err
}

// FrequencyPPB reads network card PHC device frequency in PPB
func FrequencyPPB(iface string) (float64, error) {
	device, err := IfaceToPHCDevice(iface)
	if err != nil {
		return 0.0, err
	}
	return FrequencyPPBFromDevice(device)
}

// ClockAdjFreq adjusts PHC clock frequency in PPB
func ClockAdjFreq(phcDevice string, freqPPB float64) error {
	// we need RW permissions to issue CLOCK_ADJTIME on the device, even with empty struct
	f, err := os.OpenFile(phcDevice, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("opening device %q to set frequency: %w", phcDevice, err)
	}
	defer f.Close()
	state, err := clock.AdjFreqPPB(FDToClockID(f.Fd()), freqPPB)
	if err == nil && state != unix.TIME_OK {
		return fmt.Errorf("clock %q state %d is not TIME_OK", phcDevice, state)
	}
	return err
}

// ClockStep steps PHC clock by given step
func ClockStep(phcDevice string, step time.Duration) error {
	// we need RW permissions to issue CLOCK_ADJTIME on the device, even with empty struct
	f, err := os.OpenFile(phcDevice, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("opening device %q to set frequency: %w", phcDevice, err)
	}
	defer f.Close()
	state, err := clock.Step(FDToClockID(f.Fd()), step)
	if err == nil && state != unix.TIME_OK {
		return fmt.Errorf("clock %q state %d is not TIME_OK", phcDevice, state)
	}
	return err
}
