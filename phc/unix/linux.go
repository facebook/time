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
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// https://go-review.googlesource.com/c/sys/+/620376

// HwTstampConfig is used in SIOCGHWTSTAMP and SIOCSHWTSTAMP ioctls
type HwTstampConfig struct {
	Flags     int32 //nolint:revive
	Tx_type   int32 //nolint:revive
	Rx_filter int32 //nolint:revive
}

// IoctlGetHwTstamp retrieves the hardware timestamping configuration
// for the network device specified by ifname.
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

// IoctlSetHwTstamp updates the hardware timestamping configuration for
// the network device specified by ifname.
func IoctlSetHwTstamp(fd int, ifname string, cfg *HwTstampConfig) error {
	ifr, err := NewIfreq(ifname)
	if err != nil {
		return err
	}
	ifrd := ifr.withData(unsafe.Pointer(cfg))
	return ioctlIfreqData(fd, SIOCSHWTSTAMP, &ifrd)
}

const (
	HWTSTAMP_FILTER_NONE            = 0x0 //nolint:revive
	HWTSTAMP_FILTER_ALL             = 0x1 //nolint:revive
	HWTSTAMP_FILTER_SOME            = 0x2 //nolint:revive
	HWTSTAMP_FILTER_PTP_V1_L4_EVENT = 0x3 //nolint:revive
	HWTSTAMP_FILTER_PTP_V2_L4_EVENT = 0x6 //nolint:revive
	HWTSTAMP_FILTER_PTP_V2_L2_EVENT = 0x9 //nolint:revive
	HWTSTAMP_FILTER_PTP_V2_EVENT    = 0xc //nolint:revive
)

const (
	HWTSTAMP_TX_OFF          = 0x0 //nolint:revive
	HWTSTAMP_TX_ON           = 0x1 //nolint:revive
	HWTSTAMP_TX_ONESTEP_SYNC = 0x2 //nolint:revive
)

// https://go-review.googlesource.com/c/sys/+/619335

// EthtoolTsInfo a struct returned by ETHTOOL_GET_TS_INFO function of
// SIOCETHTOOL ioctl.
type EthtoolTsInfo struct {
	Cmd             uint32
	So_timestamping uint32    //nolint:revive
	Phc_index       int32     //nolint:revive
	Tx_types        uint32    //nolint:revive
	Tx_reserved     [3]uint32 //nolint:revive
	Rx_filters      uint32    //nolint:revive
	Rx_reserved     [3]uint32 //nolint:revive
}

// IoctlGetEthtoolTsInfo fetches ethtool timestamping and PHC
// association for the network device specified by ifname.
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

// https://go-review.googlesource.com/c/sys/+/619255

// ClockSettime calls the CLOCK_SETTIME syscall
func ClockSettime(clockid int32, time *Timespec) (err error) {
	_, _, e1 := Syscall(SYS_CLOCK_SETTIME, uintptr(clockid), uintptr(unsafe.Pointer(time)), 0)
	if e1 != 0 {
		err = errnoErr(e1)
	}
	return
}

// https://go-review.googlesource.com/c/sys/+/621375

type (
	PtpClockCaps struct {
		Max_adj            int32 //nolint:revive
		N_alarm            int32 //nolint:revive
		N_ext_ts           int32 //nolint:revive
		N_per_out          int32 //nolint:revive
		Pps                int32
		N_pins             int32 //nolint:revive
		Cross_timestamping int32 //nolint:revive
		Adjust_phase       int32 //nolint:revive
		Max_phase_adj      int32 //nolint:revive
		Rsv                [11]int32
	}
	PtpClockTime struct {
		Sec      int64
		Nsec     uint32
		Reserved uint32
	}
	PtpExttsEvent struct {
		T     PtpClockTime
		Index uint32
		Flags uint32
		Rsv   [2]uint32
	}
	PtpExttsRequest struct {
		Index uint32
		Flags uint32
		Rsv   [2]uint32
	}
	PtpPeroutRequest struct {
		StartOrPhase PtpClockTime
		Period       PtpClockTime
		Index        uint32
		Flags        uint32
		On           PtpClockTime
	}
	PtpPinDesc struct {
		Name  [64]byte
		Index uint32
		Func  uint32
		Chan  uint32
		Rsv   [5]uint32
	}
	PtpSysOffset struct {
		Samples uint32
		Rsv     [3]uint32
		Ts      [51]PtpClockTime
	}
	PtpSysOffsetExtended struct {
		Samples uint32
		ClockID uint32
		Rsv     [2]uint32
		Ts      [25][3]PtpClockTime
	}
	PtpSysOffsetPrecise struct {
		Device   PtpClockTime
		Realtime PtpClockTime
		Monoraw  PtpClockTime
		Rsv      [4]uint32
	}
)

