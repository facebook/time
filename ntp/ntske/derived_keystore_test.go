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

package ntske

import (
	"bytes"
	"sync"
	"testing"

	"github.com/facebook/time/ntp/protocol"
	"github.com/stretchr/testify/require"
)

var (
	testC2S = bytes.Repeat([]byte{0x11}, 64)
	testS2C = bytes.Repeat([]byte{0x22}, 64)
)

func testMaster() []byte { return bytes.Repeat([]byte{0x2a}, masterKeyLen) }

func newDerivedKeystore(t *testing.T) *DerivedKeystore {
	t.Helper()
	ks, err := NewDerivedKeystore(DerivedKeystoreOptions{Master: testMaster()})
	require.NoError(t, err)
	return ks
}

// same (master, id) must always derive the same key.
func TestDeriveCookieKeyDeterministic(t *testing.T) {
	master := bytes.Repeat([]byte{0x01}, masterKeyMinLength)
	k1, err := deriveCookieKey(master, 42)
	require.NoError(t, err)
	k2, err := deriveCookieKey(master, 42)
	require.NoError(t, err)
	require.Equal(t, k1, k2)
}

// a different id must derive a different key.
func TestDeriveCookieKeyDistinctPerID(t *testing.T) {
	master := bytes.Repeat([]byte{0x01}, masterKeyMinLength)
	k1, err := deriveCookieKey(master, 1)
	require.NoError(t, err)
	k2, err := deriveCookieKey(master, 2)
	require.NoError(t, err)
	require.NotEqual(t, k1, k2)
}

// a different master must derive a different key.
func TestDeriveCookieKeyDistinctPerMaster(t *testing.T) {
	k1, err := deriveCookieKey(bytes.Repeat([]byte{0x01}, masterKeyMinLength), 7)
	require.NoError(t, err)
	k2, err := deriveCookieKey(bytes.Repeat([]byte{0x02}, masterKeyMinLength), 7)
	require.NoError(t, err)
	require.NotEqual(t, k1, k2)
}

// output must be masterKeyLen octets (feeds AES-SIV-CMAC-512).
func TestDeriveCookieKeyOutputLength(t *testing.T) {
	k, err := deriveCookieKey(bytes.Repeat([]byte{0x01}, masterKeyMinLength), 0)
	require.NoError(t, err)
	require.Len(t, k, masterKeyLen)
}

// a master under the minimum length must be rejected.
func TestDeriveCookieKeyShortMaster(t *testing.T) {
	_, err := deriveCookieKey(bytes.Repeat([]byte{0x01}, masterKeyMinLength-1), 0)
	require.ErrorIs(t, err, ErrMasterKeyTooShort)
}

// core claim: a cookie sealed by one keystore opens in another sharing only the master.
func TestDerivedKeystoreCrossProcessRoundTrip(t *testing.T) {
	cookie, err := newDerivedKeystore(t).SealCookie(protocol.AEADAESSIVCMAC512, testC2S, testS2C)
	require.NoError(t, err)
	aeadID, c2s, s2c, err := newDerivedKeystore(t).OpenCookie(cookie)
	require.NoError(t, err)
	require.Equal(t, protocol.AEADAESSIVCMAC512, aeadID)
	require.Equal(t, testC2S, c2s)
	require.Equal(t, testS2C, s2c)
}

// cookie length must be unchanged from InMemoryKeystore (session algorithm inferred from it).
func TestDerivedKeystoreCookieLengths(t *testing.T) {
	ks := newDerivedKeystore(t)
	for _, tc := range []struct {
		aead            protocol.AEADAlgorithm
		keyLen, wantLen int
	}{
		{protocol.AEADAES128GCMSIV, 16, 68},
		{protocol.AEADAESSIVCMAC512, 64, 164},
	} {
		cookie, err := ks.SealCookie(tc.aead, bytes.Repeat([]byte{1}, tc.keyLen), bytes.Repeat([]byte{2}, tc.keyLen))
		require.NoError(t, err)
		require.Len(t, cookie, tc.wantLen)
	}
}

// a cookie sealed under a different master must fail verification.
func TestDerivedKeystoreWrongMaster(t *testing.T) {
	cookie, err := newDerivedKeystore(t).SealCookie(protocol.AEADAESSIVCMAC512, testC2S, testS2C)
	require.NoError(t, err)
	opener, err := NewDerivedKeystore(DerivedKeystoreOptions{Master: bytes.Repeat([]byte{0x99}, masterKeyLen)})
	require.NoError(t, err)
	_, _, _, err = opener.OpenCookie(cookie)
	require.ErrorIs(t, err, ErrCookieVerify)
}

// tampering with the key id, nonce, or ciphertext must fail verification.
func TestDerivedKeystoreTamper(t *testing.T) {
	ks := newDerivedKeystore(t)
	cookie, err := ks.SealCookie(protocol.AEADAESSIVCMAC512, testC2S, testS2C)
	require.NoError(t, err)
	for _, tc := range []struct {
		name string
		pos  int
	}{
		{"key id", 0},
		{"nonce", cookieKeyIDLen},
		{"ciphertext", len(cookie) - 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := bytes.Clone(cookie)
			bad[tc.pos] ^= 0xff
			_, _, _, err := ks.OpenCookie(bad)
			require.ErrorIs(t, err, ErrCookieVerify)
		})
	}
}

// constructing with a master under the minimum length must be rejected.
func TestNewDerivedKeystoreShortMaster(t *testing.T) {
	_, err := NewDerivedKeystore(DerivedKeystoreOptions{Master: bytes.Repeat([]byte{1}, masterKeyMinLength-1)})
	require.ErrorIs(t, err, ErrMasterKeyTooShort)
}

// concurrent seal+open must be race-free (no shared mutable state). Each goroutine
// writes to its own slot; results are checked with require after wg.Wait().
func TestDerivedKeystoreConcurrent(t *testing.T) {
	ks := newDerivedKeystore(t)
	const n = 50
	errs := make([]error, n)
	gotC2S := make([][]byte, n)
	gotS2C := make([][]byte, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			cookie, err := ks.SealCookie(protocol.AEADAESSIVCMAC512, testC2S, testS2C)
			if err != nil {
				errs[i] = err
				return
			}
			_, gotC2S[i], gotS2C[i], errs[i] = ks.OpenCookie(cookie)
		})
	}
	wg.Wait()
	for i := range n {
		require.NoError(t, errs[i])
		require.Equal(t, testC2S, gotC2S[i])
		require.Equal(t, testS2C, gotS2C[i])
	}
}
