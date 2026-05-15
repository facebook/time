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

// buildFrame constructs a valid RTCM3 frame with the given payload.
func buildFrame(payload []byte) []byte {
	payloadLen := len(payload)
	frame := make([]byte, HeaderSize+payloadLen+CRCSize)
	frame[0] = Preamble
	frame[1] = byte((payloadLen >> 8) & 0x03)
	frame[2] = byte(payloadLen & 0xFF)
	copy(frame[HeaderSize:], payload)

	crc := CRC24Q(frame[:HeaderSize+payloadLen])
	frame[HeaderSize+payloadLen] = byte(crc >> 16)
	frame[HeaderSize+payloadLen+1] = byte(crc >> 8)
	frame[HeaderSize+payloadLen+2] = byte(crc)
	return frame
}

// buildPayloadWithType creates a payload with the given RTCM3 message type
// encoded in the first 12 bits, padded to the specified total length.
func buildPayloadWithType(msgType uint16, totalLen int) []byte {
	if totalLen < 2 {
		totalLen = 2
	}
	payload := make([]byte, totalLen)
	payload[0] = byte(msgType >> 4)
	payload[1] = byte((msgType << 4) & 0xF0)
	return payload
}

func TestCRC24Q(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected uint32
	}{
		{
			name:     "empty",
			data:     []byte{},
			expected: 0x000000,
		},
		{
			name:     "single zero byte",
			data:     []byte{0x00},
			expected: 0x000000,
		},
		{
			name:     "preamble only",
			data:     []byte{0xD3},
			expected: crc24qTable[0xD3],
		},
		{
			name: "known header",
			data: []byte{0xD3, 0x00, 0x04},
			expected: func() uint32 {
				var crc uint32
				for _, b := range []byte{0xD3, 0x00, 0x04} {
					crc = (crc << 8) ^ crc24qTable[((crc>>16)^uint32(b))&0xFF]
				}
				return crc & 0xFFFFFF
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CRC24Q(tt.data)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestCRC24QRoundtrip(t *testing.T) {
	// Verify that CRC computed over header+payload matches what's in a built frame.
	payload := []byte{0x3E, 0xD0, 0x00, 0x03}
	frame := buildFrame(payload)

	crc := CRC24Q(frame[:HeaderSize+len(payload)])
	gotCRC := uint32(frame[len(frame)-3])<<16 |
		uint32(frame[len(frame)-2])<<8 |
		uint32(frame[len(frame)-1])
	require.Equal(t, crc, gotCRC)
}

func TestPayloadLen(t *testing.T) {
	tests := []struct {
		name     string
		header   []byte
		expected int
	}{
		{"zero length", []byte{0xD3, 0x00, 0x00}, 0},
		{"small length", []byte{0xD3, 0x00, 0x0A}, 10},
		{"max length", []byte{0xD3, 0x03, 0xFF}, 1023},
		{"reserved bits set", []byte{0xD3, 0xFC, 0x0A}, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PayloadLen(tt.header)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestMessageTypeFromPayload(t *testing.T) {
	tests := []struct {
		name     string
		msgType  uint16
		totalLen int
	}{
		{"StationARP", TypeStationARP, 20},
		{"GPS MSM7", TypeGPSMSM7, 100},
		{"GLONASS MSM7", TypeGLONASSMSM7, 100},
		{"Galileo MSM7", TypeGalileoMSM7, 100},
		{"BeiDou MSM7", TypeBeiDouMSM7, 100},
		{"GLONASS Biases", TypeGLONASSBiases, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := buildPayloadWithType(tt.msgType, tt.totalLen)
			got := MessageTypeFromPayload(payload)
			require.Equal(t, tt.msgType, got)
		})
	}
}

func TestParseFrameValid(t *testing.T) {
	payload := buildPayloadWithType(TypeGPSMSM7, 50)
	data := buildFrame(payload)

	frame, err := ParseFrame(data)
	require.NoError(t, err)
	require.Equal(t, TypeGPSMSM7, frame.MessageType)
	require.Equal(t, payload, frame.Payload)
	require.Equal(t, data, frame.Raw)
}

func TestParseFrameErrors(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		err  error
	}{
		{
			name: "too short",
			data: []byte{0xD3, 0x00},
			err:  ErrFrameTooShort,
		},
		{
			name: "invalid preamble",
			data: []byte{0xFF, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00},
			err:  ErrInvalidPreamble,
		},
		{
			name: "truncated payload",
			data: []byte{0xD3, 0x00, 0x0A, 0x00, 0x00}, // claims 10 bytes but only 2
			err:  ErrFrameTooShort,
		},
		{
			name: "invalid CRC",
			data: func() []byte {
				f := buildFrame([]byte{0x3E, 0xD0, 0x00, 0x03})
				f[len(f)-1] ^= 0xFF // corrupt CRC
				return f
			}(),
			err: ErrCRCMismatch,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseFrame(tt.data)
			require.ErrorIs(t, err, tt.err)
		})
	}
}

func TestParseFrameEmptyPayload(t *testing.T) {
	data := buildFrame([]byte{})
	frame, err := ParseFrame(data)
	require.NoError(t, err)
	require.Equal(t, uint16(0), frame.MessageType)
	require.Empty(t, frame.Payload)
}

func TestParseFrameMaxPayload(t *testing.T) {
	payload := make([]byte, MaxPayloadLen)
	payload[0] = 0x3E
	payload[1] = 0xD0
	data := buildFrame(payload)

	frame, err := ParseFrame(data)
	require.NoError(t, err)
	require.Len(t, frame.Payload, MaxPayloadLen)
}

func TestParseFrameExtraTrailingData(t *testing.T) {
	payload := buildPayloadWithType(TypeStationARP, 20)
	data := buildFrame(payload)
	// Append extra bytes — ParseFrame should ignore them.
	data = append(data, 0xFF, 0xFF, 0xFF)

	frame, err := ParseFrame(data)
	require.NoError(t, err)
	require.Equal(t, TypeStationARP, frame.MessageType)
	// Raw should be only the frame, not the trailing bytes.
	require.Len(t, frame.Raw, HeaderSize+20+CRCSize)
}
