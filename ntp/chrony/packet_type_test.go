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

package chrony

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPacketTypeStringCoversAllValues(t *testing.T) {
	// Verify all defined packet types produce non-generic output
	// and that undefined types produce distinguishable "unknown" format
	defined := []PacketType{pktTypeCmdRequest, pktTypeCmdReply}
	seen := map[string]bool{}
	for _, pt := range defined {
		s := pt.String()
		require.NotContains(t, s, "unknown", "defined PacketType %d should not be unknown", pt)
		require.False(t, seen[s], "duplicate String() for different PacketType values")
		seen[s] = true
	}

	// undefined type must include the numeric value for debugging
	undefined := PacketType(99)
	s := undefined.String()
	require.Contains(t, s, fmt.Sprintf("%d", 99))
}
