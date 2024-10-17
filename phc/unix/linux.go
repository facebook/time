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

// bridging to upstream

type Errno = unix.Errno
type RawSockaddrInet4 = unix.RawSockaddrInet4

func Socket(domain, typ, proto int) (fd int, err error)    { return unix.Socket(domain, typ, proto) }
func Close(fd int) (err error)                             { return unix.Close(fd) }
func Syscall(a, b, c, d uintptr) (uintptr, uintptr, Errno) { return unix.Syscall(a, b, c, d) }
func ByteSliceToString(b []byte) string                    { return unix.ByteSliceToString(b) }

const (
	AF_INET             = unix.AF_INET             //nolint:revive
	EAGAIN              = unix.EAGAIN              //nolint:revive
	EINVAL              = unix.EINVAL              //nolint:revive
	ENOENT              = unix.ENOENT              //nolint:revive
	ETHTOOL_GET_TS_INFO = unix.ETHTOOL_GET_TS_INFO //nolint:revive
	IFNAMSIZ            = unix.IFNAMSIZ            //nolint:revive
	SIOCETHTOOL         = unix.SIOCETHTOOL         //nolint:revive
	SIOCGHWTSTAMP       = unix.SIOCGHWTSTAMP       //nolint:revive
	SizeofPtr           = unix.SizeofPtr
	SizeofSockaddrInet4 = unix.SizeofSockaddrInet4
	SOCK_DGRAM          = unix.SOCK_DGRAM //nolint:revive
	SYS_IOCTL           = unix.SYS_IOCTL  //nolint:revive
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
