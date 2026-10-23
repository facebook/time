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
#define FBCLOCK_CGO 1

#include <stddef.h>

#include "fbclock.h" // @oss-only
// @fb-only: #include "time/fbclock/fbclock.h"

// A v2 section has a v3 section's shape, so the v3 section load reads it too.
_Static_assert(sizeof(fbclock_shmsection) == sizeof(fbclock_data_section), "section");
_Static_assert(sizeof(fbclock_clockdata_v2) == sizeof(fbclock_data_clock), "clock");
_Static_assert(offsetof(fbclock_shmsection, data) == offsetof(fbclock_data_section, clock), "offset");
*/
import "C"

import (
	"fmt"
	"math/big"
	"math/bits"
	"os"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/unix"
)

// DataV2Path is the v2 shared-memory path.
const DataV2Path = C.FBCLOCK_DIR_PATH

// DataFileSnapshot is a diagnostic view, without the library's serve checks.
type DataFileSnapshot struct {
	Path     string                `json:"path"`
	Version  int                   `json:"version"`
	Size     int64                 `json:"size"`
	Header   *DataHeaderSnapshot   `json:"header,omitempty"`
	Sections []DataSectionSnapshot `json:"sections"`
}

// DataHeaderSnapshot contains the v3 header's identity, version and the
// FBCLOCK_SOURCE_* bits the host offers, unknown ones included.
type DataHeaderSnapshot struct {
	Magic   string `json:"magic"`
	Major   uint8  `json:"major"`
	Minor   uint8  `json:"minor"`
	Sources uint32 `json:"sources"`
}

// DataSectionSnapshot has a clock only after a consistent seqlock read; Seq is
// the raw sequence word. ReadError and AgeError are diagnostics, not file errors.
type DataSectionSnapshot struct {
	Name           string             `json:"name"`
	Offset         int                `json:"offset"`
	Seq            uint64             `json:"seq"`
	ReadError      string             `json:"read_error,omitempty"`
	Clock          *DataClockSnapshot `json:"clock,omitempty"`
	AnchorAgeNS    *int64             `json:"anchor_age_ns,omitempty"`
	SinceIngressNS *int64             `json:"since_ingress_ns,omitempty"`
	AgeError       string             `json:"age_error,omitempty"`
}

// DataClockSnapshot retains raw fields. Pointers distinguish fields absent
// from one version from fields whose stored value is zero.
type DataClockSnapshot struct {
	Reserved              *[12]uint8 `json:"reserved,omitempty"`
	ClockID               uint32     `json:"clock_id"`
	RefTimeNS             *int64     `json:"ref_time_ns,omitempty"`
	PHCTimeNS             *int64     `json:"phc_time_ns,omitempty"`
	SysclockTimeNS        int64      `json:"sysclock_time_ns"`
	CoefPPB               int64      `json:"coef_ppb"`
	IngressTimeNS         int64      `json:"ingress_time_ns"`
	ErrorBoundNS          uint64     `json:"error_bound_ns"`
	HoldoverMultiplierRaw uint32     `json:"holdover_multiplier_raw"`
	HoldoverMultiplierNS  float64    `json:"holdover_multiplier_ns_per_second"`
	SmearingStartS        *uint64    `json:"clock_smearing_start_s,omitempty"`
	UTCOffsetPreS         *int16     `json:"utc_offset_pre_s,omitempty"`
	UTCOffsetPostS        *int16     `json:"utc_offset_post_s,omitempty"`
}

