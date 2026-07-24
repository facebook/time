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

package server

import (
	"crypto/rand"
	"encoding/binary"
	"testing"

	"github.com/facebook/time/ntp/ntske"
	ntp "github.com/facebook/time/ntp/protocol"
	"github.com/facebook/time/ntp/protocol/nts"
	"github.com/stretchr/testify/require"
)

// keyLenFor returns the session key length in octets required by the given AEAD.
func keyLenFor(t *testing.T, aeadID ntp.AEADAlgorithm) int {
	t.Helper()
	switch aeadID {
	case ntp.AEADAESSIVCMAC512:
		return 64
	case ntp.AEADAES128GCMSIV:
		return 16
	default:
		t.Fatalf("unsupported test AEAD %d", aeadID)
		return 0
	}
}

func randBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return b
}

// buildNTSRequest constructs a valid NTS protected NTP request the way a client
// would: UniqueID, Cookie, `placeholders` CookiePlaceholders, then an
// Authenticator sealed with the C2S key over everything before it. It returns
// the raw request bytes plus the S2C key, so the test can open the response.
func buildNTSRequest(t *testing.T, ks ntske.Keystore, aeadID ntp.AEADAlgorithm, uid []byte, placeholders int) (req, s2c []byte) {
	t.Helper()
	keyLen := keyLenFor(t, aeadID)
	c2s := randBytes(t, keyLen)
	s2c = randBytes(t, keyLen)

	cookie, err := ks.SealCookie(aeadID, c2s, s2c)
	require.NoError(t, err)

	efs := make([]ntp.ExtensionField, 0, 2+placeholders)
	efs = append(efs, nts.NewUniqueIdentifier(uid), nts.NewCookie(cookie))
	for range placeholders {
		efs = append(efs, nts.NewCookiePlaceholder(len(cookie)))
	}

	hdr, err := (&ntp.Packet{Settings: 0x23}).Bytes()
	require.NoError(t, err)
	preAuth, err := ntp.MarshalExtensionFields(efs)
	require.NoError(t, err)

	ad := append(append([]byte{}, hdr...), preAuth...)

	c2sAEAD, err := nts.NewAEAD(aeadID, c2s)
	require.NoError(t, err)
	// The request authenticator has no encrypted payload; it is pure integrity.
	auth, err := nts.SealAuthenticator(c2sAEAD, ad, nil)
	require.NoError(t, err)
	authBytes, err := ntp.MarshalExtensionFields([]ntp.ExtensionField{auth})
	require.NoError(t, err)

	return append(append([]byte{}, ad...), authBytes...), s2c
}

// openResponse plays the client: it locates the response authenticator, opens
// it with the S2C key, and returns how many NTSCookie extension fields were
// found INSIDE the decrypted payload, plus how many were (wrongly) present as
// cleartext extension fields alongside the header.
func openResponse(t *testing.T, resp []byte, aeadID ntp.AEADAlgorithm, s2c []byte) (innerCookies, cleartextCookies int) {
	t.Helper()
	require.Greater(t, len(resp), ntp.PacketSizeBytes)
	efs, err := ntp.ParseExtensionFields(resp[ntp.PacketSizeBytes:])
	require.NoError(t, err)
	require.NotEmpty(t, efs)

	authStart := ntp.PacketSizeBytes
	var authEF *ntp.ExtensionField
	for i := range efs {
		if efs[i].Type == ntp.NTSAuthenticator {
			require.Equal(t, len(efs)-1, i, "authenticator must be the final EF")
			authEF = &efs[i]
			break
		}
		if efs[i].Type == ntp.NTSCookie {
			cleartextCookies++
		}
		authStart += efs[i].EncodedSize()
	}
	require.NotNil(t, authEF, "response has no authenticator")

	s2cAEAD, err := nts.NewAEAD(aeadID, s2c)
	require.NoError(t, err)
	plaintext, err := nts.OpenAuthenticator(s2cAEAD, resp[:authStart], *authEF)
	require.NoError(t, err, "response authenticator must open with the S2C key")

	inner, err := ntp.ParseExtensionFields(plaintext)
	require.NoError(t, err)
	for _, ef := range inner {
		if ef.Type == ntp.NTSCookie {
			innerCookies++
		}
	}
	return innerCookies, cleartextCookies
}
func newTestKeystore(t *testing.T) ntske.Keystore {
	t.Helper()
	ks, err := ntske.NewInMemoryKeystore(ntske.InMemoryKeystoreOptions{})
	require.NoError(t, err)
	return ks
}

