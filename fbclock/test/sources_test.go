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

package test

import (
	"testing"

	lib "github.com/facebook/time/fbclock"
	"github.com/stretchr/testify/require"
)

func TestSourcesNames(t *testing.T) {
	for name, want := range map[string]lib.Sources{
		"ptp": lib.SourcePTP,
		"ntp": lib.SourceNTP,
		"any": lib.SourceAny,
	} {
		var got lib.Sources
		require.NoError(t, got.Set(name))
		require.Equal(t, want, got, name)
		require.Equal(t, name, got.String())
	}
	var s lib.Sources
	require.NoError(t, s.Set("NTP"))
	require.Equal(t, lib.SourceNTP, s)
	require.Error(t, s.Set("chrony"))
	require.Equal(t, "0x03", (lib.SourcePTP | lib.SourceNTP).String())
}

// every source, including ones added later
func TestSourceAnyIsEveryBit(t *testing.T) {
	require.Equal(t, lib.Sources(0xFF), lib.SourceAny)
}
