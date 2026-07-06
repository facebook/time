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
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/facebook/time/ntp/protocol"
	"github.com/stretchr/testify/require"
	"github.com/tink-crypto/tink-go/v2/daead/subtle"
)

// newTestAEAD returns an AEAD backed by AES-SIV-CMAC-512 for authenticator tests.
// Delegates to aeadFor defined in aead_test.go to avoid duplicating test helper logic
// across test files in package nts.
func newTestAEAD(t *testing.T) AEAD {
	t.Helper()
	return aeadFor(t, protocol.AEADAESSIVCMAC512)
}

// newTestAEADWithAlg builds an AEAD for the given algorithm using a deterministic key.
// Consolidated to use the single aeadFor helper from aead_test.go to keep algorithm
// support in sync across test files.
func newTestAEADWithAlg(t *testing.T, alg protocol.AEADAlgorithm) AEAD {
	t.Helper()
	return aeadFor(t, alg)
}

func TestMarshalAuthenticatorBodyEmptyNonce(t *testing.T) {
	body, err := MarshalAuthenticatorBody(nil, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	require.NoError(t, err)
	// 4-byte header + 0 nonce + 8 ciphertext = 12 octets.
	require.Len(t, body, 12)
	// Nonce length = 0, ciphertext length = 8.
	require.Equal(t, []byte{0x00, 0x00, 0x00, 0x08}, body[0:4])
	require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}, body[4:])
}

func TestMarshalAuthenticatorBodyNoncePadding(t *testing.T) {
	// 5-byte nonce should pad to 8; 7-byte ciphertext should pad to 8.
	body, err := MarshalAuthenticatorBody([]byte{1, 2, 3, 4, 5}, []byte{0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0x10})
	require.NoError(t, err)
	require.Len(t, body, 4+8+8)
	require.Equal(t, []byte{0x00, 0x05, 0x00, 0x07}, body[0:4])
	require.Equal(t, []byte{1, 2, 3, 4, 5, 0, 0, 0}, body[4:12])
	require.Equal(t, []byte{0xa, 0xb, 0xc, 0xd, 0xe, 0xf, 0x10, 0x00}, body[12:20])
}

func TestParseAuthenticatorBodyRoundTrip(t *testing.T) {
	nonce := []byte{1, 2, 3}
	ciphertext := []byte{0xa, 0xb, 0xc, 0xd, 0xe}
	body, err := MarshalAuthenticatorBody(nonce, ciphertext)
	require.NoError(t, err)
	parsed, err := ParseAuthenticatorBody(body)
	require.NoError(t, err)
	require.Equal(t, nonce, parsed.Nonce)
	require.Equal(t, ciphertext, parsed.Ciphertext)
}

func TestParseAuthenticatorBodyRejectsTruncatedHeader(t *testing.T) {
	_, err := ParseAuthenticatorBody([]byte{0x00, 0x00, 0x00})
	require.ErrorIs(t, err, ErrAuthenticatorTruncated)
}

func TestParseAuthenticatorBodyRejectsLengthExceedingBuffer(t *testing.T) {
	// Claims 100 bytes of ciphertext but body is only 4 + 0 + 4 = 8 octets.
	body := []byte{0x00, 0x00, 0x00, 0x64, 0, 0, 0, 0}
	_, err := ParseAuthenticatorBody(body)
	require.ErrorIs(t, err, ErrAuthenticatorTruncated)
}

func TestSealOpenAuthenticatorEmptyPlaintext(t *testing.T) {
	siv := newTestAEAD(t)
	ad := []byte("some-associated-data-from-the-ntp-packet")

	ef, err := SealAuthenticator(siv, ad, nil)
	require.NoError(t, err)
	require.Equal(t, protocol.NTSAuthenticator, ef.Type)

	pt, err := OpenAuthenticator(siv, ad, ef)
	require.NoError(t, err)
	require.Empty(t, pt)
}

func TestSealOpenAuthenticatorWithPlaintext(t *testing.T) {
	siv := newTestAEAD(t)
	ad := []byte("ntp-header-and-leading-extension-fields")
	plaintext := []byte("encrypted-extension-field-payload")

	ef, err := SealAuthenticator(siv, ad, plaintext)
	require.NoError(t, err)

	got, err := OpenAuthenticator(siv, ad, ef)
	require.NoError(t, err)
	require.Equal(t, plaintext, got)
}

