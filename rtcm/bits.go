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

// BitReader reads arbitrary numbers of bits from a byte slice.
type BitReader struct {
	data []byte
	pos  int // current bit position
}

// NewBitReader creates a BitReader over the given byte slice.
func NewBitReader(data []byte) *BitReader {
	return &BitReader{data: data}
}

// ReadBits reads n bits (1-32) as an unsigned value.
func (r *BitReader) ReadBits(n int) uint32 {
	var val uint32
	for range n {
		byteIdx := r.pos / 8
		bitIdx := 7 - (r.pos % 8)
		if byteIdx < len(r.data) {
			val = (val << 1) | uint32((r.data[byteIdx]>>bitIdx)&1)
		} else {
			val <<= 1
		}
		r.pos++
	}
	return val
}

// ReadSignedBits reads n bits as a signed two's complement value.
func (r *BitReader) ReadSignedBits(n int) int32 {
	val := r.ReadBits(n)
	if val&(1<<(n-1)) != 0 {
		val |= ^uint32(0) << n
	}
	//nolint:gosec // intentional two's-complement reinterpret of an n-bit field
	return int32(val)
}

// Pos returns the current bit position.
func (r *BitReader) Pos() int {
	return r.pos
}

// Skip advances the position by n bits.
func (r *BitReader) Skip(n int) {
	r.pos += n
}

// BitWriter writes arbitrary numbers of bits to a byte slice.
type BitWriter struct {
	data []byte
	pos  int
}

// NewBitWriter creates a BitWriter with the given initial capacity in bytes.
func NewBitWriter(capacity int) *BitWriter {
	return &BitWriter{data: make([]byte, capacity)}
}

// WriteBits writes n bits (1-32) from the least significant bits of val.
func (w *BitWriter) WriteBits(val uint32, n int) {
	for i := n - 1; i >= 0; i-- {
		byteIdx := w.pos / 8
		bitIdx := 7 - (w.pos % 8)
		for byteIdx >= len(w.data) {
			w.data = append(w.data, 0)
		}
		if (val>>i)&1 == 1 {
			w.data[byteIdx] |= 1 << bitIdx
		}
		w.pos++
	}
}

// WriteSignedBits writes n bits of a signed value in two's complement.
func (w *BitWriter) WriteSignedBits(val int32, n int) {
	//nolint:gosec // intentional two's-complement reinterpret into an n-bit field
	w.WriteBits(uint32(val)&((1<<n)-1), n)
}

// Pos returns the current bit position.
func (w *BitWriter) Pos() int {
	return w.pos
}

// Bytes returns the written data, trimmed to the byte boundary.
func (w *BitWriter) Bytes() []byte {
	byteLen := (w.pos + 7) / 8
	if byteLen > len(w.data) {
		return w.data
	}
	return w.data[:byteLen]
}