func respHeader(t *testing.T) *ntp.Packet {
	t.Helper()
	return &ntp.Packet{Settings: 0x24}
}

// TestProcessNTSRequestRoundTrip is the core happy path across both AEADs and a
// range of placeholder counts. It asserts the UID is echoed, the fresh cookies
// decrypt out of the authenticator, and — critically — that NO cookies leak in
// the clear.
func TestProcessNTSRequestRoundTrip(t *testing.T) {
	aeads := []ntp.AEADAlgorithm{ntp.AEADAESSIVCMAC512, ntp.AEADAES128GCMSIV}
	placeholderCounts := []int{0, 1, 7, 40} // 40 exercises the maxResponseCookies clamp

	for _, aeadID := range aeads {
		for _, p := range placeholderCounts {
			ks := newTestKeystore(t)
			uid := randBytes(t, nts.MinUniqueIdentifierLen)
			reqBytes, s2c := buildNTSRequest(t, ks, aeadID, uid, p)
			req := &ntp.Packet{}
			require.NoError(t, req.UnmarshalBinary(reqBytes))

			respPkt := respHeader(t)
			require.NoError(t, processNTSRequest(ks, req, respPkt))
			resp, err := respPkt.Bytes()
			require.NoError(t, err)

			// UID must be echoed in the clear.
			efs, err := ntp.ParseExtensionFields(resp[ntp.PacketSizeBytes:])
			require.NoError(t, err)
			require.Equal(t, ntp.UniqueIdentifier, efs[0].Type)
			require.Equal(t, uid, efs[0].Body)

			innerCookies, cleartextCookies := openResponse(t, resp, aeadID, s2c)

			want := min(1+p, maxResponseCookies)
			require.Equal(t, want, innerCookies, "wrong number of fresh cookies")

			// THE silent failure guard: response cookies must be encrypted
			// inside the authenticator, never cleartext EFs.
			require.Zero(t, cleartextCookies,
				"response cookies must be sealed inside the authenticator, not sent in the clear")
		}
	}
}

