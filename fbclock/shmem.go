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

#include "fbclock.h"

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
	Path string
	File *os.File
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
	HoldoverMultiplierNS float64 // float stored as multiplier of  2**16
}

// OpenFBClockShmCustom returns opened POSIX shared mem used by fbclock,
// with custom path
func OpenFBClockShmCustom(path string) (*Shm, error) {
	shm, err := OpenShm(
		path,
		C.O_CREAT|C.O_RDWR,
		C.S_IRUSR|C.S_IWUSR|C.S_IRGRP|C.S_IROTH,
	)
	if err != nil {
		return nil, err
	}
	if err := shm.File.Truncate(C.FBCLOCK_SHMDATA_SIZE); err != nil {
		shm.Close()
		return nil, err
	}
	return shm, nil
}

// OpenFBClockSHM returns opened POSIX shared mem used by fbclock
func OpenFBClockSHM() (*Shm, error) {
	return OpenFBClockShmCustom(C.FBCLOCK_PATH)
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
