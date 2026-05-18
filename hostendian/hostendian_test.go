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
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOrderConsistency(t *testing.T) {
	if IsBigEndian {
		require.Equal(t, binary.BigEndian, Order)
	} else {
		require.Equal(t, binary.LittleEndian, Order)
	}
}

func TestOrderCanEncodeDecode(t *testing.T) {
	var val uint32 = 0xDEADBEEF
	buf := make([]byte, 4)
	Order.PutUint32(buf, val)
	got := Order.Uint32(buf)
	require.Equal(t, val, got)
}
