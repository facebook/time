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

package shm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"
	"unsafe"

	"github.com/facebook/time/hostendian"

	"golang.org/x/sys/unix"
)

// SHMKEY is a key of the first NTPD SHM segment
// http://doc.ntp.org/current-stable/drivers/driver28.html
const SHMKEY = 0x4e545030

// IPCCREAT create if key is nonexistent
// https://man7.org/linux/man-pages/man0/sys_ipc.h.0p.html
const IPCCREAT = 00001000

// NTPSHMSize is a size of NTPSHM struct
const NTPSHMSize = 96

// NTPSHM Declaration of the SHM segment from ntp (ntpd/refclock_shm.c)
type NTPSHM struct {
	Mode                 int32
	Count                int32
	ClockTimeStampSec    int64
	ClockTimeStampUSec   int32
	ReceiveTimeStampSec  int64
	ReceiveTimeStampUSec int32
	Leap                 int32
	Precision            int32
	Nsamples             int32
	Valid                int32
	ClockTimeStampNSec   int32
	ReceiveTimeStampNSec int32
	Dummy                [8]int32
}

// Create a segment in SHM and return the ID
func Create() (uintptr, error) {
	shmID, _, errno := unix.Syscall(unix.SYS_SHMGET, uintptr(SHMKEY), 0, uintptr(IPCCREAT|0600))
	if errno != 0 {
		return 0, fmt.Errorf("failed get shm: %s", unix.ErrnoName(errno))
	}
	return shmID, nil
}

func ptrToBytes(shmptr uintptr) []byte {
	// Runtime representation of a slice in Go
	var sl = struct {
		addr uintptr
		len  int
		cap  int
	}{shmptr, NTPSHMSize, NTPSHMSize}

	return *(*[]byte)(unsafe.Pointer(&sl))
}

func ptrToNTPSHM(shmptr uintptr) (*NTPSHM, error) {
	b := ptrToBytes(shmptr)
	s := &NTPSHM{}
	r := bytes.NewReader(b)
	err := binary.Read(r, hostendian.Order, s)
	return s, err
}

// ReadID reads SHM segment by ID
func ReadID(id uintptr) (*NTPSHM, error) {
	shmptr, _, errno := unix.Syscall(unix.SYS_SHMAT, id, 0, 0)
	if errno != 0 {
		return nil, fmt.Errorf("failed to attach to shm: %s", unix.ErrnoName(errno))
	}

	return ptrToNTPSHM(shmptr)
}

// Read SHM segment
func Read() (*NTPSHM, error) {
	shmID, _, errno := unix.Syscall(unix.SYS_SHMGET, uintptr(SHMKEY), 0, uintptr(0400))
	if errno != 0 {
		return nil, fmt.Errorf("failed get shm: %s", unix.ErrnoName(errno))
	}

	return ReadID(shmID)
}

// Time returns time from SHM
func Time() (time.Time, error) {
	shm, err := Read()
	if err != nil {
		return time.Time{}, err
	}

	return shm.ClockTimeStamp(), nil
}

// ClockTimeStamp returns the clock time
func (n *NTPSHM) ClockTimeStamp() time.Time {
	return time.Unix(n.ClockTimeStampSec, int64(n.ClockTimeStampNSec))
}

// ReceiveTimeStamp returns the receive time
func (n *NTPSHM) ReceiveTimeStamp() time.Time {
	return time.Unix(n.ReceiveTimeStampSec, int64(n.ReceiveTimeStampNSec))
}
