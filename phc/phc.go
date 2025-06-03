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

	"github.com/facebook/time/phc/unix" // a temporary shim for "golang.org/x/sys/unix" until v0.27.0 is cut
)

// DefaultMaxClockFreqPPB value came from linuxptp project (clockadj.c)
const DefaultMaxClockFreqPPB = 500000.0

// TimeMethod is method we use to get time
type TimeMethod string

// Methods we support to get time
const (
	MethodSyscallClockGettime                 TimeMethod = "syscall_clock_gettime"
	MethodIoctlSysOffsetExtended              TimeMethod = "ioctl_PTP_SYS_OFFSET_EXTENDED"
	MethodIoctlSysOffsetPrecise               TimeMethod = "ioctl_PTP_SYS_OFFSET_PRECISE"
	MethodIoctlSysOffsetExtendedRealTimeClock TimeMethod = "ioctl_PTP_SYS_OFFSET_EXTENDED_REALTIMECLOCK"
)

type (
	// PtpPeroutRequest is an alias
	PtpPeroutRequest = unix.PtpPeroutRequest
	// PtpExttsRequest is an alias
	PtpExttsRequest = unix.PtpExttsRequest
	// PtpExttsEvent is an alias
	PtpExttsEvent = unix.PtpExttsEvent
	// PtpClockTime is an alias
	PtpClockTime = unix.PtpClockTime
	// PtpClockCaps is an alias
	PtpClockCaps = unix.PtpClockCaps
)

// IfaceToPHCDevice returns path to PHC device associated with given network card iface
func IfaceToPHCDevice(iface string) (string, error) {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
	if err != nil {
		return "", fmt.Errorf("failed to create socket for ioctl: %w", err)
	}
	defer unix.Close(fd)
	info, err := unix.IoctlGetEthtoolTsInfo(fd, iface)
	if err != nil {
		return "", fmt.Errorf("getting interface %s info: %w", iface, err)
	}
	if info.Phc_index < 0 {
		return "", fmt.Errorf("%s: no PHC support", iface)
	}
	return fmt.Sprintf("/dev/ptp%d", info.Phc_index), nil
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
		latest := extended.Ts[extended.Samples-1]
		tp := latest[1]
		return time.Unix(tp.Sec, int64(tp.Nsec)), nil
	case MethodIoctlSysOffsetExtendedRealTimeClock:
		extended, err := dev.ReadSysoffExtendedRealTimeClock1()
		if err != nil {
			return time.Time{}, err
		}
		latest := extended.Ts[extended.Samples-1]
		tp := latest[1]
		return time.Unix(tp.Sec, int64(tp.Nsec)), nil
	case MethodIoctlSysOffsetPrecise:
		precise, err := dev.ReadSysoffPrecise()
		if err != nil {
			return time.Time{}, err
		}
		tp := precise.Device
		return time.Unix(tp.Sec, int64(tp.Nsec)), nil
	default:
		return time.Time{}, fmt.Errorf("unknown method to get PHC time %q", method)
	}
}
