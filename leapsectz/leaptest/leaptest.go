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

// Package leaptest points leapsectz at a temporary leap second table so that
// tests describe the leap data they need instead of depending on the host
// timezone database. The table is process-wide state, so tests using it must
// not call t.Parallel.
package leaptest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/facebook/time/leapsectz"
)

const (
	// elapsedLeapSecond dates a record at 2017-01-01, when the most recent
	// real leap second was inserted.
	elapsedLeapSecond = 1483228826
	// unreachedLeapSecond dates a record at 2100-01-01, far enough ahead to
	// stay in the future for as long as this code is plausibly running.
	unreachedLeapSecond = 4102444800
)

// Use makes leapsectz.Latest report nleap leap seconds.
func Use(tb testing.TB, nleap int32) {
	tb.Helper()
	write(tb, elapsedLeapSecond, nleap)
}

// UseFutureOnly makes leapsectz.Latest report no leap seconds at all, without
// an error: it only considers records dated in the past, and this table has
// none. Callers adding the 10s pre-1972 TAI-UTC offset then see a bare 10s.
func UseFutureOnly(tb testing.TB) {
	tb.Helper()
	write(tb, unreachedLeapSecond, 1)
}

// UseUnreadable points leapsectz at a path that does not exist, so every read
// of the leap second table fails.
func UseUnreadable(tb testing.TB) {
	tb.Helper()
	use(tb, filepath.Join(tb.TempDir(), "absent"))
}

func write(tb testing.TB, tleap uint64, nleap int32) {
	tb.Helper()
	f, err := os.CreateTemp(tb.TempDir(), "leaptest-")
	if err != nil {
		tb.Fatalf("create leap second table: %v", err)
	}
	if err := leapsectz.Write(f, '2', []leapsectz.LeapSecond{{Tleap: tleap, Nleap: nleap}}, "UTC"); err != nil {
		tb.Fatalf("write leap second table: %v", err)
	}
	if err := f.Close(); err != nil {
		tb.Fatalf("close leap second table: %v", err)
	}

	use(tb, f.Name())
}

func use(tb testing.TB, path string) {
	tb.Helper()
	previous := leapsectz.LeapFile()
	leapsectz.SetLeapFile(path)
	tb.Cleanup(func() { leapsectz.SetLeapFile(previous) })
}
