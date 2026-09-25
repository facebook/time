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

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Most hosts have no PHC, and that is not an error.
func TestReadPHCWithoutDevice(t *testing.T) {
	orig := ptpDevice
	t.Cleanup(func() { ptpDevice = orig })
	ptpDevice = filepath.Join(t.TempDir(), "absent")

	require.Nil(t, readPHC())
}

// A PHC that can't be read gives no reading, so that sample has no offset_vs_phc_ms.
func TestReadPHCUnreadableDevice(t *testing.T) {
	orig := ptpDevice
	t.Cleanup(func() { ptpDevice = orig })
	ptpDevice = filepath.Join(t.TempDir(), "ptp")
	require.NoError(t, os.WriteFile(ptpDevice, nil, 0o600))

	require.Nil(t, readPHC())
}
