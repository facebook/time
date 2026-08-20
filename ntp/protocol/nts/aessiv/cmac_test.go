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
	"encoding/hex"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

// mustHex decodes a test vector, failing the test if it is malformed.
func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	return b
}

// rfc4493Key and rfc4493Message are the AES-128 key and 64-octet message from
// the RFC 4493 section 4 examples, which use its first 0, 16, 40 and 64 octets.
const (
	rfc4493Key     = "2b7e151628aed2a6abf7158809cf4f3c"
	rfc4493Message = "6bc1bee22e409f96e93d7e117393172a" +
		"ae2d8a571e03ac9c9eb76fac45af8e51" +
		"30c81c46a35ce411e5fbc1191a0a52ef" +
		"f69f2445df4f9b17ad2b417be66c3710"
)

// TestCMACSubkeysRFC4493 pins the K1/K2 derivation against RFC 4493 section 4.
// Wrong subkeys still yield a self-consistent MAC, so a round trip alone would
// not catch them.
func TestCMACSubkeysRFC4493(t *testing.T) {
	c, err := newCMAC(mustHex(t, rfc4493Key))
	require.NoError(t, err)
	require.Equal(t, mustHex(t, "fbeed618357133667c85e08f7236a8de"), c.k1[:])
	require.Equal(t, mustHex(t, "f7ddac306ae266ccf90bc11ee46d513b"), c.k2[:])
}

// cmacMessageLens are the four RFC 4493 / NIST SP 800-38B example lengths:
// empty, one whole block, a partial final block, and exactly block aligned.
var cmacMessageLens = [4]int{0, 16, 40, 64}

// TestCMACKnownAnswerVectors pins compute against published vectors for all
// three AES key sizes. Only AES-128 is reachable through NewAESSIV; the other
// two guard compute itself against a key-schedule-dependent regression.
func TestCMACKnownAnswerVectors(t *testing.T) {
	msg := mustHex(t, rfc4493Message)
	cases := []struct {
		name string
		key  string
		want [4]string
	}{
		{"AES-128 (RFC 4493)", rfc4493Key, [4]string{
			"bb1d6929e95937287fa37d129b756746",
			"070a16b46b4d4144f79bdd9dd04a287c",
			"dfa66747de9ae63030ca32611497c827",
			"51f0bebf7e3b9d92fc49741779363cfe",
		}},
		{"AES-192 (NIST SP 800-38B)", "8e73b0f7da0e6452c810f32b809079e562f8ead2522c6b7b", [4]string{
			"d17ddf46adaacde531cac483de7a9367",
			"9e99a7bf31e710900662f65e617c5184",
			"8a1de5be2eb31aad089a82e6ee908b0e",
			"a1d5df0eed790f794d77589659f39a11",
		}},
		{"AES-256 (NIST SP 800-38B)", "603deb1015ca71be2b73aef0857d77811f352c073b6108d72d9810a30914dff4", [4]string{
			"028962f61b7bf89efc6b551f4667d983",
			"28a7023f452e8f82bd4bf28d8c37c35c",
			"aaf3d8f1de5640c232f5b169b9c911e6",
			"e1992190549f6ed5696a2c056c315410",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := newCMAC(mustHex(t, tc.key))
			require.NoError(t, err)
			for i, n := range cmacMessageLens {
				var got [BlockSize]byte
				c.compute(&got, msg[:n])
				require.Equal(t, mustHex(t, tc.want[i]), got[:], "message length %d", n)
			}
		})
	}
}

// TestCMACRejectsBadKeySize checks that only AES key lengths are accepted.
func TestCMACRejectsBadKeySize(t *testing.T) {
	for _, n := range []int{0, 15, 17, 23, 25, 31, 33, 64} {
		_, err := newCMAC(make([]byte, n))
		require.Error(t, err, "key size %d", n)
	}
}

// xorEndNaive materialises "data xorend last" the obvious way, by copying the
// message and XORing last into its final block.
func xorEndNaive(data, last []byte) []byte {
	out := slices.Clone(data)
	for i := range BlockSize {
		out[len(out)-BlockSize+i] ^= last[i]
	}
	return out
}

// TestCMACXorEndAndComputeMatchesNaive checks the in-place xorend against a
// materialise-then-MAC reference. The overlap arithmetic is the fiddliest part
// of the port, so it is pinned at every alignment and AES key size.
func TestCMACXorEndAndComputeMatchesNaive(t *testing.T) {
	rng := newRNG(0x5109, 0x5297)
	for _, keyLen := range []int{16, 24, 32} {
		c, err := newCMAC(randBytes(rng, keyLen))
		require.NoError(t, err)
		last := randBytes(rng, BlockSize)

		// From one block up to five, covering every offset within a block.
		for dataLen := BlockSize; dataLen <= 5*BlockSize; dataLen++ {
			data := randBytes(rng, dataLen)
			var got, want [BlockSize]byte
			require.NoError(t, c.xorEndAndCompute(&got, data, last))
			c.compute(&want, xorEndNaive(data, last))
			require.Equal(t, want, got, "keyLen=%d dataLen=%d", keyLen, dataLen)
		}
	}
}

// TestCMACXorEndAndComputeRejectsBadSizes checks the two documented
// preconditions, since s2v relies on them holding rather than re-checking.
func TestCMACXorEndAndComputeRejectsBadSizes(t *testing.T) {
	c, err := newCMAC(mustHex(t, rfc4493Key))
	require.NoError(t, err)

	var out [BlockSize]byte
	require.Error(t, c.xorEndAndCompute(&out, make([]byte, BlockSize), make([]byte, BlockSize-1)))
	require.Error(t, c.xorEndAndCompute(&out, make([]byte, BlockSize-1), make([]byte, BlockSize)))
}

// TestCMACXorEndAndComputeDoesNotMutate guards the caller's buffer: s2v runs
// over the very plaintext that Seal then encrypts.
func TestCMACXorEndAndComputeDoesNotMutate(t *testing.T) {
	c, err := newCMAC(mustHex(t, rfc4493Key))
	require.NoError(t, err)

	last := slices.Repeat([]byte{0xff}, BlockSize)
	for _, dataLen := range []int{BlockSize, BlockSize + 1, 2*BlockSize - 1, 3 * BlockSize} {
		data := make([]byte, dataLen)
		for i := range data {
			data[i] = byte(i)
		}
		original := slices.Clone(data)
		var out [BlockSize]byte
		require.NoError(t, c.xorEndAndCompute(&out, data, last))
		require.Equal(t, original, data, "dataLen=%d", dataLen)
	}
}
