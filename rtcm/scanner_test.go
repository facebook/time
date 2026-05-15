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
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScannerSingleFrame(t *testing.T) {
	payload := buildPayloadWithType(TypeGPSMSM7, 50)
	frame := buildFrame(payload)

	scanner := NewScanner(bytes.NewReader(frame))
	require.True(t, scanner.Scan())
	require.NoError(t, scanner.Err())

	f := scanner.Frame()
	require.Equal(t, TypeGPSMSM7, f.MessageType)
	require.Equal(t, frame, f.Raw)

	// No more frames.
	require.False(t, scanner.Scan())
	require.NoError(t, scanner.Err())
}

func TestScannerMultipleFrames(t *testing.T) {
	types := []uint16{TypeStationARP, TypeGPSMSM7, TypeGLONASSMSM7, TypeGalileoMSM7}
	var buf bytes.Buffer
	for _, mt := range types {
		payload := buildPayloadWithType(mt, 30)
		buf.Write(buildFrame(payload))
	}

	scanner := NewScanner(&buf)
	for i, mt := range types {
		require.True(t, scanner.Scan(), "frame %d", i)
		require.Equal(t, mt, scanner.Frame().MessageType, "frame %d", i)
	}
	require.False(t, scanner.Scan())
	require.NoError(t, scanner.Err())
}

func TestScannerGarbageBeforePreamble(t *testing.T) {
	payload := buildPayloadWithType(TypeBeiDouMSM7, 20)
	frame := buildFrame(payload)

	// Prepend garbage bytes (none of which are 0xD3).
	garbage := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xAA, 0x55}
	var buf bytes.Buffer
	buf.Write(garbage)
	buf.Write(frame)

	scanner := NewScanner(&buf)
	require.True(t, scanner.Scan())
	require.Equal(t, TypeBeiDouMSM7, scanner.Frame().MessageType)
}

func TestScannerFalsePreamble(t *testing.T) {
	// A 0xD3 byte followed by non-zero reserved bits should be skipped.
	payload := buildPayloadWithType(TypeGLONASSBiases, 10)
	validFrame := buildFrame(payload)

	var buf bytes.Buffer
	// Write a false preamble with reserved bits set.
	buf.Write([]byte{0xD3, 0xFC, 0x00})
	buf.Write(validFrame)

	scanner := NewScanner(&buf)
	require.True(t, scanner.Scan())
	require.Equal(t, TypeGLONASSBiases, scanner.Frame().MessageType)
}

func TestScannerInvalidCRCSkipsAndRecovers(t *testing.T) {
	// Build a frame with corrupted CRC, followed by a valid frame.
	badPayload := buildPayloadWithType(TypeGPSMSM7, 20)
	badFrame := buildFrame(badPayload)
	badFrame[len(badFrame)-1] ^= 0xFF // corrupt CRC

	goodPayload := buildPayloadWithType(TypeGalileoMSM7, 20)
	goodFrame := buildFrame(goodPayload)

	var buf bytes.Buffer
	buf.Write(badFrame)
	buf.Write(goodFrame)

	scanner := NewScanner(&buf)
	require.True(t, scanner.Scan())
	// Should skip the bad frame and read the good one.
	require.Equal(t, TypeGalileoMSM7, scanner.Frame().MessageType)
}

func TestScannerEOFMidHeader(t *testing.T) {
	// Preamble followed by only 1 byte instead of 2.
	data := []byte{0xD3, 0x00}
	scanner := NewScanner(bytes.NewReader(data))
	require.False(t, scanner.Scan())
	require.NoError(t, scanner.Err()) // EOF is not an error
}

func TestScannerEOFMidPayload(t *testing.T) {
	// Valid header claiming 10-byte payload, but only 3 bytes follow.
	data := []byte{0xD3, 0x00, 0x0A, 0x00, 0x00, 0x00}
	scanner := NewScanner(bytes.NewReader(data))
	require.False(t, scanner.Scan())
	require.NoError(t, scanner.Err())
}

func TestScannerEmptyReader(t *testing.T) {
	scanner := NewScanner(bytes.NewReader(nil))
	require.False(t, scanner.Scan())
	require.NoError(t, scanner.Err())
}

func TestScannerMaxPayloadFrame(t *testing.T) {
	payload := make([]byte, MaxPayloadLen)
	payload[0] = byte(TypeGPSMSM7 >> 4)
	payload[1] = byte((TypeGPSMSM7 << 4) & 0xF0)
	frame := buildFrame(payload)

	scanner := NewScanner(bytes.NewReader(frame))
	require.True(t, scanner.Scan())
	require.Equal(t, TypeGPSMSM7, scanner.Frame().MessageType)
	require.Len(t, scanner.Frame().Payload, MaxPayloadLen)
}

func TestScannerReadError(t *testing.T) {
	r := &errReader{err: io.ErrClosedPipe}
	scanner := NewScanner(r)
	require.False(t, scanner.Scan())
	require.ErrorIs(t, scanner.Err(), io.ErrClosedPipe)
}

func TestScannerBackToBackIdenticalFrames(t *testing.T) {
	payload := buildPayloadWithType(TypeStationARP, 19)
	frame := buildFrame(payload)

	var buf bytes.Buffer
	for range 100 {
		buf.Write(frame)
	}

	scanner := NewScanner(&buf)
	count := 0
	for scanner.Scan() {
		require.Equal(t, TypeStationARP, scanner.Frame().MessageType)
		count++
	}
	require.NoError(t, scanner.Err())
	require.Equal(t, 100, count)
}

func TestScannerAllConfiguredMessageTypes(t *testing.T) {
	// Test all message types that oscillatord outputs.
	types := []uint16{
		TypeStationARP,
		TypeGPSMSM7,
		TypeGLONASSMSM7,
		TypeGalileoMSM7,
		TypeBeiDouMSM7,
		TypeGLONASSBiases,
	}

	var buf bytes.Buffer
	for _, mt := range types {
		payload := buildPayloadWithType(mt, 50)
		buf.Write(buildFrame(payload))
	}

	scanner := NewScanner(&buf)
	for i, mt := range types {
		require.True(t, scanner.Scan(), "frame %d (type %d)", i, mt)
		require.Equal(t, mt, scanner.Frame().MessageType)
	}
	require.False(t, scanner.Scan())
	require.NoError(t, scanner.Err())
}

// errReader is a test helper that always returns an error.
type errReader struct {
	err error
}

func (r *errReader) Read([]byte) (int, error) {
	return 0, r.err
}
