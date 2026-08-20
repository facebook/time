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

package aessiv

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/subtle"
	"fmt"
)

const (
	mul = 0x87       // the generator of GF(2^128)
	pad = byte(0x80) // the RFC 4493 padding bit
)

// cmac implements AES-CMAC as defined in RFC 4493. It is a port of tink's
// internal/mac/aescmac, which cannot be imported from outside tink, kept close
// to it so the AES-SIV built on top matches byte for byte.
type cmac struct {
	bc cipher.Block
	// k1 and k2 are the RFC 4493 section 2.3 subkeys, selected by whether the
	// message ends on a block boundary.
	k1, k2 [BlockSize]byte
}

// mulByX multiplies an element in GF(2^128) by its generator, the subkey
// derivation of RFC 4493 section 2.3.
func mulByX(block []byte) {
	// Left shift by one, then XOR in the generator if the top bit was set.
	// ConstantTimeSelect keeps that conditional off the branch predictor.
	v := int(block[0] >> 7)
	for i := range BlockSize - 1 {
		block[i] = block[i]<<1 | block[i+1]>>7
	}
	// ConstantTimeSelect returns mul or 0, so the conversion cannot overflow.
	reduce := byte(subtle.ConstantTimeSelect(v, mul, 0x00)) //nolint:gosec // G115: value is 0x87 or 0
	block[BlockSize-1] = block[BlockSize-1]<<1 ^ reduce
}

// newCMAC returns a CMAC keyed with a 16-, 24- or 32-octet AES key.
func newCMAC(key []byte) (*cmac, error) {
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, fmt.Errorf("aessiv: invalid cmac key size; got %d, want 16, 24, or 32", len(key))
	}
	bc, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aessiv: could not obtain cipher: %w", err)
	}
	c := &cmac{bc: bc}
	var zeroKeyBlock [BlockSize]byte
	c.bc.Encrypt(c.k1[:], zeroKeyBlock[:])
	mulByX(c.k1[:])
	copy(c.k2[:], c.k1[:])
	mulByX(c.k2[:])
	return c, nil
}

// compute writes AES-CMAC(data) into out, which doubles as the chaining value
// so no extra buffer escapes. Its timing depends only on len(data).
func (c *cmac) compute(out *[BlockSize]byte, data []byte) {
	numBlocksButLast := len(data) / BlockSize
	// This condition depends only on len(data).
	if len(data) > 0 && len(data)%BlockSize == 0 {
		numBlocksButLast--
	}

	clear(out[:])
	// Process M_1 ... M_(n-1), whatever the length of the last block.
	for range numBlocksButLast {
		subtle.XORBytes(out[:], data[:BlockSize], out[:])
		c.bc.Encrypt(out[:], out[:])
		data = data[BlockSize:]
	}

	// Last block M_n. Empty data simply leaves lastBlock as 100...0.
	var lastBlock [BlockSize]byte
	if len(data) == BlockSize {
		subtle.XORBytes(lastBlock[:], data, c.k1[:])
	} else {
		copy(lastBlock[:], data)
		lastBlock[len(data)] = pad
		subtle.XORBytes(lastBlock[:], lastBlock[:], c.k2[:])
	}
	subtle.XORBytes(out[:], out[:], lastBlock[:])
	c.bc.Encrypt(out[:], out[:])
}

// xorEndAndCompute writes the AES-CMAC over "data xorend last" into out,
// meaning last XORed into the final BlockSize octets of data. len(data) must be
// at least BlockSize and len(last) exactly BlockSize.
func (c *cmac) xorEndAndCompute(out *[BlockSize]byte, data, last []byte) error {
	if len(last) != BlockSize {
		return fmt.Errorf("aessiv: invalid size for last; got %d, want %d", len(last), BlockSize)
	}
	if len(data) < BlockSize {
		return fmt.Errorf("aessiv: invalid size for data; got %d, want at least %d", len(data), BlockSize)
	}

	numBlocksButLast := len(data) / BlockSize
	// This condition depends only on len(data).
	if len(data)%BlockSize == 0 {
		numBlocksButLast--
	}

	// startPos is where the portion of data to be XORed with last begins.
	startPos := len(data) - BlockSize
	clear(out[:])
	for i := range numBlocksButLast {
		subtle.XORBytes(out[:], data[:BlockSize], out[:])
		// Fold in whatever part of last overlaps this block. XORing it into the
		// chaining value is the same as XORing it into the input.
		if (i+1)*BlockSize > startPos {
			portionSize := (i+1)*BlockSize - startPos
			subtle.XORBytes(out[BlockSize-portionSize:], out[BlockSize-portionSize:], last[:portionSize])
			last = last[portionSize:]
		}
		c.bc.Encrypt(out[:], out[:])
		data = data[BlockSize:]
	}

	// Last block M_n, carrying whatever remains of last.
	var lastBlock [BlockSize]byte
	subtle.XORBytes(lastBlock[:], data, last)
	if len(data) == BlockSize {
		subtle.XORBytes(lastBlock[:], lastBlock[:], c.k1[:])
	} else {
		lastBlock[len(data)] = pad
		subtle.XORBytes(lastBlock[:], lastBlock[:], c.k2[:])
	}
	subtle.XORBytes(out[:], out[:], lastBlock[:])
	c.bc.Encrypt(out[:], out[:])
	return nil
}
