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

	"golang.org/x/sys/unix"
)

// Type aliases bridging to golang.org/x/sys/unix
type Cmsghdr = unix.Cmsghdr
type Errno = syscall.Errno
type Msghdr = unix.Msghdr
type PollFd = unix.PollFd
type Sockaddr = unix.Sockaddr
type SockaddrInet4 = unix.SockaddrInet4
type SockaddrInet6 = unix.SockaddrInet6
type Timespec = unix.Timespec
type Timeval = unix.Timeval
type Utsname = unix.Utsname

// PTP types

type PtpClockTime struct {
	Sec      int64
	Nsec     uint32
	Reserved uint32
}

type PtpClockCaps struct {
	Max_adj            int32 //nolint:revive // matches C kernel struct field name
	N_alarm            int32 //nolint:revive // matches C kernel struct field name
	N_ext_ts           int32 //nolint:revive // matches C kernel struct field name
	N_per_out          int32 //nolint:revive // matches C kernel struct field name
	Pps                int32
	N_pins             int32 //nolint:revive // matches C kernel struct field name
	Cross_timestamping int32 //nolint:revive // matches C kernel struct field name
	Adjust_phase       int32 //nolint:revive // matches C kernel struct field name
	Max_phase_adj      int32 //nolint:revive // matches C kernel struct field name
	Rsv                [11]int32
}

type PtpPinDesc struct {
	Name  [64]byte
	Index uint32
	Func  uint32
	Chan  uint32
	Rsv   [5]uint32
}

type PtpSysOffsetPrecise struct {
	Device   PtpClockTime
	Realtime PtpClockTime
	Monoraw  PtpClockTime
	Rsv      [4]uint32
}

type PtpSysOffsetExtended struct {
	Samples uint32
	ClockID uint32
	Rsv     [2]uint32
	Ts      [PTP_MAX_SAMPLES][3]PtpClockTime
}

type PtpPeroutRequest struct {
	StartOrPhase PtpClockTime
	Period       PtpClockTime
	Index        uint32
	Flags        uint32
	On           PtpClockTime
}

type PtpExttsEvent struct {
	T     PtpClockTime
	Index uint32
	Flags uint32
	Rsv   [2]uint32
}

type PtpExttsRequest struct {
	Index uint32
	Flags uint32
	Rsv   [2]uint32
}

// HwTstampConfig is used in SIOCGHWTSTAMP and SIOCSHWTSTAMP ioctls
type HwTstampConfig struct {
	Flags     int32
	Tx_type   int32 //nolint:revive // matches C kernel struct field name
	Rx_filter int32 //nolint:revive // matches C kernel struct field name
}

// EthtoolTsInfo is returned by ETHTOOL_GET_TS_INFO function of SIOCETHTOOL ioctl
type EthtoolTsInfo struct {
	Cmd             uint32
	So_timestamping uint32    //nolint:revive // matches C kernel struct field name
	Phc_index       int32     //nolint:revive // matches C kernel struct field name
	Tx_types        uint32    //nolint:revive // matches C kernel struct field name
	Tx_reserved     [3]uint32 //nolint:revive // matches C kernel struct field name
	Rx_filters      uint32    //nolint:revive // matches C kernel struct field name
	Rx_reserved     [3]uint32 //nolint:revive // matches C kernel struct field name
}

// Constants available on all platforms

//nolint:revive // matches C kernel constant names
const (
	PTP_PF_NONE = iota
	PTP_PF_EXTTS
	PTP_PF_PEROUT
	PTP_PF_PHYSYNC
)

//nolint:revive // matches C kernel constant names
const (
	PTP_MAX_SAMPLES    = 25
	PTP_ENABLE_FEATURE = 0x1
	PTP_RISING_EDGE    = 0x2
)

// Function wrappers bridging to golang.org/x/sys/unix

func ByteSliceToString(b []byte) string                    { return unix.ByteSliceToString(b) }
func ClockGettime(c int32, t *Timespec) error              { return unix.ClockGettime(c, t) }
func Close(fd int) (err error)                             { return unix.Close(fd) }
func ErrnoName(e syscall.Errno) string                     { return unix.ErrnoName(e) }
func Poll(f []PollFd, t int) (int, error)                  { return unix.Poll(f, t) }
func SetsockoptInt(a, b, c, d int) error                   { return unix.SetsockoptInt(a, b, c, d) }
func Socket(domain, typ, proto int) (fd int, err error)    { return unix.Socket(domain, typ, proto) }
func Syscall(a, b, c, d uintptr) (uintptr, uintptr, Errno) { return unix.Syscall(a, b, c, d) }
func TimeToTimespec(t time.Time) (Timespec, error)         { return unix.TimeToTimespec(t) }
func Uname(s *Utsname) error                               { return unix.Uname(s) }
func NsecToTimeval(nsec int64) Timeval                     { return unix.NsecToTimeval(nsec) }
func SetsockoptTimeval(fd, level, opt int, tv *Timeval) error {
	return unix.SetsockoptTimeval(fd, level, opt, tv)
}

func Recvmsg(a int, b, c []byte, d int) (int, int, int, Sockaddr, error) {
	return unix.Recvmsg(a, b, c, d)
}

// Cross-platform constants from golang.org/x/sys/unix

const (
	AF_INET             = unix.AF_INET //nolint:revive // matches C kernel constant names
	EAGAIN              = unix.EAGAIN
	EINVAL              = unix.EINVAL
	ENOENT              = unix.ENOENT
	ENOTSUP             = unix.ENOTSUP
	IFNAMSIZ            = unix.IFNAMSIZ
	POLLERR             = unix.POLLERR
	POLLIN              = unix.POLLIN
	POLLPRI             = unix.POLLPRI
	SizeofPtr           = unix.SizeofPtr
	SizeofSockaddrInet4 = unix.SizeofSockaddrInet4
	SOCK_DGRAM          = unix.SOCK_DGRAM     //nolint:revive // matches C kernel constant names
	SOL_SOCKET          = unix.SOL_SOCKET     //nolint:revive // matches C kernel constant names
	SO_RCVTIMEO         = unix.SO_RCVTIMEO    //nolint:revive // matches C kernel constant names
	SO_TIMESTAMP        = unix.SO_TIMESTAMP   //nolint:revive // matches C kernel constant names
	CLOCK_REALTIME      = unix.CLOCK_REALTIME //nolint:revive // matches C kernel constant names
)
