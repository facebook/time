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

package firmware

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFirmwareVer(t *testing.T) {
	f := &Firmware{}

	err := f.parseFooter("?v=V1.1.30.635AC64C,f=sa5x_V1.1.30.635AC64C.cfw,t=sa5x,app=clock,platform=sa5x,target=cpu.")

	require.NoError(t, err)
	require.Equal(t, 1, f.fwMajor)
	require.Equal(t, 1, f.fwMinor)
	require.Equal(t, 30, f.fwPatch)
	require.Equal(t, "635AC64C", f.fwCommit)

	require.Equal(t, "V1.1.30", f.FormatFWVersion())
	require.Equal(t, 0x1011E, f.Version())
}