// TestProcessNTSRequestRejects covers every failure class: bad ordering, missing
// mandatory fields, and cryptographic tampering.
func TestProcessNTSRequestRejects(t *testing.T) {
	const aeadID = ntp.AEADAESSIVCMAC512

	t.Run("plain NTP (no extension fields)", func(t *testing.T) {
		ks := newTestKeystore(t)
		hdr := respHeader(t)
		req := &ntp.Packet{Settings: 0x23}
		err := processNTSRequest(ks, req, hdr)
		require.ErrorIs(t, err, ErrNoExtensionFields)
	})

	t.Run("cookie before UniqueID", func(t *testing.T) {
		ks := newTestKeystore(t)
		c2s := randBytes(t, keyLenFor(t, aeadID))
		s2c := randBytes(t, keyLenFor(t, aeadID))
		cookie, err := ks.SealCookie(aeadID, c2s, s2c)
		require.NoError(t, err)
		reqBytes := buildRawRequest(t, c2s, []ntp.ExtensionField{
			nts.NewCookie(cookie),
			nts.NewUniqueIdentifier(randBytes(t, nts.MinUniqueIdentifierLen)),
		})
		req := &ntp.Packet{}
		require.NoError(t, req.UnmarshalBinary(reqBytes))
		err = processNTSRequest(ks, req, respHeader(t))
		require.ErrorIs(t, err, ErrExtensionOrder)
	})
	t.Run("short UniqueID", func(t *testing.T) {
		ks := newTestKeystore(t)
		c2s := randBytes(t, keyLenFor(t, aeadID))
		s2c := randBytes(t, keyLenFor(t, aeadID))
		cookie, err := ks.SealCookie(aeadID, c2s, s2c)
		require.NoError(t, err)
		reqBytes := buildRawRequest(t, c2s, []ntp.ExtensionField{
			nts.NewUniqueIdentifier(randBytes(t, 16)),
			nts.NewCookie(cookie),
		})
		req := &ntp.Packet{}
		require.NoError(t, req.UnmarshalBinary(reqBytes))
		err = processNTSRequest(ks, req, respHeader(t))
		require.ErrorIs(t, err, ErrShortUniqueID)
	})

	t.Run("missing cookie", func(t *testing.T) {
		ks := newTestKeystore(t)
		c2s := randBytes(t, keyLenFor(t, aeadID))
		reqBytes := buildRawRequest(t, c2s, []ntp.ExtensionField{
			nts.NewUniqueIdentifier(randBytes(t, nts.MinUniqueIdentifierLen)),
		})
		req := &ntp.Packet{}
		require.NoError(t, req.UnmarshalBinary(reqBytes))
		err := processNTSRequest(ks, req, respHeader(t))
		require.ErrorIs(t, err, ErrMissingCookie)
	})
	t.Run("unknown KeyID cookie", func(t *testing.T) {
		ks := newTestKeystore(t)
		// A different keystore's cookie cannot be opened by ks.
		otherKS := newTestKeystore(t)
		uid := randBytes(t, nts.MinUniqueIdentifierLen)
		reqBytes, _ := buildNTSRequest(t, otherKS, aeadID, uid, 0)
		req := &ntp.Packet{}
		require.NoError(t, req.UnmarshalBinary(reqBytes))
		err := processNTSRequest(ks, req, respHeader(t))
		require.ErrorIs(t, err, ErrCookieOpen)
	})
	t.Run("tampered authenticator", func(t *testing.T) {
		ks := newTestKeystore(t)
		uid := randBytes(t, nts.MinUniqueIdentifierLen)
		reqBytes, _ := buildNTSRequest(t, ks, aeadID, uid, 0)
		// Flip a byte inside the authenticator (the last EF's ciphertext).
		reqBytes[len(reqBytes)-1] ^= 0xff
		req := &ntp.Packet{}
		require.NoError(t, req.UnmarshalBinary(reqBytes))
		err := processNTSRequest(ks, req, respHeader(t))
		require.ErrorIs(t, err, ErrAuthVerify)
	})

	t.Run("missing authenticator", func(t *testing.T) {
		ks := newTestKeystore(t)
		body, err := ntp.MarshalExtensionFields([]ntp.ExtensionField{
			nts.NewUniqueIdentifier(randBytes(t, nts.MinUniqueIdentifierLen)),
			nts.NewCookie(randBytes(t, 64)),
		})
		require.NoError(t, err)
		hdr, err := (&ntp.Packet{Settings: 0x23}).Bytes()
		require.NoError(t, err)
		req := &ntp.Packet{}
		require.NoError(t, req.UnmarshalBinary(append(hdr, body...)))
		err = processNTSRequest(ks, req, respHeader(t))
		require.ErrorIs(t, err, ErrMissingAuth)
	})

	t.Run("trailing extension field after authenticator is discarded", func(t *testing.T) {
		// RFC 8915 §5.2/§5.7: the client MAY append unauthenticated EFs after
		// the authenticator; the server MUST discard them, NOT reject the
		// packet. The request must still be served with a valid response.
		ks := newTestKeystore(t)
		uid := randBytes(t, nts.MinUniqueIdentifierLen)
		reqBytes, s2c := buildNTSRequest(t, ks, aeadID, uid, 0)
		trailer, err := ntp.MarshalExtensionFields([]ntp.ExtensionField{
			{Type: 0x0500, Body: make([]byte, 4)},
		})
		require.NoError(t, err)
		reqBytes = append(reqBytes, trailer...)
		req := &ntp.Packet{}
		require.NoError(t, req.UnmarshalBinary(reqBytes))
		respPkt := respHeader(t)
		require.NoError(t, processNTSRequest(ks, req, respPkt))
		resp, err := respPkt.Bytes()
		require.NoError(t, err)
		inner, cleartext := openResponse(t, resp, aeadID, s2c)
		require.Equal(t, 1, inner)
		require.Zero(t, cleartext)
	})

	t.Run("duplicate UniqueIdentifier", func(t *testing.T) {
		ks := newTestKeystore(t)
		c2s := randBytes(t, keyLenFor(t, aeadID))
		s2c := randBytes(t, keyLenFor(t, aeadID))
		cookie, err := ks.SealCookie(aeadID, c2s, s2c)
		require.NoError(t, err)
		reqBytes := buildRawRequest(t, c2s, []ntp.ExtensionField{
			nts.NewUniqueIdentifier(randBytes(t, nts.MinUniqueIdentifierLen)),
			nts.NewUniqueIdentifier(randBytes(t, nts.MinUniqueIdentifierLen)),
			nts.NewCookie(cookie),
		})
		req := &ntp.Packet{}
		require.NoError(t, req.UnmarshalBinary(reqBytes))
		err = processNTSRequest(ks, req, respHeader(t))
		require.ErrorIs(t, err, ErrExtensionOrder)
	})

	t.Run("duplicate cookie", func(t *testing.T) {
		ks := newTestKeystore(t)
		c2s := randBytes(t, keyLenFor(t, aeadID))
		s2c := randBytes(t, keyLenFor(t, aeadID))
		cookie, err := ks.SealCookie(aeadID, c2s, s2c)
		require.NoError(t, err)
		reqBytes := buildRawRequest(t, c2s, []ntp.ExtensionField{
			nts.NewUniqueIdentifier(randBytes(t, nts.MinUniqueIdentifierLen)),
			nts.NewCookie(cookie),
			nts.NewCookie(cookie),
		})
		req := &ntp.Packet{}
		require.NoError(t, req.UnmarshalBinary(reqBytes))
		err = processNTSRequest(ks, req, respHeader(t))
		require.ErrorIs(t, err, ErrExtensionOrder)
	})

	t.Run("placeholder before cookie", func(t *testing.T) {
		ks := newTestKeystore(t)
		c2s := randBytes(t, keyLenFor(t, aeadID))
		s2c := randBytes(t, keyLenFor(t, aeadID))
		cookie, err := ks.SealCookie(aeadID, c2s, s2c)
		require.NoError(t, err)
		reqBytes := buildRawRequest(t, c2s, []ntp.ExtensionField{
			nts.NewUniqueIdentifier(randBytes(t, nts.MinUniqueIdentifierLen)),
			nts.NewCookiePlaceholder(len(cookie)),
			nts.NewCookie(cookie),
		})
		req := &ntp.Packet{}
		require.NoError(t, req.UnmarshalBinary(reqBytes))
		err = processNTSRequest(ks, req, respHeader(t))
		require.ErrorIs(t, err, ErrExtensionOrder)
	})

	t.Run("wrong-length placeholder is ignored", func(t *testing.T) {
		ks := newTestKeystore(t)
		c2s := randBytes(t, keyLenFor(t, aeadID))
		s2c := randBytes(t, keyLenFor(t, aeadID))
		cookie, err := ks.SealCookie(aeadID, c2s, s2c)
		require.NoError(t, err)
		// RFC 8915 §5.5: a placeholder whose body length differs from the
		// cookie's MUST NOT be counted, so the server returns only the single
		// replacement cookie.
		reqBytes := buildRawRequest(t, c2s, []ntp.ExtensionField{
			nts.NewUniqueIdentifier(randBytes(t, nts.MinUniqueIdentifierLen)),
			nts.NewCookie(cookie),
			nts.NewCookiePlaceholder(len(cookie) + 4),
		})
		req := &ntp.Packet{}
		require.NoError(t, req.UnmarshalBinary(reqBytes))
		respPkt := respHeader(t)
		require.NoError(t, processNTSRequest(ks, req, respPkt))
		resp, err := respPkt.Bytes()
		require.NoError(t, err)
		inner, cleartext := openResponse(t, resp, aeadID, s2c)
		require.Equal(t, 1, inner, "mismatched-length placeholder must not yield an extra cookie")
		require.Zero(t, cleartext)
	})
}