// ReadFBClockDataFile maps path read-only and copies its sections consistently.
// v2 selects the legacy layout; unavailable sections still appear as diagnostics.
func ReadFBClockDataFile(path string, v2 bool) (*DataFileSnapshot, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	size, version := int(DataSize), int(C.FBCLOCK_DATA_MAJOR)
	if v2 {
		size, version = int(C.FBCLOCK_SHMDATA_V2_SIZE), 2
	}
	if info.Size() < int64(size) {
		return nil, fmt.Errorf("%s is %d bytes, v%d needs %d", path, info.Size(), version, size)
	}
	mapping, err := unix.Mmap(int(file.Fd()), 0, size, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	defer func() { _ = unix.Munmap(mapping) }()
	view := &DataFileSnapshot{Path: path, Version: version, Size: info.Size()}
	if v2 {
		for index, name := range []string{"primary", "realtime"} {
			view.Sections = append(view.Sections, readDataV2Section(mapping, name, index*C.sizeof_fbclock_shmsection))
		}
		return view, nil
	}
	var header C.fbclock_data_header
	C.fbclock_data_header_load((*C.fbclock_data_header)(unsafe.Pointer(&mapping[0])), &header)
	magic := string(C.GoBytes(unsafe.Pointer(&header.magic[0]), 8))
	view.Header = &DataHeaderSnapshot{
		Magic: magic, Major: uint8(header.major), Minor: uint8(header.minor), Sources: uint32(header.sources),
	}
	if magic != C.FBCLOCK_DATA_MAGIC+"\x00" {
		return nil, fmt.Errorf("%s has wrong fbclock magic %q", path, magic)
	}
	if view.Header.Major != C.FBCLOCK_DATA_MAJOR {
		return nil, fmt.Errorf("%s has fbclock major %d, need %d", path, view.Header.Major, C.FBCLOCK_DATA_MAJOR)
	}
	data := (*C.fbclock_data)(unsafe.Pointer(&mapping[0]))
	for i := range data.sections {
		source := Sources(1) << i
		offset := int(uintptr(unsafe.Pointer(&data.sections[i])) - uintptr(unsafe.Pointer(data)))
		snapshot := readDataSection(mapping, DataSectionName(source), offset)
		// The known sources always show; another bit's once it's offered or written.
		if source <= SourceNTP || view.Header.Sources&uint32(source) != 0 || snapshot.Seq != 0 {
			view.Sections = append(view.Sections, snapshot)
		}
	}
	return view, nil
}

// DataSectionName names one source bit's section: its flag, or bit_k for a bit
// this build doesn't know.
func DataSectionName(source Sources) string {
	switch source {
	case SourcePTP:
		return "ptp"
	case SourcePRTC:
		return "prtc"
	case SourceNTP:
		return "ntp"
	}
	return fmt.Sprintf("bit_%d", bits.TrailingZeros32(uint32(source)))
}

func readDataSection(mapping []byte, name string, offset int) DataSectionSnapshot {
	section := (*C.fbclock_data_section)(unsafe.Pointer(&mapping[offset]))
	var clock C.fbclock_data_clock
	result := C.fbclock_data_section_load(section, &clock)
	snapshot := DataSectionSnapshot{Name: name, Offset: offset, Seq: readSeq(mapping, offset)}
	if result != 0 {
		snapshot.ReadError = strerror(result)
		return snapshot
	}
	var reserved [12]uint8
	for index := range reserved {
		reserved[index] = uint8(clock.reserved[index])
	}
	snapshot.Clock = &DataClockSnapshot{
		Reserved: &reserved, ClockID: uint32(clock.clock_id), RefTimeNS: new(int64(clock.ref_time_ns)),
		SysclockTimeNS: int64(clock.sysclock_time_ns), CoefPPB: int64(clock.coef_ppb),
		IngressTimeNS: int64(clock.ingress_time_ns), ErrorBoundNS: uint64(clock.error_bound_ns),
		HoldoverMultiplierRaw: uint32(clock.holdover_multiplier_ns),
		HoldoverMultiplierNS:  Uint32AsFloat(uint32(clock.holdover_multiplier_ns)),
	}
	dataSectionAges(&snapshot, int64(clock.ref_time_ns))
	return snapshot
}

func readDataV2Section(mapping []byte, name string, offset int) DataSectionSnapshot {
	section := (*C.fbclock_data_section)(unsafe.Pointer(&mapping[offset]))
	var raw C.fbclock_data_clock
	result := C.fbclock_data_section_load(section, &raw)
	snapshot := DataSectionSnapshot{Name: name, Offset: offset, Seq: readSeq(mapping, offset)}
	if result != 0 {
		snapshot.ReadError = strerror(result)
		return snapshot
	}
	clock := (*C.fbclock_clockdata_v2)(unsafe.Pointer(&raw))
	data := clockDataV2FromC(clock)
	snapshot.Clock = &DataClockSnapshot{
		ClockID: data.ClockID, PHCTimeNS: new(data.PHCTimeNS),
		SysclockTimeNS: data.SysclockTimeNS, CoefPPB: data.CoefPPB,
		IngressTimeNS: data.IngressTimeNS, ErrorBoundNS: data.ErrorBoundNS,
		HoldoverMultiplierRaw: uint32(clock.holdover_multiplier_ns),
		HoldoverMultiplierNS:  data.HoldoverMultiplierNS,
		SmearingStartS:        new(data.SmearingStartS),
		UTCOffsetPreS:         new(data.UTCOffsetPreS),
		UTCOffsetPostS:        new(data.UTCOffsetPostS),
	}
	dataSectionAges(&snapshot, data.PHCTimeNS)
	return snapshot
}

// readSeq reads a section's sequence word as it is now, odd while a write is in
// progress and 0 if the section was never written.
func readSeq(mapping []byte, offset int) uint64 {
	return atomic.LoadUint64((*uint64)(unsafe.Pointer(&mapping[offset])))
}

// dataClockNow reads a section's base clock; fbclock anchors only use these.
func dataClockNow(clockID uint32) (int64, error) {
	var id int32
	switch clockID {
	case unix.CLOCK_REALTIME:
		id = unix.CLOCK_REALTIME
	case unix.CLOCK_MONOTONIC_RAW:
		id = unix.CLOCK_MONOTONIC_RAW
	default:
		return 0, fmt.Errorf("unsupported base clock %d", clockID)
	}
	var ts unix.Timespec
	if err := unix.ClockGettime(id, &ts); err != nil {
		return 0, err
	}
	return ts.Nano(), nil
}

func dataSectionAges(section *DataSectionSnapshot, refTimeNS int64) {
	clock := section.Clock
	now, err := dataClockNow(clock.ClockID)
	if err != nil {
		section.AgeError = err.Error()
		return
	}
	section.AnchorAgeNS, section.SinceIngressNS = dataAges(now, clock.SysclockTimeNS, clock.CoefPPB, refTimeNS, clock.IngressTimeNS)
	if section.AnchorAgeNS == nil || section.SinceIngressNS == nil {
		section.AgeError = "computed age exceeds int64"
	}
}

// dataAges returns the anchor's age on its base clock and the time since
// ingress on the source's clock, or nil where a result exceeds int64.
func dataAges(now, sysclockTimeNS, coefPPB, refTimeNS, ingressTimeNS int64) (*int64, *int64) {
	// Raw fields can span int64; keep the signed extrapolation exact.
	var age, correction, since big.Int
	age.Sub(big.NewInt(now), big.NewInt(sysclockTimeNS))
	correction.Mul(&age, big.NewInt(coefPPB))
	correction.Quo(&correction, big.NewInt(1000000000))
	since.Sub(big.NewInt(refTimeNS), big.NewInt(ingressTimeNS))
	since.Add(&since, &age)
	since.Add(&since, &correction)
	var anchorAge, sinceIngress *int64
	if age.IsInt64() {
		anchorAge = new(age.Int64())
	}
	if since.IsInt64() {
		sinceIngress = new(since.Int64())
	}
	return anchorAge, sinceIngress
}
