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
	"fmt"
	"time"

	"github.com/facebook/time/phc/unix" // a temporary shim for "golang.org/x/sys/unix" until v0.27.0 is cut
)

// PPBToTimexPPM is what we use to conver PPB to PPM.
// man clock_adjtime(2):
// In struct timex, freq, ppsfreq, and stabil are ppm (parts per million) with a 16-bit fractional part.
// To covert value where 2^16=65536 is 1 ppm to ppb or back, we need this multiplier
const PPBToTimexPPM = 65.536

// clock_adjtime modes from usr/include/linux/timex.h
const (
	// time offset
	AdjOffset uint32 = 0x0001
	// frequency offset
	AdjFrequency uint32 = 0x0002
	// maximum time error
	AdjMaxError uint32 = 0x0004
	// estimated time error
	AdjEstError uint32 = 0x0008
	// clock status
	AdjStatus uint32 = 0x0010
	// pll time constant
	AdjTimeConst uint32 = 0x0020
	// set TAI offset
	AdjTAI uint32 = 0x0080
	// add 'time' to current time
	AdjSetOffset uint32 = 0x0100
	// select microsecond resolution
	AdjMicro uint32 = 0x1000
	// select nanosecond resolution
	AdjNano uint32 = 0x2000
	// tick value
	AdjTick uint32 = 0x4000
)

// FrequencyPPB reads device frequency in PPB
func FrequencyPPB(clockid int32) (freqPPB float64, state int, err error) {
	tx := &unix.Timex{}
	state, err = unix.ClockAdjtime(clockid, tx)
	// man(2) clock_adjtime
	freqPPB = float64(tx.Freq) / PPBToTimexPPM
	return freqPPB, state, err
}

// AdjFreqPPB adjusts clock frequency in PPB
func AdjFreqPPB(clockid int32, freqPPB float64) (state int, err error) {
	tx := &unix.Timex{}
	// this way we can have platform-dependent code isolated
	setFreq(tx, freqPPB)
	tx.Modes = AdjFrequency
	return unix.ClockAdjtime(clockid, tx)
}

// Step steps clock by given step
func Step(clockid int32, step time.Duration) (state int, err error) {
	sign := 1
	if step < 0 {
		sign = -1
		step = step * -1
	}
	tx := &unix.Timex{}
	tx.Modes = AdjSetOffset | AdjNano
	sec := time.Duration(float64(sign) * (float64(step) / float64(time.Second)))
	usec := time.Duration(sign) * (step % time.Second)
	// this way we can have platform-dependent code isolated
	setTime(tx, sec, usec)
	/*
	 * The value of a timeval is the sum of its fields, but the
	 * field tv_usec must always be non-negative.
	 */
	if tx.Time.Usec < 0 {
		tx.Time.Sec--
		tx.Time.Usec += 1000000000
	}
	return unix.ClockAdjtime(clockid, tx)
}

// MaxFreqPPB returns maximum frequency adjustment supported by the clock
func MaxFreqPPB(clockid int32) (freqPPB float64, state int, err error) {
	tx := &unix.Timex{}
	state, err = unix.ClockAdjtime(clockid, tx)
	if err != nil {
		return 0.0, state, err
	}
	// man(2) clock_adjtime
	freqPPB = float64(tx.Tolerance) / PPBToTimexPPM
	if freqPPB == 0 {
		freqPPB = 500000
	}
	return freqPPB, state, nil
}

// SetSync sets clock status to TIME_OK
func SetSync(clockid int32) error {
	tx := &unix.Timex{}
	tx.Modes = AdjStatus | AdjMaxError
	state, err := unix.ClockAdjtime(clockid, tx)

	if err == nil && state != unix.TIME_OK {
		return fmt.Errorf("clock state %d is not TIME_OK after setting sync state", state)
	}
	return err
}