const (
	PTP_CLK_MAGIC             = '='        //nolint:revive
	PTP_ENABLE_FEATURE        = 0x1        //nolint:revive
	PTP_EXTTS_EDGES           = 0x6        //nolint:revive
	PTP_EXTTS_EVENT_VALID     = 0x1        //nolint:revive
	PTP_EXTTS_V1_VALID_FLAGS  = 0x7        //nolint:revive
	PTP_EXTTS_VALID_FLAGS     = 0x1f       //nolint:revive
	PTP_EXT_OFFSET            = 0x10       //nolint:revive
	PTP_FALLING_EDGE          = 0x4        //nolint:revive
	PTP_MAX_SAMPLES           = 0x19       //nolint:revive
	PTP_PEROUT_DUTY_CYCLE     = 0x2        //nolint:revive
	PTP_PEROUT_ONE_SHOT       = 0x1        //nolint:revive
	PTP_PEROUT_PHASE          = 0x4        //nolint:revive
	PTP_PEROUT_V1_VALID_FLAGS = 0x0        //nolint:revive
	PTP_PEROUT_VALID_FLAGS    = 0x7        //nolint:revive
	PTP_PIN_GETFUNC           = 0xc0603d06 //nolint:revive
	PTP_PIN_GETFUNC2          = 0xc0603d0f //nolint:revive
	PTP_RISING_EDGE           = 0x2        //nolint:revive
	PTP_STRICT_FLAGS          = 0x8        //nolint:revive
	PTP_SYS_OFFSET_EXTENDED   = 0xc4c03d09 //nolint:revive
	PTP_SYS_OFFSET_EXTENDED2  = 0xc4c03d12 //nolint:revive
	PTP_SYS_OFFSET_PRECISE    = 0xc0403d08 //nolint:revive
	PTP_SYS_OFFSET_PRECISE2   = 0xc0403d11 //nolint:revive
)

// FdToClockID derives the clock ID from the file descriptor number
// - see clock_gettime(3), FD_TO_CLOCKID macros. The resulting ID is
// suitable for system calls like ClockGettime.
func FdToClockID(fd int) int32 { return int32(((^fd) << 3) | 3) }

// IoctlPtpClockGetcaps returns the description of a given PTP device.
func IoctlPtpClockGetcaps(fd int) (*PtpClockCaps, error) {
	var value PtpClockCaps
	err := ioctlPtr(fd, PTP_CLOCK_GETCAPS2, unsafe.Pointer(&value))
	return &value, err
}

// IoctlPtpSysOffsetPrecise returns a description of the clock
// offset compared to the system clock.
func IoctlPtpSysOffsetPrecise(fd int) (*PtpSysOffsetPrecise, error) {
	var value PtpSysOffsetPrecise
	err := ioctlPtr(fd, PTP_SYS_OFFSET_PRECISE2, unsafe.Pointer(&value))
	return &value, err
}

// IoctlPtpSysOffsetExtended returns an extended description of the
// clock offset compared to the system clock. The samples parameter
// specifies the desired number of measurements.
func IoctlPtpSysOffsetExtended(fd int, samples uint) (*PtpSysOffsetExtended, error) {
	return IoctlPtpSysOffsetExtendedClock(fd, unix.CLOCK_REALTIME, samples)
}

