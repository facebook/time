//go:build linux

// @generated
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

package unix

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Linux-only type aliases
type RawSockaddrInet4 = unix.RawSockaddrInet4
type Timex = unix.Timex

// Linux-only function wrappers
func ClockAdjtime(c int32, t *Timex) (int, error) { return unix.ClockAdjtime(c, t) }
func Adjtimex(buf *Timex) (state int, err error)  { return unix.Adjtimex(buf) }

// ClockSettime calls the CLOCK_SETTIME syscall
func ClockSettime(clockid int32, time *Timespec) (err error) {
	_, _, e1 := Syscall(SYS_CLOCK_SETTIME, uintptr(clockid), uintptr(unsafe.Pointer(time)), 0)
	if e1 != 0 {
		err = errnoErr(e1)
	}
	return
}

// FdToClockID derives the clock ID from the file descriptor number
func FdToClockID(fd int) int32 { return int32(((^fd) << 3) | 3) }

// IoctlGetHwTstamp retrieves the hardware timestamping configuration
func IoctlGetHwTstamp(fd int, ifname string) (*HwTstampConfig, error) {
	ifr, err := NewIfreq(ifname)
	if err != nil {
		return nil, err
	}
	value := HwTstampConfig{}
	ifrd := ifr.withData(unsafe.Pointer(&value))
	err = ioctlIfreqData(fd, SIOCGHWTSTAMP, &ifrd)
	return &value, err
}

// IoctlSetHwTstamp updates the hardware timestamping configuration
func IoctlSetHwTstamp(fd int, ifname string, cfg *HwTstampConfig) error {
	ifr, err := NewIfreq(ifname)
	if err != nil {
		return err
	}
	ifrd := ifr.withData(unsafe.Pointer(cfg))
	return ioctlIfreqData(fd, SIOCSHWTSTAMP, &ifrd)
}

// IoctlGetEthtoolTsInfo fetches ethtool timestamping and PHC association
func IoctlGetEthtoolTsInfo(fd int, ifname string) (*EthtoolTsInfo, error) {
	ifr, err := NewIfreq(ifname)
	if err != nil {
		return nil, err
	}
	value := EthtoolTsInfo{Cmd: ETHTOOL_GET_TS_INFO}
	ifrd := ifr.withData(unsafe.Pointer(&value))
	err = ioctlIfreqData(fd, SIOCETHTOOL, &ifrd)
	return &value, err
}

// IoctlPtpClockGetcaps returns the description of a given PTP device.
func IoctlPtpClockGetcaps(fd int) (*PtpClockCaps, error) {
	var value PtpClockCaps
	err := ioctlPtr(fd, PTP_CLOCK_GETCAPS2, unsafe.Pointer(&value))
	return &value, err
}

// IoctlPtpSysOffsetPrecise returns a description of the clock offset compared to the system clock.
func IoctlPtpSysOffsetPrecise(fd int) (*PtpSysOffsetPrecise, error) {
	var value PtpSysOffsetPrecise
	err := ioctlPtr(fd, PTP_SYS_OFFSET_PRECISE2, unsafe.Pointer(&value))
	return &value, err
}

// IoctlPtpSysOffsetExtended returns an extended description of the clock offset.
func IoctlPtpSysOffsetExtended(fd int, samples uint) (*PtpSysOffsetExtended, error) {
	return IoctlPtpSysOffsetExtendedClock(fd, unix.CLOCK_REALTIME, samples)
}

// IoctlPtpSysOffsetExtendedClock returns an extended description of the clock offset.
func IoctlPtpSysOffsetExtendedClock(fd int, clockid uint32, samples uint) (*PtpSysOffsetExtended, error) {
	value := PtpSysOffsetExtended{Samples: uint32(samples), ClockID: clockid, Rsv: [2]uint32{0, 0}}
	err := ioctlPtr(fd, PTP_SYS_OFFSET_EXTENDED2, unsafe.Pointer(&value))
	return &value, err
}

// IoctlPtpPinGetfunc returns the configuration of the specified I/O pin on given PTP device.
func IoctlPtpPinGetfunc(fd int, index uint) (*PtpPinDesc, error) {
	value := PtpPinDesc{Index: uint32(index)}
	err := ioctlPtr(fd, PTP_PIN_GETFUNC2, unsafe.Pointer(&value))
	return &value, err
}

// IoctlPtpPinSetfunc updates configuration of the specified PTP I/O pin.
func IoctlPtpPinSetfunc(fd int, pd *PtpPinDesc) error {
	return ioctlPtr(fd, PTP_PIN_SETFUNC2, unsafe.Pointer(pd))
}

// IoctlPtpPeroutRequest configures the periodic output mode of the PTP I/O pins.
func IoctlPtpPeroutRequest(fd int, r *PtpPeroutRequest) error {
	return ioctlPtr(fd, PTP_PEROUT_REQUEST2, unsafe.Pointer(r))
}

// IoctlPtpExttsRequest configures the external timestamping mode of the PTP I/O pins.
func IoctlPtpExttsRequest(fd int, r *PtpExttsRequest) error {
	return ioctlPtr(fd, PTP_EXTTS_REQUEST2, unsafe.Pointer(r))
}

// Linux-only constants

