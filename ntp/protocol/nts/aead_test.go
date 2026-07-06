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

package nts

import (
	"testing"

	"github.com/facebook/time/ntp/protocol"
	"github.com/stretchr/testify/require"
)

// aeadFor builds an AEAD for the given algorithm using a deterministic,
// correctly-sized key so tests can seal and open without external key material.
func aeadFor(t *testing.T, alg protocol.AEADAlgorithm) AEAD {
	t.Helper()
	var keyLen int
	switch alg {
	case protocol.AEADAESSIVCMAC512:
		keyLen = 64
	case protocol.AEADAES128GCMSIV:
		keyLen = 16
	default:
		t.Fatal("unsupported algorithm")
	}
	key := make([]byte, keyLen)
	for i := range key {
		key[i] = byte(i + 1)
	}
	a, err := NewAEAD(alg, key)
	require.NoError(t, err)
	return a
}

// TestAEADRoundTrip checks that both supported algorithms decrypt back to the
// original plaintext when sealing and opening with matching associated data.
func TestAEADRoundTrip(t *testing.T) {
	for _, alg := range []protocol.AEADAlgorithm{protocol.AEADAESSIVCMAC512, protocol.AEADAES128GCMSIV} {
		a := aeadFor(t, alg)
		ad := []byte("associated-data")
		pt := []byte("secret-extension-fields")
		nonce, ct, err := a.Seal(ad, pt)
		require.NoError(t, err)
		got, err := a.Open(ad, nonce, ct)
		require.NoError(t, err)
		require.Equal(t, pt, got)
	}
}

// TestAEADADTamper checks that opening fails for both algorithms when the
// associated data differs from what was bound at seal time.
func TestAEADADTamper(t *testing.T) {
	for _, alg := range []protocol.AEADAlgorithm{protocol.AEADAESSIVCMAC512, protocol.AEADAES128GCMSIV} {
		a := aeadFor(t, alg)
		pt := []byte("secret-extension-fields")
		nonce, ct, err := a.Seal([]byte("associated-data"), pt)
		require.NoError(t, err)
		_, err = a.Open([]byte("tampered-data"), nonce, ct)
		require.Error(t, err)
	}
}

// TestAEADCiphertextTamper checks that opening fails for both algorithms when a
// single ciphertext byte is flipped after sealing.
func TestAEADCiphertextTamper(t *testing.T) {
	for _, alg := range []protocol.AEADAlgorithm{protocol.AEADAESSIVCMAC512, protocol.AEADAES128GCMSIV} {
		a := aeadFor(t, alg)
		ad := []byte("associated-data")
		pt := []byte("secret-extension-fields")
		nonce, ct, err := a.Seal(ad, pt)
		require.NoError(t, err)
		require.NotEmpty(t, ct)
		ct[len(ct)-1] ^= 0xFF
		_, err = a.Open(ad, nonce, ct)
		require.Error(t, err)
	}
}

// TestAEADWrongLengthNonce checks that opening rejects a nonce of the wrong
// length with ErrAEADNonceSize: SIV requires an empty nonce, while GCM-SIV
// requires a fixed-length nonce.
func TestAEADWrongLengthNonce(t *testing.T) {
	for _, alg := range []protocol.AEADAlgorithm{protocol.AEADAESSIVCMAC512, protocol.AEADAES128GCMSIV} {
		a := aeadFor(t, alg)
		ad := []byte("associated-data")
		pt := []byte("secret-extension-fields")
		nonce, ct, err := a.Seal(ad, pt)
		require.NoError(t, err)
		badNonce := append(append([]byte{}, nonce...), 0x00)
		_, err = a.Open(ad, badNonce, ct)
		require.ErrorIs(t, err, ErrAEADNonceSize)
	}
}

// TestNewAEADUnsupportedAlgorithm checks that an IANA AEAD ID that the package
// does not implement is rejected with ErrUnsupportedAlgorithm.
func TestNewAEADUnsupportedAlgorithm(t *testing.T) {
	_, err := NewAEAD(protocol.AEADAlgorithm(0xFFFF), make([]byte, 64))
	require.ErrorIs(t, err, ErrUnsupportedAlgorithm)
}

// TestNewAEADInvalidKeySize checks that each supported algorithm rejects a
// wrong-length key with ErrAEADKeySize.
func TestNewAEADInvalidKeySize(t *testing.T) {
	cases := []struct {
		name string
		alg  protocol.AEADAlgorithm
		key  []byte
	}{
		{"SIV short key", protocol.AEADAESSIVCMAC512, make([]byte, 32)},
		{"GCM-SIV short key", protocol.AEADAES128GCMSIV, make([]byte, 15)},
		{"GCM-SIV long key", protocol.AEADAES128GCMSIV, make([]byte, 32)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewAEAD(tc.alg, tc.key)
			require.ErrorIs(t, err, ErrAEADKeySize)
		})
	}
}
