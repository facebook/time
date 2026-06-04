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

func TestBitWriterReaderRoundTrip(t *testing.T) {
	type field struct {
		val   uint32
		width int
	}
	fields := []field{
		{0, 1}, {1, 1},
		{0x2, 2}, {0x5, 4}, {0x7F, 7}, {0xFF, 8},
		{0x3FF, 10}, {0xFFF, 12}, {0x1FFFF, 17},
		{0x3FFFFF, 22}, {0xFFFFFF, 24}, {0xFFFFFFFF, 32},
		{0, 30}, {123456, 20},
	}

	w := NewBitWriter(64)
	bitsWritten := 0
	for _, f := range fields {
		w.WriteBits(f.val, f.width)
		bitsWritten += f.width
	}
	require.Equal(t, bitsWritten, w.Pos(), "writer bit position")

	r := NewBitReader(w.Bytes())
	for _, f := range fields {
		got := r.ReadBits(f.width)
		require.Equalf(t, f.val, got, "round-trip width=%d", f.width)
	}
	require.Equal(t, bitsWritten, r.Pos(), "reader bit position")
}

func TestBitWriterReaderSigned(t *testing.T) {
	type field struct {
		val   int32
		width int
	}
	fields := []field{
		{0, 8}, {1, 8}, {-1, 8}, {127, 8}, {-128, 8},
		{-2097152, 22}, {2097151, 22}, // 22-bit signed extremes
		{-8388608, 24}, {8388607, 24}, // 24-bit signed extremes
		{-519, 16}, {12345, 16},
	}

	w := NewBitWriter(64)
	for _, f := range fields {
		w.WriteSignedBits(f.val, f.width)
	}

	r := NewBitReader(w.Bytes())
	for _, f := range fields {
		got := r.ReadSignedBits(f.width)
		require.Equalf(t, f.val, got, "signed round-trip width=%d", f.width)
	}
}

func TestBitReaderSkip(t *testing.T) {
	w := NewBitWriter(16)
	w.WriteBits(0xAB, 8)
	w.WriteBits(0xCD, 8)
	w.WriteBits(0xEF, 8)

	r := NewBitReader(w.Bytes())
	r.Skip(8)
	require.Equal(t, 8, r.Pos())
	require.Equal(t, uint32(0xCD), r.ReadBits(8))
	require.Equal(t, uint32(0xEF), r.ReadBits(8))
}

func TestBitWriterBytePadding(t *testing.T) {
	// 12 bits written -> 2 bytes, low 4 bits of the last byte are zero padding.
	w := NewBitWriter(4)
	w.WriteBits(0xABC, 12)
	b := w.Bytes()
	require.Len(t, b, 2)
	require.Equal(t, byte(0xAB), b[0])
	require.Equal(t, byte(0xC0), b[1]) // 0xC in high nibble, padding in low
}

func TestBitWriterEmpty(t *testing.T) {
	w := NewBitWriter(0)
	require.Equal(t, 0, w.Pos())
	require.Empty(t, w.Bytes())
}