// IoctlPtpSysOffsetExtendedClock returns an extended description of the
// clock offset compared to the system clock. The samples parameter
// specifies the desired number of measurements.
func IoctlPtpSysOffsetExtendedClock(fd int, clockid uint32, samples uint) (*PtpSysOffsetExtended, error) {
	value := PtpSysOffsetExtended{Samples: uint32(samples), ClockID: clockid, Rsv: [2]uint32{0, 0}}
	err := ioctlPtr(fd, PTP_SYS_OFFSET_EXTENDED2, unsafe.Pointer(&value))
	return &value, err
}

// IoctlPtpPinGetfunc returns the configuration of the specified
// I/O pin on given PTP device.
func IoctlPtpPinGetfunc(fd int, index uint) (*PtpPinDesc, error) {
	value := PtpPinDesc{Index: uint32(index)}
	err := ioctlPtr(fd, PTP_PIN_GETFUNC2, unsafe.Pointer(&value))
	return &value, err
}

// IoctlPtpPinSetfunc updates configuration of the specified PTP
// I/O pin.
func IoctlPtpPinSetfunc(fd int, pd *PtpPinDesc) error {
	return ioctlPtr(fd, PTP_PIN_SETFUNC2, unsafe.Pointer(pd))
}

// IoctlPtpPeroutRequest configures the periodic output mode of the
// PTP I/O pins.
func IoctlPtpPeroutRequest(fd int, r *PtpPeroutRequest) error {
	return ioctlPtr(fd, PTP_PEROUT_REQUEST2, unsafe.Pointer(r))
}

// IoctlPtpExttsRequest configures the external timestamping mode
// of the PTP I/O pins.
func IoctlPtpExttsRequest(fd int, r *PtpExttsRequest) error {
	return ioctlPtr(fd, PTP_EXTTS_REQUEST2, unsafe.Pointer(r))
}

// https://go-review.googlesource.com/c/sys/+/621735

const (
	PTP_PF_NONE    = iota //nolint:revive
	PTP_PF_EXTTS          //nolint:revive
	PTP_PF_PEROUT         //nolint:revive
	PTP_PF_PHYSYNC        //nolint:revive
)

// bridging to upstream

type Cmsghdr = unix.Cmsghdr
type Errno = unix.Errno
type Msghdr = unix.Msghdr
type PollFd = unix.PollFd
type RawSockaddrInet4 = unix.RawSockaddrInet4
type SockaddrInet4 = unix.SockaddrInet4
type SockaddrInet6 = unix.SockaddrInet6
type Sockaddr = unix.Sockaddr
type Timespec = unix.Timespec
type Timex = unix.Timex
type Utsname = unix.Utsname

func ByteSliceToString(b []byte) string           { return unix.ByteSliceToString(b) }
func ClockAdjtime(c int32, t *Timex) (int, error) { return unix.ClockAdjtime(c, t) }
func ClockGettime(c int32, t *Timespec) error     { return unix.ClockGettime(c, t) }
func Close(fd int) (err error)                    { return unix.Close(fd) }
func ErrnoName(e syscall.Errno) string            { return unix.ErrnoName(e) }
func Poll(f []PollFd, t int) (int, error)         { return unix.Poll(f, t) }
func Recvmsg(a int, b, c []byte, d int) (int, int, int, Sockaddr, error) {
	return unix.Recvmsg(a, b, c, d)
}
func SetsockoptInt(a, b, c, d int) error                   { return unix.SetsockoptInt(a, b, c, d) }
func Socket(domain, typ, proto int) (fd int, err error)    { return unix.Socket(domain, typ, proto) }
func Syscall(a, b, c, d uintptr) (uintptr, uintptr, Errno) { return unix.Syscall(a, b, c, d) }
func TimeToTimespec(t time.Time) (Timespec, error)         { return unix.TimeToTimespec(t) }
func Uname(s *Utsname) error                               { return unix.Uname(s) }

