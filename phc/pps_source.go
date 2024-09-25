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
	"log"
	"time"
)

// PPSSource represents a PPS source
type PPSSource struct {
	PHCDevice   DeviceController
	state       PPSSourceState
	peroutPhase int
}

// PPSSourceState represents the state of a PPS source
type PPSSourceState int

const (
	// UnknownStatus is the initial state of a PPS source, which means PPS may or may not be configured
	UnknownStatus PPSSourceState = iota
	// PPSSet means the underlying device is activated as a PPS source
	PPSSet
)

// PPS related constants
const (
	ptpPeroutDutyCycle   = (1 << 1)
	ptpPeroutPhase       = (1 << 2)
	defaultTs2PhcChannel = 0
	defaultTs2PhcIndex   = 0
	defaultPulseWidth    = uint32(500000000)
	// should default to 0 if config specified. Otherwise -1 (ignore phase)
	defaultPeroutPhase = int32(-1) //nolint:all
	// ppsStartDelay is the delay in seconds before the first PPS signal is sent
	ppsStartDelay = 2
)

// ActivatePPSSource configures the PHC device to be a PPS timestamp source
func ActivatePPSSource(dev DeviceController) (*PPSSource, error) {
	// Initialize the PTPPeroutRequest struct
	peroutRequest := PTPPeroutRequest{}

	err := dev.setPinFunc(defaultTs2PhcIndex, PinFuncPerOut, defaultTs2PhcChannel)

	if err != nil {
		log.Printf("Failed to set PPS Perout on pin index %d, channel %d, PHC %s. Error: %s. Continuing bravely on...",
			defaultTs2PhcIndex, defaultTs2PhcChannel, dev.File().Name(), err)
	}

	ts, err := dev.Time()
	if err != nil {
		return nil, fmt.Errorf("failed (clock_gettime) on %s", dev.File().Name())
	}

	// Set the index and period
	peroutRequest.Index = defaultTs2PhcIndex
	peroutRequest.Period = PTPClockTime{Sec: 1, NSec: 0}

	// Set flags and pulse width
	pulsewidth := defaultPulseWidth

	// TODO: skip this block if pulsewidth > 0 once pulsewidth is configurable
	peroutRequest.Flags |= ptpPeroutDutyCycle
	peroutRequest.On = PTPClockTime{Sec: int64(pulsewidth / nsPerSec), NSec: pulsewidth % nsPerSec}

	// Set phase or start time
	// TODO: reintroduce peroutPhase != -1 condition once peroutPhase is configurable
	peroutRequest.StartOrPhase = PTPClockTime{Sec: int64(ts.Second() + ppsStartDelay), NSec: 0}

	err = dev.setPTPPerout(peroutRequest)

	if err != nil {
		peroutRequest.Flags &^= ptpPeroutDutyCycle
		err = dev.setPTPPerout(peroutRequest)

		if err != nil {
			return nil, fmt.Errorf("error retrying PTP_PEROUT_REQUEST2 with DUTY_CYCLE flag unset for backwards compatibility, %w", err)
		}
	}

	return &PPSSource{PHCDevice: dev, state: PPSSet}, nil
}

// Timestamp returns the timestamp of the last PPS output edge from the given PPS source
// A Pointer is returned to avoid additional memory allocation
func (ppsSource *PPSSource) Timestamp() (*time.Time, error) {
	if ppsSource.state != PPSSet {
		return nil, fmt.Errorf("PPS source not set")
	}

	currTime, err := ppsSource.PHCDevice.Time()

	if err != nil {
		return nil, fmt.Errorf("error getting time (clock_gettime) on %s", ppsSource.PHCDevice.File().Name())
	}

	// subtract device perout phase from current time to get the time of the last perout output edge
	// TODO: optimize section below using binary operations instead of type conversions
	currTime = currTime.Add(-time.Duration(ppsSource.peroutPhase))
	sourceTs := timeToTimespec(currTime)

	/*
	* As long as the kernel doesn't support a proper API for reporting
	* back a precise perout timestamp, we have to assume that the current
	* time on the PPS source is still within +/- half a second of the last
	* perout output edge, and hence, we can deduce the current second
	* (nanossecond is omitted) of this edge at the emitter based on the
	* emitter's current time. We support only PHC sources, so we can ignore
	* the NMEA source edge case described in ts2phc.c
	 */
	//nolint:unconvert
	if int64(sourceTs.Nsec) > int64(nsPerSec/2) {
		sourceTs.Sec++
		sourceTs.Nsec = 0
	}
	//nolint:unconvert
	currTime = time.Unix(int64(sourceTs.Sec), int64(sourceTs.Nsec))
	currTime = currTime.Add(time.Duration(ppsSource.peroutPhase))

	return &currTime, nil
}
