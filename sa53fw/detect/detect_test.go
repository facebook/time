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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyVendorOrolia(t *testing.T) {
	require.Equal(t, VendorOrolia, classifyVendor("R4006G000103"))
}

func TestClassifyVendorCelestica(t *testing.T) {
	require.Equal(t, VendorCelestica, classifyVendor("1003066C00"))
}

func TestClassifyVendorUnknown(t *testing.T) {
	require.Equal(t, VendorUnknown, classifyVendor(""))
	require.Equal(t, VendorUnknown, classifyVendor("XYZ123"))
	require.Equal(t, VendorUnknown, classifyVendor("R3006G000100"))
}

func TestParseBoardIDValid(t *testing.T) {
	output := `pci/0000:11:00.0:
  driver ptp_ocp
  serial_number 3d:00:00:0f:9a:7c
  versions:
      fixed:
        board.id R4006G000103
      running:
        fw 1.14`

	boardID, err := parseBoardID(output)
	require.NoError(t, err)
	require.Equal(t, "R4006G000103", boardID)
}

func TestParseBoardIDCelestica(t *testing.T) {
	output := `pci/0000:11:00.0:
  driver ptp_ocp
  serial_number 34:31:30:39:35:37
  versions:
      fixed:
        board.id 1003066C00
      running:
        fw 2.16`

	boardID, err := parseBoardID(output)
	require.NoError(t, err)
	require.Equal(t, "1003066C00", boardID)
}

func TestParseBoardIDMissing(t *testing.T) {
	output := `pci/0000:11:00.0:
  driver ptp_ocp
  serial_number 3d:00:00:0f:9a:7c`

	_, err := parseBoardID(output)
	require.Error(t, err)
	require.Contains(t, err.Error(), "board.id not found")
}

func TestFindTimecardPCIAddrs(t *testing.T) {
	dir := t.TempDir()
	ocp0 := filepath.Join(dir, "ocp0")
	require.NoError(t, os.Mkdir(ocp0, 0o755))
	require.NoError(t, os.Symlink("../../../0000:11:00.0", filepath.Join(ocp0, "device")))

	addrs, err := findTimecardPCIAddrs(dir)
	require.NoError(t, err)
	require.Equal(t, []string{"0000:11:00.0"}, addrs)
}

func TestFindTimecardPCIAddrsMultiple(t *testing.T) {
	dir := t.TempDir()
	for i, addr := range []string{"0000:11:00.0", "0000:12:00.0"} {
		ocpDir := filepath.Join(dir, fmt.Sprintf("ocp%d", i))
		require.NoError(t, os.Mkdir(ocpDir, 0o755))
		require.NoError(t, os.Symlink("../../../"+addr, filepath.Join(ocpDir, "device")))
	}

	addrs, err := findTimecardPCIAddrs(dir)
	require.NoError(t, err)
	require.Equal(t, []string{"0000:11:00.0", "0000:12:00.0"}, addrs)
}

func TestFindTimecardPCIAddrsEmpty(t *testing.T) {
	dir := t.TempDir()
	ocpDir := filepath.Join(dir, "ocp0")
	require.NoError(t, os.Mkdir(ocpDir, 0o755))

	addrs, err := findTimecardPCIAddrs(dir)
	require.NoError(t, err)
	require.Empty(t, addrs)
}

func TestFindTimecardPCIAddrsNoClass(t *testing.T) {
	_, err := findTimecardPCIAddrs("/nonexistent/path")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no timecard class found")
}

func TestResultIsSA5x(t *testing.T) {
	celestica := &Result{Vendor: VendorCelestica, BoardID: "1003066C00", PCIAddr: "0000:11:00.0"}
	require.True(t, celestica.IsSA5x())

	orolia := &Result{Vendor: VendorOrolia, BoardID: "R4006G000103", PCIAddr: "0000:11:00.0"}
	require.False(t, orolia.IsSA5x())

	unknown := &Result{Vendor: VendorUnknown, BoardID: "", PCIAddr: ""}
	require.False(t, unknown.IsSA5x())
}
