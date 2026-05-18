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

package hostendian

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

func TestByteOrderMatchesHardware(t *testing.T) {
	// Verify Order matches actual hardware byte layout by inspecting
	// how a multi-byte value is stored in memory
	var val uint16 = 0x0102
	bytes := (*[2]byte)(unsafe.Pointer(&val))

	if bytes[0] == 0x01 {
		// big endian: MSB first in memory
		require.True(t, IsBigEndian)
	} else {
		// little endian: LSB first in memory
		require.False(t, IsBigEndian)
		require.Equal(t, byte(0x02), bytes[0])
	}
}

func TestOrderCanEncodeDecode(t *testing.T) {
	var val uint32 = 0xDEADBEEF
	buf := make([]byte, 4)
	Order.PutUint32(buf, val)
	got := Order.Uint32(buf)
	require.Equal(t, val, got)
}

func TestOrderProducesNativeLayout(t *testing.T) {
	// Verify that encoding with Order produces bytes matching native memory layout
	var val uint32 = 0x01020304
	nativeBytes := (*[4]byte)(unsafe.Pointer(&val))

	encoded := make([]byte, 4)
	Order.PutUint32(encoded, val)
	require.Equal(t, nativeBytes[:], encoded)
}
