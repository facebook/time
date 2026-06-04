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
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScannerUBXFrame(t *testing.T) {
	payload := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	frame := buildUBXFrame(UBXMsgSFRBX, payload)

	s := NewMixedScanner(bytes.NewReader(frame))
	require.True(t, s.Scan())
	require.Equal(t, MsgUBX, s.Type())
	require.Equal(t, UBXClassRXM, s.UBXClass())
	require.Equal(t, UBXMsgSFRBX, s.UBXMsgID())
	require.Equal(t, payload, s.UBXPayload())

	require.False(t, s.Scan())
	require.NoError(t, s.Err())
}

func TestScannerMixedUBXAndRTCM(t *testing.T) {
	rtcmA := Encode1033(AntennaDescriptor{StationID: 1, AntennaType: "A"})
	ubx := buildUBXFrame(UBXMsgRAWX, []byte{1, 2, 3, 4, 5})
	rtcmB := Encode1033(AntennaDescriptor{StationID: 2, AntennaType: "BB"})

	var buf bytes.Buffer
	buf.Write(rtcmA)
	buf.Write(ubx)
	buf.Write(rtcmB)

	s := NewMixedScanner(&buf)

	require.True(t, s.Scan())
	require.Equal(t, MsgRTCM3, s.Type())
	require.Equal(t, uint16(1033), s.Frame().MessageType)

	require.True(t, s.Scan())
	require.Equal(t, MsgUBX, s.Type())
	require.Equal(t, UBXClassRXM, s.UBXClass())
	require.Equal(t, UBXMsgRAWX, s.UBXMsgID())
	require.Equal(t, []byte{1, 2, 3, 4, 5}, s.UBXPayload())

	require.True(t, s.Scan())
	require.Equal(t, MsgRTCM3, s.Type())
	require.Equal(t, uint16(1033), s.Frame().MessageType)

	require.False(t, s.Scan())
	require.NoError(t, s.Err())
}

func TestScannerUBXBadChecksumResyncs(t *testing.T) {
	bad := buildUBXFrame(UBXMsgRAWX, []byte{0x11, 0x22, 0x33})
	bad[len(bad)-1] ^= 0xFF // corrupt the checksum
	good := Encode1033(AntennaDescriptor{StationID: 3, AntennaType: "CCC"})

	var buf bytes.Buffer
	buf.Write(bad)
	buf.Write(good)

	s := NewMixedScanner(&buf)
	// The corrupt UBX frame is rejected; the scanner recovers and yields the
	// following valid RTCM3 frame.
	require.True(t, s.Scan())
	require.Equal(t, MsgRTCM3, s.Type())
	require.Equal(t, uint16(1033), s.Frame().MessageType)
}