func TestOpenAuthenticatorRejectsTamperedAD(t *testing.T) {
	siv := newTestAEAD(t)
	ad := []byte("authentic-associated-data")

	ef, err := SealAuthenticator(siv, ad, nil)
	require.NoError(t, err)

	tampered := bytes.Clone(ad)
	tampered[0] ^= 0xff
	_, err = OpenAuthenticator(siv, tampered, ef)
	require.ErrorIs(t, err, ErrAuthenticatorVerify)
}

func TestOpenAuthenticatorRejectsTamperedCiphertext(t *testing.T) {
	siv := newTestAEAD(t)
	ad := []byte("ad")

	ef, err := SealAuthenticator(siv, ad, []byte("plaintext"))
	require.NoError(t, err)

	// Flip the first ciphertext byte (index 4 = right after the 4-byte header).
	// Must avoid trailing padding bytes which the parser strips before SIV verifies.
	tampered := protocol.ExtensionField{Type: ef.Type, Body: bytes.Clone(ef.Body)}
	tampered.Body[authenticatorHeaderLen] ^= 0xff
	_, err = OpenAuthenticator(siv, ad, tampered)
	require.ErrorIs(t, err, ErrAuthenticatorVerify)
}

// TestOpenAuthenticatorVerifyChainTerminatesAtSentinel locks in the
// ErrAuthenticatorVerify contract: the tink error from a failed SIV decrypt
// MUST NOT appear in the errors.Is chain. Only the sentinel should be
// unwrappable. Guards against a future regression where someone (or a
// linter) changes the wrap from "%w: %s" to "%w: %w" and silently puts
// tink's err back in the chain.
func TestOpenAuthenticatorVerifyChainTerminatesAtSentinel(t *testing.T) {
	siv := newTestAEAD(t)
	ad := []byte("ad")

	ef, err := SealAuthenticator(siv, ad, []byte("plaintext"))
	require.NoError(t, err)

	tampered := protocol.ExtensionField{Type: ef.Type, Body: bytes.Clone(ef.Body)}
	tampered.Body[authenticatorHeaderLen] ^= 0xff
	_, err = OpenAuthenticator(siv, ad, tampered)
	require.Error(t, err)

	// Unwrap once should yield the sentinel; unwrap again should yield nil
	// (chain terminates). If someone re-introduces %w on the tink err, the
	// second Unwrap would return a non-nil tink error and this test fails.
	require.Equal(t, ErrAuthenticatorVerify, errors.Unwrap(err),
		"first unwrap should be ErrAuthenticatorVerify")
	require.Nil(t, errors.Unwrap(ErrAuthenticatorVerify),
		"chain must terminate at sentinel — no tink err in chain")
}

func TestOpenAuthenticatorRejectsWrongType(t *testing.T) {
	siv := newTestAEAD(t)
	bogus := protocol.ExtensionField{Type: protocol.UniqueIdentifier, Body: make([]byte, 32)}
	_, err := OpenAuthenticator(siv, []byte("ad"), bogus)
	require.ErrorIs(t, err, ErrAuthenticatorMalformed)
}

func TestOpenAuthenticatorRejectsNonEmptyNonce(t *testing.T) {
	siv := newTestAEAD(t)
	body, err := MarshalAuthenticatorBody([]byte{1, 2, 3, 4}, []byte{5, 6, 7, 8})
	require.NoError(t, err)
	ef := protocol.ExtensionField{Type: protocol.NTSAuthenticator, Body: body}
	_, err = OpenAuthenticator(siv, []byte("ad"), ef)
	require.ErrorIs(t, err, ErrAuthenticatorMalformed)
}

// TestSealOpenAuthenticatorGCMSIV exercises the non-deterministic GCM-SIV path
// through the authenticator: unlike SIV, GCM-SIV emits a non-empty nonce that
// must survive marshalling into and parsing back out of the EF body.
func TestSealOpenAuthenticatorGCMSIV(t *testing.T) {
	aead := newTestAEADWithAlg(t, protocol.AEADAES128GCMSIV)
	ad := []byte("ntp-header-and-leading-extension-fields")
	plaintext := []byte("encrypted-extension-field-payload")

	ef, err := SealAuthenticator(aead, ad, plaintext)
	require.NoError(t, err)
	require.Equal(t, protocol.NTSAuthenticator, ef.Type)

	got, err := OpenAuthenticator(aead, ad, ef)
	require.NoError(t, err)
	require.Equal(t, plaintext, got)
}

