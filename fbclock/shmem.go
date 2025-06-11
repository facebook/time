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

package fbclock

/*
#cgo LDFLAGS: -lrt

#include "fbclock.h" // @oss-only
// @fb-only: #include "time/fbclock/fbclock.h"

#include <stdlib.h>   // for free()
#include <sys/stat.h> // For mode constants
#include <fcntl.h>    // For O_* constants
#include <unistd.h>   // for ftruncate

*/
import "C"

import (
	"fmt"
	"math"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// PTPPath is the path we set for PTP device
const PTPPath = C.FBCLOCK_PTPPATH

// Shm is POSIX shared memory
type Shm struct {
	Path    string
	File    *os.File
	Version int
}

// OpenShm opens POSIX shared memory
func OpenShm(path string, flags int, permissions os.FileMode) (*Shm, error) {
	var err error
	// store old umask, set it to 0.
	// otherwise our creating flags might be affected by umask
	// and shmem won't be readable by all users.
	oldUmask := C.umask(0)
	// make sure we return umask back to original value
	defer C.umask(oldUmask)
	file, err := os.OpenFile(path, flags, permissions)
	if err != nil {
		return nil, err
	}
	return &Shm{File: file, Path: path}, nil
}

// Close cleans up open POSIX shm resources
func (s *Shm) Close() error {
	if err := s.File.Close(); err != nil {
		return err
	}

	cPath := C.CString(s.Path)
	defer C.free(unsafe.Pointer(cPath))
	return nil
}

// Data is a Go equivalent of what we want to store in shared memory for fbclock to use
type Data struct {
	IngressTimeNS        int64
	ErrorBoundNS         uint64
	HoldoverMultiplierNS float64 // float stored as multiplier of 2**16
	SmearingStartS       uint64  // Smearing starts before the Leap Second Event Time (midnight on June-30 or Dec-31)
	SmearingEndS         uint64  // Smearing ends after the Leap Second Event Time (midnight on June-30 or Dec-31)
	UTCOffsetPreS        int32   // UTC Offset before Leap Second Event
	UTCOffsetPostS       int32   // UTC Offset after Leap Second Event
}

// DataV2 is a Go equivalent of what we want to store in shared memory for fbclock to use
type DataV2 struct {
	IngressTimeNS        int64
	ErrorBoundNS         uint64
	HoldoverMultiplierNS float64 // float stored as multiplier of 2**16
	SmearingStartS       uint64  // Smearing starts before the Leap Second Event Time (midnight on June-30 or Dec-31)
	UTCOffsetPreS        int16   // UTC Offset before Leap Second Event
	UTCOffsetPostS       int16   // UTC Offset after Leap Second Event
	ClockID              uint32  // Clock ID of SysclockTimeNS (MONOTONIC_RAW or REALTIME)
	PHCTimeNS            int64   // Periodically updated PHC time used to calculate real PHC time
	SysclockTimeNS       int64   // Periodically updated system clock time (MONOTONIC_RAW or REALTIME) received with PHC time
	CoefPPB              int64   // Coefficient of the approximation of the PHC time
}

// OpenFBClockShmCustom returns opened POSIX shared mem used by fbclock,
// with custom path and version specified
func OpenFBClockShmCustom(path string) (*Shm, error) {
	return OpenFBClockShmCustomVer(path, 1)
}

// OpenFBClockShmCustomVer returns opened POSIX shared mem used by fbclock,
// with custom path and version specified
func OpenFBClockShmCustomVer(path string, version int) (*Shm, error) {
	shm, err := OpenShm(
		path,
		C.O_CREAT|C.O_RDWR,
		C.S_IRUSR|C.S_IWUSR|C.S_IRGRP|C.S_IROTH,
	)
	if err != nil {
		return nil, err
	}
	size := int64(C.FBCLOCK_SHMDATA_SIZE)
	if version == 2 {
		size = int64(C.FBCLOCK_SHMDATA_V2_SIZE)
	}
	if err := shm.File.Truncate(size); err != nil {
		shm.Close()
		return nil, err
	}
	shm.Version = version
	return shm, nil
}

// OpenFBClockSHM returns opened POSIX shared mem used by fbclock
func OpenFBClockSHM() (*Shm, error) {
	return OpenFBClockShmCustom(C.FBCLOCK_PATH)
}

// OpenFBClockSHMv2 returns opened POSIX shared mem used by fbclock
func OpenFBClockSHMv2() (*Shm, error) {
	return OpenFBClockShmCustomVer(C.FBCLOCK_PATH_V2, 2)
}

// FloatAsUint32 stores float as multiplier of 2**16.
// Effectively this means we can store max 65k like this.
func FloatAsUint32(val float64) uint32 {
	valAsUint := C.FBCLOCK_POW2_16 * val
	if valAsUint > math.MaxUint32 {
		valAsUint = math.MaxUint32
	}
	return uint32(valAsUint)
}

// Uint32AsFloat restores float that was stored as a multiplier of 2**16.
func Uint32AsFloat(val uint32) float64 {
	return float64(val) / C.FBCLOCK_POW2_16
}

// Uint64ToUint32 converts uint64 to uint32, handling the overflow.
// If the uint64 value is more than MaxUint32, result is set to MaxUint32.
func Uint64ToUint32(val uint64) uint32 {
	if val > math.MaxUint32 {
		val = math.MaxUint32
	}
	return uint32(val)
}

// StoreFBClockData will store fbclock data in shared mem,
// fd param should be open file descriptor of that shared mem.
func StoreFBClockData(fd uintptr, d Data) error {
	cData := &C.fbclock_clockdata{
		ingress_time_ns:        C.int64_t(d.IngressTimeNS),
		error_bound_ns:         C.uint32_t(Uint64ToUint32(d.ErrorBoundNS)),
		holdover_multiplier_ns: C.uint32_t(FloatAsUint32(d.HoldoverMultiplierNS)),
		clock_smearing_start_s: C.uint64_t(d.SmearingStartS),
		clock_smearing_end_s:   C.uint64_t(d.SmearingEndS),
		utc_offset_pre_s:       C.int32_t(d.UTCOffsetPreS),
		utc_offset_post_s:      C.int32_t(d.UTCOffsetPostS),
	}
	// fbclock_clockdata_store_data comes from fbclock.c
	res := C.fbclock_clockdata_store_data(C.uint(fd), cData)
	if res != 0 {
		return fmt.Errorf("failed to store data: %s", strerror(res))
	}
	return nil
}

// MmapShmpData mmaps open file as fbclock shared memory. Used in tests only.
func MmapShmpData(fd uintptr) (unsafe.Pointer, error) {
	data, err := unix.Mmap(int(fd), 0, C.FBCLOCK_SHMDATA_SIZE, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	return unsafe.Pointer(&data[0]), nil
}

// ReadFBClockData will read Data from mmaped fbclock shared memory. Used in tests only
func ReadFBClockData(shmp unsafe.Pointer) (*Data, error) {
	cData := &C.fbclock_clockdata{}
	shmpData := (*C.fbclock_shmdata)(shmp)
	// fbclock_clockdata_load_data comes from fbclock.c
	res := C.fbclock_clockdata_load_data(shmpData, cData)
	if res != 0 {
		return nil, fmt.Errorf("failed to store data: %s", strerror(res))
	}
	return &Data{
		IngressTimeNS:        int64(cData.ingress_time_ns),
		ErrorBoundNS:         uint64(cData.error_bound_ns),
		HoldoverMultiplierNS: Uint32AsFloat(uint32(cData.holdover_multiplier_ns)),
	}, nil
}

// MmapShmpDataV2 mmaps open file as fbclock shared memory. Used in tests only.
func MmapShmpDataV2(fd uintptr) (unsafe.Pointer, error) {
	data, err := unix.Mmap(int(fd), 0, C.FBCLOCK_SHMDATA_V2_SIZE, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	return unsafe.Pointer(&data[0]), nil
}

// StoreFBClockDataV2 will store fbclock data in shared mem,
// fd param should be open file descriptor of that shared mem.
func StoreFBClockDataV2(fd uintptr, d DataV2) error {
	cData := &C.fbclock_clockdata_v2{
		ingress_time_ns:        C.int64_t(d.IngressTimeNS),
		error_bound_ns:         C.uint32_t(Uint64ToUint32(d.ErrorBoundNS)),
		holdover_multiplier_ns: C.uint32_t(FloatAsUint32(d.HoldoverMultiplierNS)),
		clock_smearing_start_s: C.uint64_t(d.SmearingStartS),
		utc_offset_pre_s:       C.int16_t(d.UTCOffsetPreS),
		utc_offset_post_s:      C.int16_t(d.UTCOffsetPostS),
		clockId:                C.uint32_t(d.ClockID),
		phc_time_ns:            C.int64_t(d.PHCTimeNS),
		sysclock_time_ns:       C.int64_t(d.SysclockTimeNS),
		coef_ppb:               C.int64_t(d.CoefPPB),
	}
	// fbclock_clockdata_store_data comes from fbclock.c
	res := C.fbclock_clockdata_store_data_v2(C.uint(fd), cData)
	if res != 0 {
		return fmt.Errorf("failed to store data: %s", strerror(res))
	}
	return nil
}

// ReadFBClockDataV2 will read Data from mmaped fbclock shared memory. Used in tests only
func ReadFBClockDataV2(shmp unsafe.Pointer) (*DataV2, error) {
	cData := &C.fbclock_clockdata_v2{}
	shmpData := (*C.fbclock_shmdata_v2)(shmp)
	// fbclock_clockdata_load_data comes from fbclock.c
	res := C.fbclock_clockdata_load_data_v2(shmpData, cData)
	if res != 0 {
		return nil, fmt.Errorf("failed to store data: %s", strerror(res))
	}
	return &DataV2{
		IngressTimeNS:        int64(cData.ingress_time_ns),
		ErrorBoundNS:         uint64(cData.error_bound_ns),
		HoldoverMultiplierNS: Uint32AsFloat(uint32(cData.holdover_multiplier_ns)),
		PHCTimeNS:            int64(cData.phc_time_ns),
		SysclockTimeNS:       int64(cData.sysclock_time_ns),
		ClockID:              uint32(cData.clockId),
		CoefPPB:              int64(cData.coef_ppb),
	}, nil
}
