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
	"syscall"
	"time"

	"github.com/facebook/time/phc/unix" // a temporary shim for "golang.org/x/sys/unix" until v0.27.0 is cut
)

// isClockMonoRawSupported is a global variable to keep information whether clockid is supported in PTPSysOffsetExtended
var isClockMonoRawSupported = true

// Device represents a PHC device
type Device os.File

// FromFile returns a *Device corresponding to an *os.File
func FromFile(file *os.File) *Device { return (*Device)(file) }

// File returns the underlying *os.File
func (dev *Device) File() *os.File { return (*os.File)(dev) }

// Fd returns the underlying file descriptor
func (dev *Device) Fd() uintptr { return dev.File().Fd() }

// ClockID derives the clock ID from the file descriptor number -
// see clock_gettime(3), FD_TO_CLOCKID macros
func (dev *Device) ClockID() int32 { return unix.FdToClockID(int(dev.Fd())) }

// Time returns time from the PTP device using the clock_gettime syscall
func (dev *Device) Time() (time.Time, error) {
	var ts unix.Timespec
	if err := unix.ClockGettime(dev.ClockID(), &ts); err != nil {
		return time.Time{}, fmt.Errorf("failed clock_gettime: %w", err)
	}
	return time.Unix(ts.Unix()), nil
}

// ReadSysoffExtended reads the precise time from the PHC along with SYS time to measure the call delay.
// The nsamples parameter is set to ExtendedNumProbes.
func (dev *Device) ReadSysoffExtended() (*PTPSysOffsetExtended, error) {
	return dev.readSysoffExtended(ExtendedNumProbes)
}

// ReadSysoffExtended1 reads the precise time from the PHC along with MONOTONIC_RAW SYS time to measure the call delay.
// The samples parameter is set to 1.
func (dev *Device) ReadSysoffExtended1() (*PTPSysOffsetExtended, error) {
	return dev.readSysoffExtended(1)
}

// ReadSysoffExtendedRealTimeClock1 reads the precise time from the PHC along with REAL_TIME_CLOCK SYS time to measure the call delay.
// The samples parameter is set to 1.
func (dev *Device) ReadSysoffExtendedRealTimeClock1() (*PTPSysOffsetExtended, error) {
	return dev.readSysoffExtendedClockID(1, unix.CLOCK_REALTIME)
}

// ReadSysoffPrecise reads the precise time from the PHC along with SYS time to measure the call delay.
func (dev *Device) ReadSysoffPrecise() (*PTPSysOffsetPrecise, error) {
	return dev.readSysoffPrecise()
}

func (dev *Device) readSysoffExtended(samples uint) (*PTPSysOffsetExtended, error) {
	if isClockMonoRawSupported {
		value, err := dev.readSysoffExtendedClockID(samples, unix.CLOCK_MONOTONIC_RAW)
		if err == nil {
			return value, nil
		}
		isClockMonoRawSupported = false
	}
	return dev.readSysoffExtendedClockID(samples, unix.CLOCK_REALTIME)
}

func (dev *Device) readSysoffExtendedClockID(samples uint, clockid uint32) (*PTPSysOffsetExtended, error) {
	value, err := unix.IoctlPtpSysOffsetExtendedClock(int(dev.Fd()), clockid, samples)
	if err != nil {
		return nil, err
	}
	our := PTPSysOffsetExtended(*value)
	return &our, nil
}

func (dev *Device) readSysoffPrecise() (*PTPSysOffsetPrecise, error) {
	value, err := unix.IoctlPtpSysOffsetPrecise(int(dev.Fd()))
	if err != nil {
		return nil, err
	}
	our := PTPSysOffsetPrecise(*value)
	return &our, nil
}

// readCaps reads PTP capabilities using ioctl
func (dev *Device) readCaps() (*PtpClockCaps, error) {
	return unix.IoctlPtpClockGetcaps(int(dev.Fd()))
}

// setPinFunc sets the function on a single PTP pin descriptor
func (dev *Device) setPinFunc(index uint, pf int, ch uint) error {
	raw := unix.PtpPinDesc{
		Index: uint32(index), //#nosec G115
		Func:  uint32(pf),    //#nosec G115
		Chan:  uint32(ch),    //#nosec G115
	}
	if err := unix.IoctlPtpPinSetfunc(int(dev.Fd()), &raw); err != nil {
		return fmt.Errorf("%s: ioctl(PTP_PIN_SETFUNC) failed: %w", dev.File().Name(), err)
	}
	return nil
}

// MaxFreqAdjPPB reads max value for frequency adjustments (in PPB) from ptp device
func (dev *Device) MaxFreqAdjPPB() (maxFreq float64, err error) {
	caps, err := dev.readCaps()
	if err != nil {
		return 0, err
	}
	return maxAdj(caps), nil
}

func maxAdj(caps *PtpClockCaps) float64 {
	if caps == nil || caps.Max_adj == 0 {
		return DefaultMaxClockFreqPPB
	}
	return float64(caps.Max_adj)
}

func (dev *Device) setPTPPerout(req *PtpPeroutRequest) error {
	return unix.IoctlPtpPeroutRequest(int(dev.Fd()), req)
}

func (dev *Device) extTTSRequest(req *PtpExttsRequest) error {
	return unix.IoctlPtpExttsRequest(int(dev.Fd()), req)
}

// FreqPPB reads PHC device frequency in PPB (parts per billion)
func (dev *Device) FreqPPB() (freqPPB float64, err error) { return freqPPBFromDevice(dev) }

// AdjFreq adjusts the PHC clock frequency in PPB
func (dev *Device) AdjFreq(freqPPB float64) error { return clockAdjFreq(dev, freqPPB) }

// Step steps the PHC clock by given duration
func (dev *Device) Step(step time.Duration) error { return clockStep(dev, step) }

// SetTime sets the time of the PHC clock
func (dev *Device) SetTime(t time.Time) error { return clockSetTime(dev, t) }

func (dev *Device) Read(buffer []byte) (int, error) {
	return syscall.Read(int(dev.Fd()), buffer)
}
