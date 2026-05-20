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

package fbclock

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const PTPPath = "/dev/null"

const (
	shmDataSize   = 48 // sizeof(fbclock_shmdata): 8 (atomic_uint64) + 40 (fbclock_clockdata)
	shmDataV2Size = 64 // sizeof(fbclock_shmdata_v2): 8 (atomic_uint64) + 56 (fbclock_clockdata_v2)
	shmPath       = "/run/fbclock_data_v1"
	shmPathV2     = "/run/fbclock_data_v2"
	maxReadTries  = 1000
)

// OpenShm opens a shared memory file
func OpenShm(path string, flags int, permissions os.FileMode) (*Shm, error) {
	oldUmask := syscall.Umask(0)
	defer syscall.Umask(oldUmask)
	file, err := os.OpenFile(path, flags, permissions)
	if err != nil {
		return nil, err
	}
	return &Shm{File: file, Path: path}, nil
}

// OpenFBClockShmCustom returns opened POSIX shared mem used by fbclock
func OpenFBClockShmCustom(path string) (*Shm, error) {
	return OpenFBClockShmCustomVer(path, 1)
}

// OpenFBClockShmCustomVer returns opened POSIX shared mem used by fbclock
func OpenFBClockShmCustomVer(path string, version int) (*Shm, error) {
	shm, err := OpenShm(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	size := int64(shmDataSize)
	if version == 2 {
		size = int64(shmDataV2Size)
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
	return OpenFBClockShmCustom(shmPath)
}

// OpenFBClockSHMv2 returns opened POSIX shared mem used by fbclock
func OpenFBClockSHMv2() (*Shm, error) {
	return OpenFBClockShmCustomVer(shmPathV2, 2)
}

// MmapShmpData mmaps open file as fbclock shared memory
func MmapShmpData(fd uintptr) (unsafe.Pointer, error) {
	data, err := unix.Mmap(int(fd), 0, shmDataSize, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	return unsafe.Pointer(&data[0]), nil
}

// MmapShmpDataV2 mmaps open file as fbclock v2 shared memory
func MmapShmpDataV2(fd uintptr) (unsafe.Pointer, error) {
	data, err := unix.Mmap(int(fd), 0, shmDataV2Size, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	return unsafe.Pointer(&data[0]), nil
}

func clockDataCRC(d *[40]byte) uint64 {
	ingress := binary.LittleEndian.Uint64(d[0:8])
	errorBound := uint64(binary.LittleEndian.Uint32(d[8:12]))
	holdover := uint64(binary.LittleEndian.Uint32(d[12:16]))
	counter := uint64(0xFFFFFFFF) ^ ingress
	counter ^= errorBound
	counter ^= holdover
	return counter ^ 0xFFFFFFFF
}

// StoreFBClockData writes fbclock data to shared memory via mmap
func StoreFBClockData(fd uintptr, d Data) error {
	data, err := unix.Mmap(int(fd), 0, shmDataSize, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("mmap failed: %w", err)
	}
	defer func() { _ = unix.Munmap(data) }()

	buf := (*[40]byte)(unsafe.Pointer(&data[8]))
	binary.LittleEndian.PutUint64(buf[0:8], uint64(d.IngressTimeNS))
	binary.LittleEndian.PutUint32(buf[8:12], Uint64ToUint32(d.ErrorBoundNS))
	binary.LittleEndian.PutUint32(buf[12:16], FloatAsUint32(d.HoldoverMultiplierNS))
	binary.LittleEndian.PutUint64(buf[16:24], d.SmearingStartS)
	binary.LittleEndian.PutUint64(buf[24:32], d.SmearingEndS)
	binary.LittleEndian.PutUint32(buf[32:36], uint32(d.UTCOffsetPreS))
	binary.LittleEndian.PutUint32(buf[36:40], uint32(d.UTCOffsetPostS))

	crc := clockDataCRC(buf)
	atomic.StoreUint64((*uint64)(unsafe.Pointer(&data[0])), crc)
	return nil
}

// ReadFBClockData reads Data from mmaped fbclock shared memory
func ReadFBClockData(shmp unsafe.Pointer) (*Data, error) {
	for range maxReadTries {
		base := (*[shmDataSize]byte)(shmp)
		var buf [40]byte
		copy(buf[:], base[8:48])
		crc := atomic.LoadUint64((*uint64)(unsafe.Pointer(&base[0])))
		if clockDataCRC(&buf) == crc {
			return &Data{
				IngressTimeNS:        int64(binary.LittleEndian.Uint64(buf[0:8])),
				ErrorBoundNS:         uint64(binary.LittleEndian.Uint32(buf[8:12])),
				HoldoverMultiplierNS: Uint32AsFloat(binary.LittleEndian.Uint32(buf[12:16])),
			}, nil
		}
	}
	return nil, fmt.Errorf("CRC check failed after %d tries", maxReadTries)
}

// StoreFBClockDataV2 writes fbclock v2 data to shared memory via mmap using seqlock
func StoreFBClockDataV2(fd uintptr, d DataV2) error {
	data, err := unix.Mmap(int(fd), 0, shmDataV2Size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("mmap failed: %w", err)
	}
	defer func() { _ = unix.Munmap(data) }()

	seqPtr := (*uint64)(unsafe.Pointer(&data[0]))

	for range maxReadTries {
		seq := atomic.LoadUint64(seqPtr)
		if seq&1 != 0 {
			time.Sleep(time.Microsecond)
			continue
		}
		seq = (seq &^ 1) + 1
		atomic.StoreUint64(seqPtr, seq)
		seq++

		buf := data[8:64]
		binary.LittleEndian.PutUint64(buf[0:8], uint64(d.IngressTimeNS))
		binary.LittleEndian.PutUint32(buf[8:12], Uint64ToUint32(d.ErrorBoundNS))
		binary.LittleEndian.PutUint32(buf[12:16], FloatAsUint32(d.HoldoverMultiplierNS))
		binary.LittleEndian.PutUint64(buf[16:24], d.SmearingStartS)
		binary.LittleEndian.PutUint16(buf[24:26], uint16(d.UTCOffsetPreS))
		binary.LittleEndian.PutUint16(buf[26:28], uint16(d.UTCOffsetPostS))
		binary.LittleEndian.PutUint32(buf[28:32], d.ClockID)
		binary.LittleEndian.PutUint64(buf[32:40], uint64(d.PHCTimeNS))
		binary.LittleEndian.PutUint64(buf[40:48], uint64(d.SysclockTimeNS))
		binary.LittleEndian.PutUint64(buf[48:56], uint64(d.CoefPPB))

		if seq == 0 {
			seq = 2
		}
		atomic.StoreUint64(seqPtr, seq)
		return nil
	}
	return fmt.Errorf("seqlock contention after %d tries", maxReadTries)
}

// ReadFBClockDataV2 reads DataV2 from mmaped fbclock shared memory using seqlock
func ReadFBClockDataV2(shmp unsafe.Pointer) (*DataV2, error) {
	base := (*[shmDataV2Size]byte)(shmp)
	seqPtr := (*uint64)(unsafe.Pointer(&base[0]))

	for range maxReadTries {
		seq := atomic.LoadUint64(seqPtr)
		if seq == 0 {
			time.Sleep(10 * time.Microsecond)
			continue
		}
		if seq&1 != 0 {
			continue
		}
		var buf [56]byte
		copy(buf[:], base[8:64])
		if seq == atomic.LoadUint64(seqPtr) {
			return &DataV2{
				IngressTimeNS:        int64(binary.LittleEndian.Uint64(buf[0:8])),
				ErrorBoundNS:         uint64(binary.LittleEndian.Uint32(buf[8:12])),
				HoldoverMultiplierNS: Uint32AsFloat(binary.LittleEndian.Uint32(buf[12:16])),
				UTCOffsetPreS:        int16(binary.LittleEndian.Uint16(buf[24:26])),
				UTCOffsetPostS:       int16(binary.LittleEndian.Uint16(buf[26:28])),
				ClockID:              binary.LittleEndian.Uint32(buf[28:32]),
				PHCTimeNS:            int64(binary.LittleEndian.Uint64(buf[32:40])),
				SysclockTimeNS:       int64(binary.LittleEndian.Uint64(buf[40:48])),
				CoefPPB:              int64(binary.LittleEndian.Uint64(buf[48:56])),
			}, nil
		}
	}
	return nil, fmt.Errorf("seqlock read failed after %d tries", maxReadTries)
}