// buildRawRequest builds a request with an arbitrary EF list plus a valid
// authenticator sealed with c2s, without imposing the ordering buildNTSRequest
// does — so tests can craft malformed requests.
func buildRawRequest(t *testing.T, c2s []byte, efs []ntp.ExtensionField) []byte {
	t.Helper()
	// buildRawRequest only exercises the SIV path; callers needing a different
	// AEAD build the request inline.
	const aeadID = ntp.AEADAESSIVCMAC512
	hdr, err := (&ntp.Packet{Settings: 0x23}).Bytes()
	require.NoError(t, err)
	preAuth, err := ntp.MarshalExtensionFields(efs)
	require.NoError(t, err)
	ad := append(append([]byte{}, hdr...), preAuth...)
	c2sAEAD, err := nts.NewAEAD(aeadID, c2s)
	require.NoError(t, err)
	auth, err := nts.SealAuthenticator(c2sAEAD, ad, nil)
	require.NoError(t, err)
	authBytes, err := ntp.MarshalExtensionFields([]ntp.ExtensionField{auth})
	require.NoError(t, err)
	return append(append([]byte{}, ad...), authBytes...)
}

func TestProcessNTSRequestVerifiesOverRawBytes(t *testing.T) {
	// RFC 8915 §5.2: unrecognized EFs before the authenticator are still covered
	// by the MAC. Build a valid NTS request with a reserved-type EF (0xF001); the
	// server must reconstruct the exact signed bytes by faithfully re-serializing
	// the parsed packet (reserved type and all).
	const aeadID = ntp.AEADAESSIVCMAC512
	ks := newTestKeystore(t)
	c2s := randBytes(t, keyLenFor(t, aeadID))
	s2c := randBytes(t, keyLenFor(t, aeadID))
	cookie, err := ks.SealCookie(aeadID, c2s, s2c)
	require.NoError(t, err)
	uid := randBytes(t, nts.MinUniqueIdentifierLen)

	// Build EF list: UID, Cookie, reserved EF, Authenticator (last).
	// The reserved EF is type 0xF001, length 8 (header + 4 zero body bytes).
	reservedEF := ntp.ExtensionField{Type: 0xF001, Body: make([]byte, 4)}
	efs := []ntp.ExtensionField{
		nts.NewUniqueIdentifier(uid),
		nts.NewCookie(cookie),
		reservedEF,
	}

	// Marshal header + EFs before auth, seal authenticator over exact bytes.
	hdr, err := (&ntp.Packet{Settings: 0x23}).Bytes()
	require.NoError(t, err)
	// Manually marshal the reserved EF since MarshalExtensionFields rejects it.
	reservedBytes := make([]byte, 8)
	binary.BigEndian.PutUint16(reservedBytes[0:2], uint16(reservedEF.Type))
	binary.BigEndian.PutUint16(reservedBytes[2:4], 8)
	preAuthKnown, err := ntp.MarshalExtensionFields(efs[:2])
	require.NoError(t, err)
	ad := append(append(append([]byte{}, hdr...), preAuthKnown...), reservedBytes...)

	c2sAEAD, err := nts.NewAEAD(aeadID, c2s)
	require.NoError(t, err)
	auth, err := nts.SealAuthenticator(c2sAEAD, ad, nil)
	require.NoError(t, err)
	authBytes, err := ntp.MarshalExtensionFields([]ntp.ExtensionField{auth})
	require.NoError(t, err)
	reqBytes := append(append([]byte{}, ad...), authBytes...)

	req := &ntp.Packet{}
	require.NoError(t, req.UnmarshalBinary(reqBytes))

	err = processNTSRequest(ks, req, respHeader(t))
	require.NoError(t, err, "server must cover unrecognized EFs in the authenticator, reproducing exact received bytes per RFC 8915 §5.4")
}

