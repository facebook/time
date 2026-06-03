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

package rtcm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// stationIDOf decodes DF003 (reference station ID), payload bits 12..23.
func stationIDOf(payload []byte) uint16 {
	return uint16(payload[1]&0x0F)<<8 | uint16(payload[2])
}

func TestEncode1033RoundTrip(t *testing.T) {
	desc := AntennaDescriptor{
		StationID:    1234,
		AntennaType:  "ADVNULLANTENNA",
		ReceiverType: "u-blox F9T",
		ReceiverFW:   "2.20",
	}
	frame := Encode1033(desc)

	f, err := ParseFrame(frame) // also validates CRC-24Q
	require.NoError(t, err)
	require.Equal(t, uint16(1033), f.MessageType)
	require.Equal(t, uint16(1234), stationIDOf(f.Payload))
	require.Contains(t, string(f.Payload), "ADVNULLANTENNA")
	require.Contains(t, string(f.Payload), "u-blox F9T")
	require.Contains(t, string(f.Payload), "2.20")
}

func TestPatchStationID(t *testing.T) {
	orig := Encode1033(AntennaDescriptor{StationID: 1, AntennaType: "X"})
	patched := PatchStationID(orig, 4090)

	f, err := ParseFrame(patched) // CRC must still validate after the patch
	require.NoError(t, err)
	require.Equal(t, uint16(1033), f.MessageType)
	require.Equal(t, uint16(4090), stationIDOf(f.Payload))
	require.NotEqual(t, orig[len(orig)-3:], patched[len(patched)-3:], "CRC recomputed")
}

func TestPatchStationIDShortFrameUnchanged(t *testing.T) {
	short := []byte{0xD3, 0x00}
	require.Equal(t, short, PatchStationID(short, 5))
}

func TestPatch1005RefStation(t *testing.T) {
	// Synthetic 1005 frame: message number 1005 in the first 12 bits, 19-byte
	// payload (the size of a real 1005), reference-station indicator clear.
	payload := make([]byte, 19)
	payload[0] = byte(TypeStationARP >> 4)
	payload[1] = byte((TypeStationARP & 0x0F) << 4)
	frame := frameRTCM(payload)

	require.Equal(t, byte(0), frame[7]&0x40, "indicator initially clear")

	patched := Patch1005RefStation(frame)
	f, err := ParseFrame(patched) // CRC must validate
	require.NoError(t, err)
	require.Equal(t, TypeStationARP, f.MessageType)
	require.Equal(t, byte(0x40), patched[7]&0x40, "reference station indicator set")
}

func TestPatch1005ShortFrameUnchanged(t *testing.T) {
	short := make([]byte, 7)
	require.Equal(t, short, Patch1005RefStation(short))
}