// TestOpenAuthenticatorGCMSIVRejectsTamperedAD checks that the GCM-SIV path
// fails verification when the associated data is altered after sealing.
func TestOpenAuthenticatorGCMSIVRejectsTamperedAD(t *testing.T) {
	aead := newTestAEADWithAlg(t, protocol.AEADAES128GCMSIV)
	ad := []byte("authentic-associated-data")

	ef, err := SealAuthenticator(aead, ad, []byte("plaintext"))
	require.NoError(t, err)

	tampered := bytes.Clone(ad)
	tampered[0] ^= 0xff
	_, err = OpenAuthenticator(aead, tampered, ef)
	require.ErrorIs(t, err, ErrAuthenticatorVerify)
}

// TestAESSIVKnownAnswerVectors pins tink's deterministic AES-SIV-CMAC-512
// output for fixed inputs. Roundtripping via Seal+Open only proves
// self-consistency, so it can't catch a future tink output change that
// flips both Encrypt and Decrypt in step. This test asserts exact ciphertext
// bytes against captured tink output so any drift in the primitive — version
// bump, output-format change, accidental tag/ciphertext reordering — fails
// loudly.
//
// We do NOT use the Wycheproof aead_aes_siv_cmac_test.json vectors directly:
// Wycheproof's tests have separate aad and iv fields that map to a *two-entry*
// S2V AD list ([aad, iv]), but tink's single-AD EncryptDeterministically takes
// one AD bytes-slice and computes a *one-entry* S2V AD list ([ad]). Even with
// aad="" the resulting outputs differ — S2V([empty, iv]) ≠ S2V([iv]) because
// the empty entry still drives an extra round of S2V state mutation.
//
// Captured outputs are reproducible: same key (all-zero 64-byte), same AD,
// same plaintext → tink emits the same bytes deterministically. If tink ever
// updates AESSIV in a way that changes any of these, the test fails and we
// re-evaluate.
func TestAESSIVKnownAnswerVectors(t *testing.T) {
	key := make([]byte, subtle.AESSIVKeySize) // all-zero 64-byte key
	siv, err := subtle.NewAESSIV(key)
	require.NoError(t, err)

	cases := []struct {
		name    string
		ad      []byte
		pt      []byte
		wantHex string // synthetic IV (16 octets) || CTR-encrypted plaintext
	}{
		{
			name:    "empty-ad-empty-pt",
			ad:      nil,
			pt:      nil,
			wantHex: "7009249e60b613d224028adcc038d4ac",
		},
		{
			name:    "ad-empty-pt",
			ad:      []byte("associated-data"),
			pt:      nil,
			wantHex: "2d843364bf6958d9ea65ec6754f8481d",
		},
		{
			name:    "ad-32byte-pt",
			ad:      []byte("associated-data"),
			pt:      []byte("plaintext-msg-32-bytes-long-xxx!"),
			wantHex: "daec02b3076a9843466aff2cd869b39418baff9157c6a142023dafb90686902190fabb0c62052ff41fea3091a805660b",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want, err := hex.DecodeString(tc.wantHex)
			require.NoError(t, err)

			got, err := siv.EncryptDeterministically(tc.pt, tc.ad)
			require.NoError(t, err)
			require.Equal(t, want, got,
				"tink AES-SIV-CMAC-512 output drift detected — primitive or output layout has changed; re-derive expected bytes and confirm the new output is still RFC 5297-compliant before updating")

			pt, err := siv.DecryptDeterministically(want, tc.ad)
			require.NoError(t, err)
			require.True(t, bytes.Equal(tc.pt, pt))
		})
	}
}

func TestSealAuthenticatorThroughExtensionFramework(t *testing.T) {
	// Verify the EF can be encoded with MarshalExtensionFields and round-trips
	// through ParseExtensionFields cleanly.
	siv := newTestAEAD(t)
	ad := []byte("packet-up-to-authenticator")
	plaintext := make([]byte, 16)
	_, err := rand.Read(plaintext)
	require.NoError(t, err)

	ef, err := SealAuthenticator(siv, ad, plaintext)
	require.NoError(t, err)

	wire, err := protocol.MarshalExtensionFields([]protocol.ExtensionField{ef})
	require.NoError(t, err)
	parsed, err := protocol.ParseExtensionFields(wire)
	require.NoError(t, err)
	require.Len(t, parsed, 1)
	require.Equal(t, protocol.NTSAuthenticator, parsed[0].Type)

	// Body bytes from parse may include trailing zero padding — strip back to
	// the inner authenticator body length before comparing semantics.
	got, err := OpenAuthenticator(siv, ad, protocol.ExtensionField{Type: parsed[0].Type, Body: parsed[0].Body[:len(ef.Body)]})
	require.NoError(t, err)
	require.True(t, bytes.Equal(plaintext, got))
}
