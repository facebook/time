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

// Package detect resolves the OCP TimeCard's u-blox GNSS receiver
// serial port from sysfs.
//
// gnssfw cares only about the GNSS receiver itself (model, firmware
// version, baudrate). It does not enumerate or classify the TimeCard
// host: vendor / board_id is the consumer's responsibility (e.g. ANR's
// timecard.py already reads board.id via `devlink dev info`).
//
// Two resolution paths are supported:
//
//   - GNSSSerial() reads /sys/class/timecard/ocp0/tty/ttyGNSS  the
//     legacy single-card fallback.
//   - GNSSSerialFromPCI(bdf) globs
//     /sys/bus/pci/devices/<bdf>/timecard/ocp*/tty/ttyGNSS so callers
//     that know the TimeCard's PCI BDF (e.g. ANR's logical_location)
//     can target a specific card on multi-TimeCard hosts.
package detect

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// TTYGNSSPath is the sysfs symlink that exposes the kernel-assigned
	// tty device for the TimeCard's u-blox GNSS receiver. The OCP TAP
	// ptp_ocp driver creates one entry per timecard under
	// /sys/class/timecard.
	TTYGNSSPath = "/sys/class/timecard/ocp0/tty/ttyGNSS"

	// DefaultGNSSSerial is the conventional fallback device name when
	// the sysfs symlink cannot be read.
	DefaultGNSSSerial = "/dev/ttyGNSS0"

	// pciDevicesRoot is the sysfs root that exposes every PCI device on
	// the host indexed by BDF.
	pciDevicesRoot = "/sys/bus/pci/devices"
)

// GNSSSerial reads the kernel-assigned tty device name for the
// TimeCard's u-blox GNSS receiver from sysfs and returns the absolute
// /dev path (e.g. "/dev/ttyGNSS0").
//
// This is the legacy single-card path; it always reads ocp0. Use
// GNSSSerialFromPCI on hosts with more than one TimeCard.
func GNSSSerial() (string, error) {
	return detectGNSSSerialFrom(TTYGNSSPath)
}

// GNSSSerialFromPCI resolves the GNSS tty for the TimeCard at the
// given PCI BDF (e.g. "0000:11:00.0"). It globs
// /sys/bus/pci/devices/<bdf>/timecard/ocp*/tty/ttyGNSS, expects exactly
// one match, reads the matched file (whose contents name a tty device
// like "ttyS4"), and returns the absolute /dev path.
//
// The single-match invariant is intentional: a given PCI device hosts
// at most one TimeCard, so more than one match indicates a kernel /
// sysfs anomaly worth surfacing.
func GNSSSerialFromPCI(bdf string) (string, error) {
	if bdf == "" {
		return "", fmt.Errorf("empty PCI BDF")
	}
	pattern := filepath.Join(pciDevicesRoot, bdf, "timecard", "ocp*", "tty", "ttyGNSS")
	return detectGNSSSerialFromGlob(pattern)
}

// detectGNSSSerialFrom is the testable variant of GNSSSerial.
func detectGNSSSerialFrom(sysfsPath string) (string, error) {
	data, err := os.ReadFile(sysfsPath)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", sysfsPath, err)
	}
	ttyName := strings.TrimSpace(string(data))
	if ttyName == "" {
		return "", fmt.Errorf("empty tty device name in %s", sysfsPath)
	}
	return filepath.Join("/dev", ttyName), nil
}

// detectGNSSSerialFromGlob is the testable variant of
// GNSSSerialFromPCI. It accepts a filepath.Glob pattern, requires
// exactly one match, and reads the matched file.
func detectGNSSSerialFromGlob(pattern string) (string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("globbing %s: %w", pattern, err)
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no GNSS tty found at %s", pattern)
	case 1:
		return detectGNSSSerialFrom(matches[0])
	default:
		return "", fmt.Errorf("expected exactly one GNSS tty match for %s, got %d: %v",
			pattern, len(matches), matches)
	}
}
