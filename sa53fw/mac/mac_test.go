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

package mac

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFirmwareVer(t *testing.T) {
	f := &Mac{}

	err := f.parseMacFirmware("[=V1.0.4.0.5ADA4E31,V1.0]\r\n")

	require.NoError(t, err)
	require.Equal(t, 1, f.fwMajor)
	require.Equal(t, 0, f.fwMinor)
	require.Equal(t, 4, f.fwPatch)
	require.Equal(t, "5ADA4E31", f.fwCommit)

	require.Equal(t, "V1.0.4", f.FormatFWVersion())
	require.Equal(t, 0x10004, f.Version())
}