//nolint:revive
const (
	HWTSTAMP_FILTER_NONE            = 0x0
	HWTSTAMP_FILTER_ALL             = 0x1
	HWTSTAMP_FILTER_SOME            = 0x2
	HWTSTAMP_FILTER_PTP_V1_L4_EVENT = 0x3
	HWTSTAMP_FILTER_PTP_V2_L4_EVENT = 0x6
	HWTSTAMP_FILTER_PTP_V2_L2_EVENT = 0x9
	HWTSTAMP_FILTER_PTP_V2_EVENT    = 0xc
)

//nolint:revive
const (
	HWTSTAMP_TX_OFF          = 0x0
	HWTSTAMP_TX_ON           = 0x1
	HWTSTAMP_TX_ONESTEP_SYNC = 0x2
)

//nolint:revive
const (
	PTP_CLK_MAGIC             = '='
	PTP_EXTTS_EDGES           = 0x6
	PTP_EXTTS_EVENT_VALID     = 0x1
	PTP_EXTTS_V1_VALID_FLAGS  = 0x7
	PTP_EXTTS_VALID_FLAGS     = 0x1f
	PTP_EXT_OFFSET            = 0x10
	PTP_FALLING_EDGE          = 0x4
	PTP_PEROUT_DUTY_CYCLE     = 0x2
	PTP_PEROUT_ONE_SHOT       = 0x1
	PTP_PEROUT_PHASE          = 0x4
	PTP_PEROUT_V1_VALID_FLAGS = 0x0
	PTP_PEROUT_VALID_FLAGS    = 0x7
	PTP_PIN_GETFUNC           = 0xc0603d06
	PTP_PIN_GETFUNC2          = 0xc0603d0f
	PTP_STRICT_FLAGS          = 0x8
	PTP_SYS_OFFSET_EXTENDED   = 0xc4c03d09
	PTP_SYS_OFFSET_EXTENDED2  = 0xc4c03d12
	PTP_SYS_OFFSET_PRECISE    = 0xc0403d08
	PTP_SYS_OFFSET_PRECISE2   = 0xc0403d11
)

const (
	SYS_IOCTL                     = unix.SYS_IOCTL                     //nolint:revive
	SYS_RECVMSG                   = unix.SYS_RECVMSG                   //nolint:revive
	ETHTOOL_GET_TS_INFO           = unix.ETHTOOL_GET_TS_INFO           //nolint:revive
	MSG_ERRQUEUE                  = unix.MSG_ERRQUEUE                  //nolint:revive
	SIOCETHTOOL                   = unix.SIOCETHTOOL                   //nolint:revive
	SIOCGHWTSTAMP                 = unix.SIOCGHWTSTAMP                 //nolint:revive
	SIOCSHWTSTAMP                 = unix.SIOCSHWTSTAMP                 //nolint:revive
	SOF_TIMESTAMPING_OPT_TSONLY   = unix.SOF_TIMESTAMPING_OPT_TSONLY   //nolint:revive
	SOF_TIMESTAMPING_RAW_HARDWARE = unix.SOF_TIMESTAMPING_RAW_HARDWARE //nolint:revive
	SOF_TIMESTAMPING_RX_HARDWARE  = unix.SOF_TIMESTAMPING_RX_HARDWARE  //nolint:revive
	SOF_TIMESTAMPING_RX_SOFTWARE  = unix.SOF_TIMESTAMPING_RX_SOFTWARE  //nolint:revive
	SOF_TIMESTAMPING_SOFTWARE     = unix.SOF_TIMESTAMPING_SOFTWARE     //nolint:revive
	SOF_TIMESTAMPING_TX_HARDWARE  = unix.SOF_TIMESTAMPING_TX_HARDWARE  //nolint:revive
	SOF_TIMESTAMPING_TX_SOFTWARE  = unix.SOF_TIMESTAMPING_TX_SOFTWARE  //nolint:revive
	SO_SELECT_ERR_QUEUE           = unix.SO_SELECT_ERR_QUEUE           //nolint:revive
	SO_TIMESTAMPING_NEW           = unix.SO_TIMESTAMPING_NEW           //nolint:revive
	SO_TIMESTAMPING               = unix.SO_TIMESTAMPING               //nolint:revive
	SYS_CLOCK_SETTIME             = unix.SYS_CLOCK_SETTIME             //nolint:revive
	TIME_OK                       = unix.TIME_OK                       //nolint:revive
	CLOCK_BOOTTIME                = unix.CLOCK_BOOTTIME                //nolint:revive
)

var (
	errEAGAIN error = syscall.EAGAIN
	errEINVAL error = syscall.EINVAL
	errENOENT error = syscall.ENOENT
)

func ioctlIfreqData(fd int, req uint, value *ifreqData) error {
	return ioctlPtr(fd, req, unsafe.Pointer(value))
}

func ioctlPtr(fd int, req uint, arg unsafe.Pointer) (err error) {
	_, _, e1 := Syscall(SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(arg))
	if e1 != 0 {
		err = errnoErr(e1)
	}
	return
}

func errnoErr(e syscall.Errno) error {
	switch e {
	case 0:
		return nil
	case EAGAIN:
		return errEAGAIN
	case EINVAL:
		return errEINVAL
	case ENOENT:
		return errENOENT
	}
	return e
}
