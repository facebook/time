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

package checker

import (
	"os"
	"testing"

	"github.com/facebook/time/ntp/chrony"

	"github.com/stretchr/testify/require"
)

func TestGetPublicServer(t *testing.T) {
	require.Equal(t, "[::1]:323", getPublicServer(flavourChrony))
	require.Equal(t, "[::1]:123", getPublicServer(flavourNTPD))
}

func TestGetPrivateServer(t *testing.T) {
	require.Equal(t, chrony.ChronySocketPath, getPrivateServer(flavourChrony))
	require.Equal(t, "[::1]:123", getPrivateServer(flavourNTPD))
}

func TestIsChronyListeningTrue(t *testing.T) {
	f, err := os.CreateTemp("", "ntpchecktest")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	content := `48958: 00000000000000000000000000000000:7A6A 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 342899484 2 000000007257aba8 0
48958: 00000000000000000000000000000000:7A6A 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 342899485 2 0000000072b96944 0
17943: 00000000000000000000000001000000:0143 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 4169423132 2 0000000075f6521c 0
48958: 00000000000000000000000000000000:7A6A 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 342899480 2 000000002e163f32 0
48958: 00000000000000000000000000000000:7A6A 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 342899481 2 00000000a6580d72 0
`
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.True(t, isChronyListening(f.Name()))
}

func TestIsChronyListeningFalse(t *testing.T) {
	f, err := os.CreateTemp("", "ntpchecktest")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	content := `48958: 00000000000000000000000000000000:7A6A 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 342899484 2 000000007257aba8 0
48958: 00000000000000000000000000000000:7A6A 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 342899485 2 0000000072b96944 0
48958: 00000000000000000000000000000000:7A6A 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 342899480 2 000000002e163f32 0
48958: 00000000000000000000000000000000:7A6A 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 342899481 2 00000000a6580d72 0
`
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.False(t, isChronyListening(f.Name()))
}

func TestIsChronyListeningErr(t *testing.T) {
	require.False(t, isChronyListening("/does/not/exist/for/sure"))
	// empty file
	f, err := os.CreateTemp("", "ntpchecktest")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	require.False(t, isChronyListening(f.Name()))
}
