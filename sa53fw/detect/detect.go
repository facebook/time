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
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// VendorCelestica identifies Celestica time cards (SA53 CSAC)
	VendorCelestica = "celestica"
	// VendorOrolia identifies Orolia time cards (MRO50 atomic clock)
	VendorOrolia = "orolia"
	// VendorUnknown is returned when the vendor cannot be determined
	VendorUnknown = "unknown"

	// oroliaBoardID is the fixed board.id for Orolia time cards
	oroliaBoardID = "R4006G000103"
	// celesticaBoardID is the fixed board.id for Celestica time cards
	celesticaBoardID = "1003066C00"

	// sysfs path for timecard class devices
	timecardClassPath = "/sys/class/timecard"
)

// Result holds the outcome of time card detection
type Result struct {
	Vendor  string `json:"vendor"`
	BoardID string `json:"board_id"`
	PCIAddr string `json:"pci_addr"`
}

// IsSA5x returns true if the detected time card uses an SA5x (Celestica) oscillator
func (r *Result) IsSA5x() bool {
	return r.Vendor == VendorCelestica
}

// Timecards enumerates all time cards via /sys/class/timecard and determines
// each card's vendor by reading the board.id from devlink.
// Returns an error only if the class path is unreadable.
func Timecards() ([]*Result, error) {
	addrs, err := findTimecardPCIAddrs(timecardClassPath)
	if err != nil {
		return nil, err
	}

	results := make([]*Result, 0, len(addrs))
	for _, addr := range addrs {
		boardID, err := getDevlinkBoardID(addr)
		if err != nil {
			return nil, fmt.Errorf("card %s: %w", addr, err)
		}
		results = append(results, &Result{
			Vendor:  classifyVendor(boardID),
			BoardID: boardID,
			PCIAddr: addr,
		})
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no time card devices found under %s", timecardClassPath)
	}

	return results, nil
}

// findTimecardPCIAddrs resolves the PCI addresses of all time cards by
// following the "device" symlink under each /sys/class/timecard/ocp<n>.
func findTimecardPCIAddrs(classPath string) ([]string, error) {
	entries, err := os.ReadDir(classPath)
	if err != nil {
		return nil, fmt.Errorf("no timecard class found at %s: %w", classPath, err)
	}

	addrs := make([]string, 0, len(entries))
	for _, entry := range entries {
		deviceLink := filepath.Join(classPath, entry.Name(), "device")
		target, err := os.Readlink(deviceLink)
		if err != nil {
			continue
		}
		addrs = append(addrs, filepath.Base(target))
	}

	return addrs, nil
}

// getDevlinkBoardID runs "devlink dev info" and extracts the board.id field
func getDevlinkBoardID(pciAddr string) (string, error) {
	devPath := filepath.Join("pci", pciAddr)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "devlink", "dev", "info", devPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("devlink dev info %s failed: %w: %s", devPath, err, stderr.String())
	}

	return parseBoardID(stdout.String())
}

// parseBoardID extracts the board.id value from devlink output
func parseBoardID(output string) (string, error) {
	for line := range strings.SplitSeq(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "board.id") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				return parts[1], nil
			}
		}
	}
	return "", fmt.Errorf("board.id not found in devlink output")
}

// classifyVendor determines the vendor from the board.id string.
func classifyVendor(boardID string) string {
	switch boardID {
	case oroliaBoardID:
		return VendorOrolia
	case celesticaBoardID:
		return VendorCelestica
	default:
		return VendorUnknown
	}
}