// TestProcessNTSRequestWithClientHelpers drives the real server path with the
// public nts client helpers (what ntsketest uses), covering both AEADs.
func TestProcessNTSRequestWithClientHelpers(t *testing.T) {
	cases := []struct {
		name string
		aead ntp.AEADAlgorithm
	}{
		{"AES-128-GCM-SIV", ntp.AEADAES128GCMSIV},
		{"AES-SIV-CMAC-512", ntp.AEADAESSIVCMAC512},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ks := newTestKeystore(t)
			keyLen := keyLenFor(t, tc.aead)
			c2s := randBytes(t, keyLen)
			s2c := randBytes(t, keyLen)
			cookie, err := ks.SealCookie(tc.aead, c2s, s2c)
			require.NoError(t, err)

			uid := randBytes(t, nts.MinUniqueIdentifierLen)
			reqBytes, err := nts.BuildNTSRequest(ntp.Packet{Settings: 0x23}, nts.RequestParams{
				AEAD: tc.aead, C2S: c2s, Cookie: cookie, UniqueID: uid, Placeholders: 2,
			})
			require.NoError(t, err)

			req := &ntp.Packet{}
			require.NoError(t, req.UnmarshalBinary(reqBytes))
			resp := respHeader(t)
			require.NoError(t, processNTSRequest(ks, req, resp))
			respBytes, err := resp.Bytes()
			require.NoError(t, err)

			_, cookies, err := nts.VerifyNTSResponse(respBytes, tc.aead, s2c, uid)
			require.NoError(t, err)
			require.Len(t, cookies, 3) // one spent cookie + two placeholders
		})
	}
}
