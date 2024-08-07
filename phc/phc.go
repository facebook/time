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

// DefaultMaxClockFreqPPB value came from linuxptp project (clockadj.c)
const DefaultMaxClockFreqPPB = 500000.0

// PTPClockTime as defined in linux/ptp_clock.h
type PTPClockTime struct {
	Sec      int64  /* seconds */
	NSec     uint32 /* nanoseconds */
	Reserved uint32
}

// Time returns PTPClockTime as time.Time
func (t PTPClockTime) Time() time.Time {
	return time.Unix(t.Sec, int64(t.NSec))
}

// TimeMethod is method we use to get time
type TimeMethod string

// Methods we support to get time
const (
	MethodSyscallClockGettime    TimeMethod = "syscall_clock_gettime"
	MethodIoctlSysOffsetExtended TimeMethod = "ioctl_PTP_SYS_OFFSET_EXTENDED"
)

// SupportedMethods is a list of supported TimeMethods
var SupportedMethods = []TimeMethod{MethodSyscallClockGettime, MethodIoctlSysOffsetExtended}

func ifaceInfoToPHCDevice(info *EthtoolTSinfo) (string, error) {
	if info.PHCIndex < 0 {
		return "", fmt.Errorf("interface doesn't support PHC")
	}
	return fmt.Sprintf("/dev/ptp%d", info.PHCIndex), nil
}

// IfaceToPHCDevice returns path to PHC device associated with given network card iface
func IfaceToPHCDevice(iface string) (string, error) {
	info, err := IfaceInfo(iface)
	if err != nil {
		return "", fmt.Errorf("getting interface %s info: %w", iface, err)
	}
	return ifaceInfoToPHCDevice(info)
}

// Time returns time we got from network card
func Time(iface string, method TimeMethod) (time.Time, error) {
	device, err := IfaceToPHCDevice(iface)
	if err != nil {
		return time.Time{}, err
	}

	f, err := os.Open(device)
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()
	dev := FromFile(f)

	switch method {
	case MethodSyscallClockGettime:
		return dev.Time()
	case MethodIoctlSysOffsetExtended:
		extended, err := dev.ReadSysoffExtended1()
		if err != nil {
			return time.Time{}, err
		}
		latest := extended.TS[extended.NSamples-1]
		return latest[1].Time(), nil
	default:
		return time.Time{}, fmt.Errorf("unknown method to get PHC time %q", method)
	}
}

// Device represents a PHC device
type Device os.File

// FromFile returns a *Device corresponding to an *os.File
func FromFile(file *os.File) *Device { return (*Device)(file) }

// File returns the underlying *os.File
func (dev *Device) File() *os.File { return (*os.File)(dev) }

// Fd returns the underlying file descriptor
func (dev *Device) Fd() uintptr { return dev.File().Fd() }

// ClockID derives the clock ID from the file descriptor number - see clock_gettime(3), FD_TO_CLOCKID macros
func (dev *Device) ClockID() int32 { return int32((int(^dev.Fd()) << 3) | 3) }

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

// ReadSysoffExtended1 reads the precise time from the PHC along with SYS time to measure the call delay.
// The nsamples parameter is set to 1.
func (dev *Device) ReadSysoffExtended1() (*PTPSysOffsetExtended, error) {
	return dev.readSysoffExtended(1)
}

func (dev *Device) readSysoffExtended(nsamples int) (*PTPSysOffsetExtended, error) {
	res := &PTPSysOffsetExtended{
		NSamples: uint32(nsamples),
	}
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL, dev.Fd(),
		ioctlPTPSysOffsetExtended,
		uintptr(unsafe.Pointer(res)),
	)
	if errno != 0 {
		return nil, fmt.Errorf("failed PTP_SYS_OFFSET_EXTENDED: %w", errno)
	}
	return res, nil
}

// readCaps reads PTP capabilities using ioctl
func (dev *Device) readCaps() (*PTPClockCaps, error) {
	caps := &PTPClockCaps{}
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL, dev.Fd(),
		ioctlPTPClockGetcaps,
		uintptr(unsafe.Pointer(caps)),
	)
	if errno != 0 {
		return nil, fmt.Errorf("clock didn't respond properly: %w", errno)
	}

	return caps, nil
}

// MaxFreqAdjPPB reads max value for frequency adjustments (in PPB) from ptp device
func (dev *Device) MaxFreqAdjPPB() (maxFreq float64, err error) {
	caps, err := dev.readCaps()
	if err != nil {
		return 0, err
	}
	return caps.maxAdj(), nil
}

// FreqPPB reads PHC device frequency in PPB (parts per billion)
func (dev *Device) FreqPPB() (freqPPB float64, err error) { return freqPPBFromDevice(dev) }

// AdjFreq adjusts the PHC clock frequency in PPB
func (dev *Device) AdjFreq(freqPPB float64) error { return clockAdjFreq(dev, freqPPB) }

// Step steps the PHC clock by given duration
func (dev *Device) Step(step time.Duration) error { return clockStep(dev, step) }
