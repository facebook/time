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

// ReadPTPClockCapsFromDevice reads ptp capabilities using ioctl
func ReadPTPClockCapsFromDevice(phcDevice string) (*PTPClockCaps, error) {
	f, err := os.OpenFile(phcDevice, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("opening device %q to get max frequency: %w", phcDevice, err)
	}
	defer f.Close()

	caps := &PTPClockCaps{}

	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL, f.Fd(),
		ioctlPTPClockGetcaps,
		uintptr(unsafe.Pointer(caps)),
	)
	if errno != 0 {
		return nil, fmt.Errorf("clock didn't respond properly: %w", errno)
	}

	return caps, nil
}

// MaxFreqAdjPPBFromDevice reads max value for frequency adjustments (in PPB) from ptp device
func MaxFreqAdjPPBFromDevice(phcDevice string) (maxFreq float64, err error) {
	caps, err := ReadPTPClockCapsFromDevice(phcDevice)

	if err != nil {
		return 0, err
	}

	return maxAdj(caps), nil
}

func maxAdj(caps *PTPClockCaps) float64 {
	if caps == nil || caps.MaxAdj == 0 {
		return DefaultMaxClockFreqPPB
	}
	return float64(caps.MaxAdj)
}
