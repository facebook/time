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

package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// --- detectGNSSSerialFrom (single-file reader) ---

func TestDetectGNSSSerialFromValid(t *testing.T) {
	dir := t.TempDir()
	sysfsFile := filepath.Join(dir, "ttyGNSS")
	require.NoError(t, os.WriteFile(sysfsFile, []byte("ttyGNSS0\n"), 0o644))

	got, err := detectGNSSSerialFrom(sysfsFile)
	require.NoError(t, err)
	require.Equal(t, "/dev/ttyGNSS0", got)
}

func TestDetectGNSSSerialFromTrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	sysfsFile := filepath.Join(dir, "ttyGNSS")
	require.NoError(t, os.WriteFile(sysfsFile, []byte("  ttyS7\t\n"), 0o644))

	got, err := detectGNSSSerialFrom(sysfsFile)
	require.NoError(t, err)
	require.Equal(t, "/dev/ttyS7", got)
}

func TestDetectGNSSSerialFromMissingFile(t *testing.T) {
	_, err := detectGNSSSerialFrom("/nonexistent/sysfs/ttyGNSS")
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot read")
}

func TestDetectGNSSSerialFromEmptyFile(t *testing.T) {
	dir := t.TempDir()
	sysfsFile := filepath.Join(dir, "ttyGNSS")
	require.NoError(t, os.WriteFile(sysfsFile, []byte("\n"), 0o644))

	_, err := detectGNSSSerialFrom(sysfsFile)
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty tty device name")
}

// --- detectGNSSSerialFromGlob (PCI BDF resolver) ---

// makeFakeTimecardSysfs creates a sysfs-shaped layout under root mimicking
// /sys/bus/pci/devices/<bdf>/timecard/<ocp>/tty/ttyGNSS = <ttyName>.
// Returns the absolute path to the ttyGNSS file it created.
func makeFakeTimecardSysfs(t *testing.T, root, bdf, ocp, ttyName string) string {
	t.Helper()
	dir := filepath.Join(root, bdf, "timecard", ocp, "tty")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	file := filepath.Join(dir, "ttyGNSS")
	require.NoError(t, os.WriteFile(file, []byte(ttyName+"\n"), 0o644))
	return file
}

func TestDetectGNSSSerialFromGlobSingleCard(t *testing.T) {
	root := t.TempDir()
	makeFakeTimecardSysfs(t, root, "0000:11:00.0", "ocp0", "ttyS4")

	pattern := filepath.Join(root, "0000:11:00.0", "timecard", "ocp*", "tty", "ttyGNSS")
	got, err := detectGNSSSerialFromGlob(pattern)
	require.NoError(t, err)
	require.Equal(t, "/dev/ttyS4", got)
}

func TestDetectGNSSSerialFromGlobMultipleCardsPicksRightOne(t *testing.T) {
	// Two TimeCards on the host, each at a different PCI BDF. Globbing
	// against the second BDF must return only the second card's tty.
	root := t.TempDir()
	makeFakeTimecardSysfs(t, root, "0000:11:00.0", "ocp0", "ttyS4")
	makeFakeTimecardSysfs(t, root, "0000:21:00.0", "ocp1", "ttyS9")

	pattern := filepath.Join(root, "0000:21:00.0", "timecard", "ocp*", "tty", "ttyGNSS")
	got, err := detectGNSSSerialFromGlob(pattern)
	require.NoError(t, err)
	require.Equal(t, "/dev/ttyS9", got)
}

func TestDetectGNSSSerialFromGlobNoMatch(t *testing.T) {
	root := t.TempDir()
	pattern := filepath.Join(root, "0000:11:00.0", "timecard", "ocp*", "tty", "ttyGNSS")

	_, err := detectGNSSSerialFromGlob(pattern)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no GNSS tty found")
}

func TestDetectGNSSSerialFromGlobMultipleMatchesIsError(t *testing.T) {
	// Defensive: a single PCI BDF should never expose two TimeCards
	// simultaneously. Surface this as an error rather than silently
	// picking the first match.
	root := t.TempDir()
	bdf := "0000:11:00.0"
	makeFakeTimecardSysfs(t, root, bdf, "ocp0", "ttyS4")
	makeFakeTimecardSysfs(t, root, bdf, "ocp1", "ttyS5")

	pattern := filepath.Join(root, bdf, "timecard", "ocp*", "tty", "ttyGNSS")
	_, err := detectGNSSSerialFromGlob(pattern)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected exactly one")
}

// --- GNSSSerialFromPCI (public wrapper) ---

func TestGNSSSerialFromPCIEmptyBDF(t *testing.T) {
	_, err := GNSSSerialFromPCI("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty PCI BDF")
}
