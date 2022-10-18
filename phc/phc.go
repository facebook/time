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

// FDToClockID converts file descriptor number to clockID.
// see man(3) clock_gettime, FD_TO_CLOCKID macros
func FDToClockID(fd uintptr) int32 {
	return int32((int(^fd) << 3) | 3)
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

// IfaceToPHCDevice returns path to PHC device associated with given network card iface
func IfaceToPHCDevice(iface string) (string, error) {
	info, err := IfaceInfo(iface)
	if err != nil {
		return "", fmt.Errorf("getting interface info: %w", err)
	}
	if info.PHCIndex < 0 {
		return "", fmt.Errorf("%s doesn't support PHC", iface)
	}
	return fmt.Sprintf("/dev/ptp%d", info.PHCIndex), nil
}

// Time returns time we got from network card
func Time(iface string, method TimeMethod) (time.Time, error) {
	device, err := IfaceToPHCDevice(iface)
	if err != nil {
		return time.Time{}, err
	}
	switch method {
	case MethodSyscallClockGettime:
		return TimeFromDevice(device)
	case MethodIoctlSysOffsetExtended:
		extended, err := ReadPTPSysOffsetExtended(device, 1)
		if err != nil {
			return time.Time{}, err
		}
		latest := extended.TS[extended.NSamples-1]
		return latest[1].Time(), nil
	}
	return time.Time{}, fmt.Errorf("unknown method to get PHC time %q", method)
}

// TimeFromDevice returns time we got from PTP device
func TimeFromDevice(device string) (time.Time, error) {
	f, err := os.Open(device)
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()
	var ts unix.Timespec
	if err := unix.ClockGettime(FDToClockID(f.Fd()), &ts); err != nil {
		return time.Time{}, fmt.Errorf("failed clock_gettime: %w", err)
	}
	return time.Unix(ts.Unix()), nil
}

// ReadPTPSysOffsetExtended gets precise time from PHC along with SYS time to measure the call delay.
func ReadPTPSysOffsetExtended(device string, nsamples int) (*PTPSysOffsetExtended, error) {
	f, err := os.Open(device)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	res := &PTPSysOffsetExtended{
		NSamples: uint32(nsamples),
	}
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL, f.Fd(),
		ioctlPTPSysOffsetExtended,
		uintptr(unsafe.Pointer(res)),
	)
	if errno != 0 {
		return nil, fmt.Errorf("failed PTP_SYS_OFFSET_EXTENDED: %w", errno)
	}
	return res, nil
}

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
	freqPPB = float64(tx.Freq) / 65.536
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
