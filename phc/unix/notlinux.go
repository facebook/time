//go:build !linux

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
	"errors"
)

// Timex is a simplified version for non-linux platforms.
// On Linux this is an alias for unix.Timex which has many more fields.
type Timex struct {
	Modes     uint32
	Freq      int64
	Tolerance int64
	Time      Timeval
}

// Linux-only constants that need stub values on Darwin
//
//nolint:revive
const (
	TIME_OK             = 0
	CLOCK_BOOTTIME      = 7
	CLOCK_MONOTONIC_RAW = 4
)

// FdToClockID is not supported on non-linux.
func FdToClockID(_ int) int32 { return 0 }

// ClockAdjtime is not supported on non-linux.
func ClockAdjtime(_ int32, _ *Timex) (int, error) {
	return 0, errors.New("clock adjtime is unsupported on this platform")
}

// Adjtimex is not supported on non-linux.
func Adjtimex(_ *Timex) (int, error) {
	return 0, errors.New("adjtimex is unsupported on this platform")
}

// ClockSettime is not supported on non-linux.
func ClockSettime(_ int32, _ *Timespec) error {
	return errors.New("clock settime is unsupported on this platform")
}

// IoctlPtpSysOffsetExtendedClock is not supported on non-linux.
func IoctlPtpSysOffsetExtendedClock(_ int, _ uint32, _ uint) (*PtpSysOffsetExtended, error) {
	return nil, errors.New("unsupported on this platform")
}

// IoctlPtpSysOffsetPrecise is not supported on non-linux.
func IoctlPtpSysOffsetPrecise(_ int) (*PtpSysOffsetPrecise, error) {
	return nil, errors.New("unsupported on this platform")
}

// IoctlPtpClockGetcaps is not supported on non-linux.
func IoctlPtpClockGetcaps(_ int) (*PtpClockCaps, error) {
	return nil, errors.New("unsupported on this platform")
}

// IoctlPtpPinSetfunc is not supported on non-linux.
func IoctlPtpPinSetfunc(_ int, _ *PtpPinDesc) error {
	return errors.New("unsupported on this platform")
}

// IoctlPtpPeroutRequest is not supported on non-linux.
func IoctlPtpPeroutRequest(_ int, _ *PtpPeroutRequest) error {
	return errors.New("unsupported on this platform")
}

// IoctlPtpExttsRequest is not supported on non-linux.
func IoctlPtpExttsRequest(_ int, _ *PtpExttsRequest) error {
	return errors.New("unsupported on this platform")
}

// IoctlGetEthtoolTsInfo is not supported on non-linux.
func IoctlGetEthtoolTsInfo(_ int, _ string) (*EthtoolTsInfo, error) {
	return nil, errors.New("unsupported on this platform")
}

// IoctlGetHwTstamp is not supported on non-linux.
func IoctlGetHwTstamp(_ int, _ string) (*HwTstampConfig, error) {
	return nil, errors.New("unsupported on this platform")
}

// IoctlPtpPinGetfunc is not supported on non-linux.
func IoctlPtpPinGetfunc(_ int, _ uint) (*PtpPinDesc, error) {
	return nil, errors.New("unsupported on this platform")
}