const (
	AF_INET                       = unix.AF_INET             //nolint:revive
	EAGAIN                        = unix.EAGAIN              //nolint:revive
	EINVAL                        = unix.EINVAL              //nolint:revive
	ENOENT                        = unix.ENOENT              //nolint:revive
	ENOTSUP                       = unix.ENOTSUP             //nolint:revive
	ETHTOOL_GET_TS_INFO           = unix.ETHTOOL_GET_TS_INFO //nolint:revive
	IFNAMSIZ                      = unix.IFNAMSIZ            //nolint:revive
	MSG_ERRQUEUE                  = unix.MSG_ERRQUEUE        //nolint:revive
	POLLERR                       = unix.POLLERR             //nolint:revive
	POLLIN                        = unix.POLLIN
	POLLPRI                       = unix.POLLPRI
	SIOCETHTOOL                   = unix.SIOCETHTOOL   //nolint:revive
	SIOCGHWTSTAMP                 = unix.SIOCGHWTSTAMP //nolint:revive
	SIOCSHWTSTAMP                 = unix.SIOCSHWTSTAMP //nolint:revive
	SizeofPtr                     = unix.SizeofPtr
	SizeofSockaddrInet4           = unix.SizeofSockaddrInet4
	SOCK_DGRAM                    = unix.SOCK_DGRAM                    //nolint:revive
	SOF_TIMESTAMPING_OPT_TSONLY   = unix.SOF_TIMESTAMPING_OPT_TSONLY   //nolint:revive
	SOF_TIMESTAMPING_RAW_HARDWARE = unix.SOF_TIMESTAMPING_RAW_HARDWARE //nolint:revive
	SOF_TIMESTAMPING_RX_HARDWARE  = unix.SOF_TIMESTAMPING_RX_HARDWARE  //nolint:revive
	SOF_TIMESTAMPING_RX_SOFTWARE  = unix.SOF_TIMESTAMPING_RX_SOFTWARE  //nolint:revive
	SOF_TIMESTAMPING_SOFTWARE     = unix.SOF_TIMESTAMPING_SOFTWARE     //nolint:revive
	SOF_TIMESTAMPING_TX_HARDWARE  = unix.SOF_TIMESTAMPING_TX_HARDWARE  //nolint:revive
	SOF_TIMESTAMPING_TX_SOFTWARE  = unix.SOF_TIMESTAMPING_TX_SOFTWARE  //nolint:revive
	SOL_SOCKET                    = unix.SOL_SOCKET                    //nolint:revive
	SO_SELECT_ERR_QUEUE           = unix.SO_SELECT_ERR_QUEUE           //nolint:revive
	SO_TIMESTAMPING_NEW           = unix.SO_TIMESTAMPING_NEW           //nolint:revive
	SO_TIMESTAMPING               = unix.SO_TIMESTAMPING               //nolint:revive
	SYS_CLOCK_SETTIME             = unix.SYS_CLOCK_SETTIME             //nolint:revive
	SYS_IOCTL                     = unix.SYS_IOCTL                     //nolint:revive
	SYS_RECVMSG                   = unix.SYS_RECVMSG                   //nolint:revive
	TIME_OK                       = unix.TIME_OK                       //nolint:revive
	CLOCK_REALTIME                = unix.CLOCK_REALTIME                //nolint:revive
	CLOCK_MONOTONIC_RAW           = unix.CLOCK_MONOTONIC_RAW           //nolint:revive
)

var (
	errEAGAIN error = syscall.EAGAIN
	errEINVAL error = syscall.EINVAL
	errENOENT error = syscall.ENOENT
)

// ioctlIfreqData performs an ioctl using an ifreqData structure for input
// and/or output. See the netdevice(7) man page for details.
func ioctlIfreqData(fd int, req uint, value *ifreqData) error {
	// The memory layout of IfreqData (type-safe) and ifreq (not type-safe) are
	// identical so pass *IfreqData directly.
	return ioctlPtr(fd, req, unsafe.Pointer(value))
}

func ioctlPtr(fd int, req uint, arg unsafe.Pointer) (err error) {
	_, _, e1 := Syscall(SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(arg))
	if e1 != 0 {
		err = errnoErr(e1)
	}
	return
}

// errnoErr returns common boxed Errno values, to prevent
// allocations at runtime.
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
