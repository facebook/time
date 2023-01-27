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
	"unsafe"

	"golang.org/x/sys/unix"
)

// man clock_adjtime(2):
// In struct timex, freq, ppsfreq, and stabil are ppm (parts per million) with a 16-bit fractional part.
// To covert value where 2^16=65536 is 1 ppm to ppb or back, we need this multiplier
const ppbToTimexPPM = 65.536

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

// ClockAdjtime issues CLOCK_ADJTIME syscall to either adjust the parameters of given clock,
// or read them if buf is empty.  man(2) clock_adjtime
func ClockAdjtime(clockid int32, buf *unix.Timex) (state int, err error) {
	r0, _, errno := unix.Syscall(unix.SYS_CLOCK_ADJTIME, uintptr(clockid), uintptr(unsafe.Pointer(buf)), 0)
	state = int(r0)
	if errno != 0 {
		err = errno
	}
	return state, err
}

// FrequencyPPBFromDevice reads PHC device frequency in PPB
func FrequencyPPBFromDevice(device string) (freqPPB float64, err error) {
	// we need RW permissions to issue CLOCK_ADJTIME on the device, even with empty struct
	f, err := os.OpenFile(device, os.O_RDWR, 0)
	if err != nil {
		return freqPPB, fmt.Errorf("opening device %q to read frequency: %w", device, err)
	}
	defer f.Close()
	tx := &unix.Timex{}
	state, err := ClockAdjtime(FDToClockID(f.Fd()), tx)
	// man(2) clock_adjtime
	freqPPB = float64(tx.Freq) / ppbToTimexPPM
	if err == nil && state != unix.TIME_OK {
		return freqPPB, fmt.Errorf("clock %q state %d is not TIME_OK", device, state)
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
	tx := &unix.Timex{}
	// man(2) clock_adjtime, turn ppb to ppm
	tx.Freq = int64(freqPPB * ppbToTimexPPM)
	tx.Modes = AdjFrequency
	state, err := ClockAdjtime(FDToClockID(f.Fd()), tx)

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
	sign := 1
	if step < 0 {
		sign = -1
		step = step * -1
	}
	tx := &unix.Timex{}
	tx.Modes = AdjSetOffset | AdjNano
	tx.Time.Sec = int64(float64(sign) * (float64(step) / float64(time.Second)))
	tx.Time.Usec = int64(time.Duration(sign) * (step % time.Second))
	/*
	 * The value of a timeval is the sum of its fields, but the
	 * field tv_usec must always be non-negative.
	 */
	if tx.Time.Usec < 0 {
		tx.Time.Sec--
		tx.Time.Usec += 1000000000
	}
	state, err := ClockAdjtime(FDToClockID(f.Fd()), tx)

	if err == nil && state != unix.TIME_OK {
		return fmt.Errorf("clock %q state %d is not TIME_OK", phcDevice, state)
	}
	return err
}
