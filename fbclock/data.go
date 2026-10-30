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

#include "fbclock.h" // @oss-only
// @fb-only: #include "time/fbclock/fbclock.h"
#include <string.h>
*/
import "C"

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"path/filepath"
	"unsafe"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

const (
	// DataPath is the v3 shared-memory path.
	DataPath = C.FBCLOCK_DATA_PATH
	// DataSize is the fixed v3 file size.
	DataSize = C.FBCLOCK_DATA_SIZE
	// Readers in containers run as other users.
	dataFileMode = 0o644
)

// DataSectionConfig is how this host serves a source's section, fixed at startup.
type DataSectionConfig struct {
	// Written for monitoring but left out of the header's sources, so clients
	// don't use it.
	ObserveOnly bool
}

// DataClock is one measurement for a section. HoldoverMultiplierNS is in ns per second.
type DataClock struct {
	ClockID              uint32
	RefTimeNS            int64
	SysclockTimeNS       int64
	CoefPPB              int64
	IngressTimeNS        int64
	ErrorBoundNS         uint64
	HoldoverMultiplierNS float64
}

// DataFile writes the file; as with v2, the one sidecar is its only writer.
// One writer per section, so stores to different sections can run at once; close it only after they finish.
type DataFile struct {
	Path     string
	file     *os.File
	mapping  []byte
	sections map[Sources]DataSectionConfig
}

// OpenFBClockData opens and configures the v3 file in place.
func OpenFBClockData(sections map[Sources]DataSectionConfig) (*DataFile, error) {
	return OpenFBClockDataCustom(DataPath, sections)
}

// OpenFBClockDataCustom opens the v3 file at path in place, so reader mappings
// survive a restart; each key of sections is the FBCLOCK_SOURCE_* bit it names.
func OpenFBClockDataCustom(path string, sections map[Sources]DataSectionConfig) (*DataFile, error) {
	for source := range sections {
		if C.fbclock_data_section_index(C.uint32_t(source)) < 0 {
			return nil, fmt.Errorf("fbclock data: %s isn't one source", source)
		}
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	// A file that isn't ours gets zeroed, so only ever open a regular file.
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s isn't a regular file", path)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW, dataFileMode)
	if err != nil {
		return nil, err
	}
	f := &DataFile{Path: path, file: file, sections: maps.Clone(sections)}
	if err := f.initialize(); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func (f *DataFile) initialize() error {
	info, err := f.file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s isn't a regular file", f.Path)
	}
	// The umask cuts the create mode, which also doesn't apply to a file left
	// behind with another mode.
	if err := f.file.Chmod(dataFileMode); err != nil {
		return err
	}
	var header C.fbclock_data_header
	_, readErr := f.file.ReadAt(unsafe.Slice((*byte)(unsafe.Pointer(&header)), C.sizeof_fbclock_data_header), 0)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr
	}
	magic := C.GoBytes(unsafe.Pointer(&header.magic[0]), C.int(len(header.magic)))
	ours := info.Size() == DataSize && readErr == nil &&
		bytes.Equal(magic, []byte(C.FBCLOCK_DATA_MAGIC+"\x00")) && header.major == C.FBCLOCK_DATA_MAJOR
	if !ours {
		if info.Size() > 0 {
			log.Warningf("%s isn't an fbclock data file we can reuse (%d bytes, magic %q, major %d), rebuilding it",
				f.Path, info.Size(), magic, header.major)
		}
		// Resizing mustn't make a foreign header appear valid.
		if _, err := f.file.WriteAt(make([]byte, len(header.magic)), 0); err != nil {
			return err
		}
	}
	if info.Size() != DataSize {
		if err := f.file.Truncate(DataSize); err != nil {
			return err
		}
	}
	f.mapping, err = unix.Mmap(int(f.file.Fd()), 0, DataSize, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return err
	}
	if !ours {
		C.memset(unsafe.Pointer(&f.mapping[0]), 0, C.size_t(DataSize))
	}
	var offered Sources
	for source, config := range f.sections {
		if !config.ObserveOnly {
			offered |= source
		}
	}
	for i := range f.data().sections {
		source := Sources(1) << i
		cSection := f.section(source)
		var clock C.fbclock_data_clock
		result := C.fbclock_data_section_load(cSection, &clock)
		if result == C.FBCLOCK_E_NO_DATA {
			// Never stored, so readers skip it; clear whatever else it holds.
			C.memset(unsafe.Pointer(&cSection.clock), 0, C.size_t(C.sizeof_fbclock_data_clock))
			continue
		}
		if _, ok := f.sections[source]; ok {
			// A restart keeps the anchor, as v2's does; readers widen its bound
			// with holdover.
			if result != 0 {
				log.Warningf("fbclock data section %s: discarding its anchor (load: %s)", source, strerror(result))
				clock = C.fbclock_data_clock{}
			}
			// A newer daemon may have used these; this one writes them as zero.
			clock.reserved = [12]C.uint8_t{}
			C.fbclock_data_section_store(cSection, &clock)
		} else if result != 0 || clock != (C.fbclock_data_clock{}) {
			log.Warningf("fbclock data section %s: clearing it, this host no longer serves it", source)
			clock = C.fbclock_data_clock{}
			C.fbclock_data_section_store(cSection, &clock)
		}
	}
	C.fbclock_data_header_store(&f.data().header, C.uint32_t(offered))
	return nil
}

func (f *DataFile) data() *C.fbclock_data {
	return (*C.fbclock_data)(unsafe.Pointer(&f.mapping[0]))
}

// section is source's section; the caller checks source is one bit.
func (f *DataFile) section(source Sources) *C.fbclock_data_section {
	return &f.data().sections[C.fbclock_data_section_index(C.uint32_t(source))]
}

func dataClockToC(d DataClock) *C.fbclock_data_clock {
	// FloatAsUint32 can turn a NaN or negative rate into 0, a bound that never
	// grows. Readers refuse UINT32_MAX, so an unusable rate is written as that.
	holdover := uint32(math.MaxUint32)
	if d.HoldoverMultiplierNS >= 0 {
		holdover = FloatAsUint32(d.HoldoverMultiplierNS)
	}
	return &C.fbclock_data_clock{
		ref_time_ns:            C.int64_t(d.RefTimeNS),
		sysclock_time_ns:       C.int64_t(d.SysclockTimeNS),
		coef_ppb:               C.int64_t(d.CoefPPB),
		clock_id:               C.uint32_t(d.ClockID),
		error_bound_ns:         C.uint32_t(Uint64ToUint32(d.ErrorBoundNS)),
		ingress_time_ns:        C.int64_t(d.IngressTimeNS),
		holdover_multiplier_ns: C.uint32_t(holdover),
	}
}

// Store publishes one measurement in source's section under its seqlock.
func (f *DataFile) Store(source Sources, clock DataClock) error {
	if f.file == nil || f.mapping == nil {
		return os.ErrClosed
	}
	if _, ok := f.sections[source]; !ok {
		return fmt.Errorf("fbclock data: source %s isn't configured", source)
	}
	C.fbclock_data_section_store(f.section(source), dataClockToC(clock))
	return nil
}

// Close unmaps and closes the file. It may be called again after closing.
func (f *DataFile) Close() error {
	var unmapErr, closeErr error
	if f.mapping != nil {
		unmapErr = unix.Munmap(f.mapping)
		f.mapping = nil
	}
	if f.file != nil {
		closeErr = f.file.Close()
		f.file = nil
	}
	return errors.Join(unmapErr, closeErr)
}
