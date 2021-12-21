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

// file descriptor number to clockID
func fdToClockID(fd uintptr) int32 {
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

// Time returns time we got from network card
func Time(iface string, method TimeMethod) (time.Time, error) {
	info, err := IfaceInfo(iface)
	if err != nil {
		return time.Time{}, fmt.Errorf("getting interface info: %w", err)
	}
	if info.PHCIndex < 0 {
		return time.Time{}, fmt.Errorf("%s doesn't support PHC", iface)
	}
	device := fmt.Sprintf("/dev/ptp%d", info.PHCIndex)
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
	if err := unix.ClockGettime(fdToClockID(f.Fd()), &ts); err != nil {
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
		unix.SYS_IOCTL, uintptr(f.Fd()),
		uintptr(ioctlPTPSysOffsetExtended),
		uintptr(unsafe.Pointer(res)),
	)
	if errno != 0 {
		return nil, fmt.Errorf("failed PTP_SYS_OFFSET_EXTENDED %s (%d)", unix.ErrnoName(errno), errno)
	}
	return res, nil
}
