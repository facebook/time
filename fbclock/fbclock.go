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
#cgo amd64 CFLAGS: -msse4.2

#include "fbclock.h" // @oss-only
// @fb-only: #include "time/fbclock/fbclock.h"

#include <stdlib.h>   // for free()
*/
import "C"

import (
	"fmt"
	"time"
	"unsafe"
)

func strerror(errCode C.int) string {
	cStr := C.fbclock_strerror(errCode)
	return C.GoString(cStr)
}

// TrueTime is a time interval we are confident the clock is right now
type TrueTime struct {
	Earliest time.Time
	Latest   time.Time
}

// FBClock wraps around fbclock C lib
type FBClock struct {
	cFBClock *C.fbclock_lib
}

// NewFBClockCustom returns new FBClock wrapper with custom path
func NewFBClockCustom(path string) (*FBClock, error) {
	cFBClock := &C.fbclock_lib{}
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	errCode := C.fbclock_init(cFBClock, cPath)
	if errCode != 0 {
		return nil, fmt.Errorf("initializing FBClock: %s", strerror(errCode))
	}
	return &FBClock{cFBClock: cFBClock}, nil
}

// NewFBClock returns new FBClock wrapper
func NewFBClock() (*FBClock, error) {
	return NewFBClockCustom(C.FBCLOCK_PATH)
}

// NewFBClockV2 returns new FBClock wrapper using v2 data structure
func NewFBClockV2() (*FBClock, error) {
	return NewFBClockCustom(C.FBCLOCK_PATH_V2)
}

// Close destroys fbclock wrapper
func (f *FBClock) Close() error {
	errCode := C.fbclock_destroy(f.cFBClock)
	if errCode != 0 {
		return fmt.Errorf("destroying FBClock: %s", strerror(errCode))
	}
	return nil
}

// GetTime returns TrueTime
func (f *FBClock) GetTime() (*TrueTime, error) {
	tt := &C.fbclock_truetime{}
	errCode := C.fbclock_gettime(f.cFBClock, tt)
	if errCode != 0 {
		return nil, fmt.Errorf("reading FBClock TrueTime: %s", strerror(errCode))
	}

	earliest := time.Unix(0, int64(tt.earliest_ns))
	latest := time.Unix(0, int64(tt.latest_ns))

	return &TrueTime{Earliest: earliest, Latest: latest}, nil
}

// GetTimeUTC returns TrueTime in UTC
func (f *FBClock) GetTimeUTC() (*TrueTime, error) {
	tt := &C.fbclock_truetime{}
	errCode := C.fbclock_gettime_utc(f.cFBClock, tt)
	if errCode != 0 {
		return nil, fmt.Errorf("reading FBClock TrueTime UTC: %s", strerror(errCode))
	}

	earliest := time.Unix(0, int64(tt.earliest_ns))
	latest := time.Unix(0, int64(tt.latest_ns))

	return &TrueTime{Earliest: earliest, Latest: latest}, nil
}
