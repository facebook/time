//go:build linux

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
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// PTPPath is the path we set for PTP device
const PTPPath = C.FBCLOCK_PTPPATH

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

// OpenFBClockShmCustom returns opened POSIX shared mem used by fbclock,
// with custom path and version specified
func OpenFBClockShmCustom(path string) (*Shm, error) {
	return OpenFBClockShmCustomVer(path, 2)
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
	size := int64(C.FBCLOCK_SHMDATA_V2_SIZE)
	if version != 2 {
		size = int64(C.FBCLOCK_SHMDATA_SIZE)
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
	return OpenFBClockShmCustomVer(C.FBCLOCK_PATH, 2)
}

// OpenFBClockSHMv2 returns opened POSIX shared mem used by fbclock
func OpenFBClockSHMv2() (*Shm, error) {
	// TODO: remove this once all call sites are updated to use v2
	return OpenFBClockShmCustomVer(C.FBCLOCK_PATH, 2)
}

// OpenFBClockSHMv1 returns opened POSIX shared mem used by fbclock
func OpenFBClockSHMv1() (*Shm, error) {
	return OpenFBClockShmCustomVer(C.FBCLOCK_PATH_V1, 1)
}

// StoreFBClockData is a wrapper for StoreFBClockDataV2
func StoreFBClockData(fd uintptr, d DataV2) error {
	return StoreFBClockDataV2(fd, d)
}

// StoreFBClockDataV1 will store fbclock data in shared mem,
// fd param should be open file descriptor of that shared mem.
func StoreFBClockDataV1(fd uintptr, d Data) error {
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
	return MmapShmpDataV2(fd)
}

// MmapShmpDataV1 mmaps open file as fbclock shared memory. Used in tests only.
func MmapShmpDataV1(fd uintptr) (unsafe.Pointer, error) {
	data, err := unix.Mmap(int(fd), 0, C.FBCLOCK_SHMDATA_SIZE, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	return unsafe.Pointer(&data[0]), nil
}

// ReadFBClockData will read Data from mmaped fbclock shared memory. Used in tests only
func ReadFBClockData(shmp unsafe.Pointer) (*DataV2, error) {
	return ReadFBClockDataV2(shmp)
}

// ReadFBClockDataV1 will read Data from mmaped fbclock shared memory. Used in tests only
func ReadFBClockDataV1(shmp unsafe.Pointer) (*Data, error) {
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

// clockDataV2ToC builds the C clockdata_v2 struct from a DataV2.
func clockDataV2ToC(d DataV2) *C.fbclock_clockdata_v2 {
	return &C.fbclock_clockdata_v2{
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
}

// clockDataV2FromC builds a DataV2 from the C clockdata_v2 struct.
func clockDataV2FromC(c *C.fbclock_clockdata_v2) *DataV2 {
	return &DataV2{
		IngressTimeNS:        int64(c.ingress_time_ns),
		ErrorBoundNS:         uint64(c.error_bound_ns),
		HoldoverMultiplierNS: Uint32AsFloat(uint32(c.holdover_multiplier_ns)),
		SmearingStartS:       uint64(c.clock_smearing_start_s),
		UTCOffsetPreS:        int16(c.utc_offset_pre_s),
		UTCOffsetPostS:       int16(c.utc_offset_post_s),
		ClockID:              uint32(c.clockId),
		PHCTimeNS:            int64(c.phc_time_ns),
		SysclockTimeNS:       int64(c.sysclock_time_ns),
		CoefPPB:              int64(c.coef_ppb),
	}
}

// StoreFBClockDataV2 stores the primary anchor section.
// fd param should be open file descriptor of that shared mem.
func StoreFBClockDataV2(fd uintptr, d DataV2) error {
	// fbclock_clockdata_store_data_v2 comes from fbclock.c
	res := C.fbclock_clockdata_store_data_v2(C.uint(fd), clockDataV2ToC(d))
	if res != 0 {
		return fmt.Errorf("failed to store data: %s", strerror(res))
	}
	return nil
}

// StoreFBClockDataRealtime stores the realtime anchor section, used on hosts
// whose primary is MONOTONIC_RAW. It's a separate call from StoreFBClockDataV2
// so the daemon can publish the primary anchor before the slower REALTIME read.
func StoreFBClockDataRealtime(fd uintptr, d DataV2) error {
	// fbclock_clockdata_store_data_realtime comes from fbclock.c
	res := C.fbclock_clockdata_store_data_realtime(C.uint(fd), clockDataV2ToC(d))
	if res != 0 {
		return fmt.Errorf("failed to store realtime data: %s", strerror(res))
	}
	return nil
}

// ReadFBClockDataV2 reads the primary anchor section. Used in tests only.
func ReadFBClockDataV2(shmp unsafe.Pointer) (*DataV2, error) {
	cData := &C.fbclock_clockdata_v2{}
	shmpData := (*C.fbclock_shmdata_v2)(shmp)
	res := C.fbclock_clockdata_load_data_v2(shmpData, cData)
	if res != 0 {
		return nil, fmt.Errorf("failed to load data: %s", strerror(res))
	}
	return clockDataV2FromC(cData), nil
}

// ReadFBClockDataRealtime reads the realtime anchor section. Used in tests only.
func ReadFBClockDataRealtime(shmp unsafe.Pointer) (*DataV2, error) {
	cData := &C.fbclock_clockdata_v2{}
	shmpData := (*C.fbclock_shmdata_v2)(shmp)
	res := C.fbclock_clockdata_load_data_realtime(shmpData, cData)
	if res != 0 {
		return nil, fmt.Errorf("failed to load realtime data: %s", strerror(res))
	}
	return clockDataV2FromC(cData), nil
}
