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
	"testing"

	"github.com/facebook/time/ntp/protocol"
	"github.com/stretchr/testify/require"
)

const testAEAD = protocol.AEADAES128GCMSIV // 16-octet keys

func testKey(b byte) []byte { return bytes.Repeat([]byte{b}, 16) }
func testUID() []byte       { return bytes.Repeat([]byte{9}, MinUniqueIdentifierLen) }

// sealResponse mimics the server: UID echo in the clear + an authenticator that
// encrypts the fresh cookies, sealed with the S2C key.
func sealResponse(t *testing.T, aeadID protocol.AEADAlgorithm, s2c, uid []byte, cookies [][]byte) []byte {
	t.Helper()
	resp := &protocol.Packet{Settings: 0x24}
	resp.ExtensionFields = []protocol.ExtensionField{NewUniqueIdentifier(uid)}
	ad, err := resp.AssociatedData(1)
	require.NoError(t, err)

	cookieEFs := make([]protocol.ExtensionField, 0, len(cookies))
	for _, c := range cookies {
		cookieEFs = append(cookieEFs, NewCookie(c))
	}
	enc, err := protocol.MarshalExtensionFields(cookieEFs)
	require.NoError(t, err)

	aead, err := NewAEAD(aeadID, s2c)
	require.NoError(t, err)
	auth, err := SealAuthenticator(aead, ad, enc)
	require.NoError(t, err)
	resp.ExtensionFields = append(resp.ExtensionFields, auth)

	b, err := resp.Bytes()
	require.NoError(t, err)
	return b
}

func TestBuildNTSRequestOrder(t *testing.T) {
	base := protocol.Packet{Settings: 0x1B}
	reqBytes, err := BuildNTSRequest(base, RequestParams{
		AEAD: testAEAD, C2S: testKey(3), Cookie: []byte("a-cookie-value"),
		UniqueID: testUID(), Placeholders: 2,
	})
	require.NoError(t, err)

	pkt := &protocol.Packet{}
	require.NoError(t, pkt.UnmarshalBinary(reqBytes))

	got := make([]protocol.ExtensionFieldType, 0, len(pkt.ExtensionFields))
	for _, ef := range pkt.ExtensionFields {
		got = append(got, ef.Type)
	}
	require.Equal(t, []protocol.ExtensionFieldType{
		protocol.UniqueIdentifier,
		protocol.NTSCookie,
		protocol.NTSCookiePlaceholder,
		protocol.NTSCookiePlaceholder,
		protocol.NTSAuthenticator,
	}, got)
}

func TestBuildNTSRequestValidates(t *testing.T) {
	base := protocol.Packet{}
	_, err := BuildNTSRequest(base, RequestParams{
		AEAD: testAEAD, C2S: testKey(3), Cookie: []byte("c"), UniqueID: []byte("short"),
	})
	require.Error(t, err)

	_, err = BuildNTSRequest(base, RequestParams{
		AEAD: testAEAD, C2S: testKey(3), Cookie: nil, UniqueID: testUID(),
	})
	require.Error(t, err)
}

func TestVerifyNTSResponseRoundTrip(t *testing.T) {
	s2c := testKey(1)
	uid := testUID()
	fresh := [][]byte{[]byte("cookie-1"), []byte("cookie-2")}
	resp := sealResponse(t, testAEAD, s2c, uid, fresh)

	pkt, cookies, err := VerifyNTSResponse(resp, testAEAD, s2c, uid)
	require.NoError(t, err)
	require.NotNil(t, pkt)
	require.Equal(t, fresh, cookies)
}

// TestNTPRoundTrip exercises both helpers for every supported AEAD: build a
// request, confirm its authenticator verifies (server side), then verify a reply.
func TestNTPRoundTrip(t *testing.T) {
	cases := []struct {
		name   string
		aead   protocol.AEADAlgorithm
		keyLen int
	}{
		{"AES-128-GCM-SIV", protocol.AEADAES128GCMSIV, 16},
		{"AES-SIV-CMAC-512", protocol.AEADAESSIVCMAC512, 64},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c2s := bytes.Repeat([]byte{3}, tc.keyLen)
			s2c := bytes.Repeat([]byte{4}, tc.keyLen)
			uid := testUID()

			reqBytes, err := BuildNTSRequest(protocol.Packet{Settings: 0x23}, RequestParams{
				AEAD: tc.aead, C2S: c2s, Cookie: []byte("a-cookie-value"),
				UniqueID: uid, Placeholders: 1,
			})
			require.NoError(t, err)

			// Server side: the request authenticator verifies under C2S.
			req := &protocol.Packet{}
			require.NoError(t, req.UnmarshalBinary(reqBytes))
			authIdx := -1
			for i := range req.ExtensionFields {
				if req.ExtensionFields[i].Type == protocol.NTSAuthenticator {
					authIdx = i
					break
				}
			}
			require.GreaterOrEqual(t, authIdx, 0)
			reqAD, err := req.AssociatedData(authIdx)
			require.NoError(t, err)
			c2sAEAD, err := NewAEAD(tc.aead, c2s)
			require.NoError(t, err)
			_, err = OpenAuthenticator(c2sAEAD, reqAD, req.ExtensionFields[authIdx])
			require.NoError(t, err)

			// Client verifies the server's response.
			fresh := [][]byte{[]byte("fresh-01"), []byte("fresh-02")}
			resp := sealResponse(t, tc.aead, s2c, uid, fresh)
			_, cookies, err := VerifyNTSResponse(resp, tc.aead, s2c, uid)
			require.NoError(t, err)
			require.Equal(t, fresh, cookies)
		})
	}
}

func TestVerifyNTSResponseRejects(t *testing.T) {
	s2c := testKey(1)
	uid := testUID()
	resp := sealResponse(t, testAEAD, s2c, uid, [][]byte{[]byte("cookie-1")})

	// UID that does not match the request.
	_, _, err := VerifyNTSResponse(resp, testAEAD, s2c, bytes.Repeat([]byte{7}, MinUniqueIdentifierLen))
	require.Error(t, err)

	// Wrong S2C key fails authenticator verification.
	_, _, err = VerifyNTSResponse(resp, testAEAD, testKey(2), uid)
	require.Error(t, err)
}

func TestVerifyNTSResponseRejectsTrailingEF(t *testing.T) {
	s2c := testKey(1)
	uid := testUID()
	resp := &protocol.Packet{Settings: 0x24}
	resp.ExtensionFields = []protocol.ExtensionField{NewUniqueIdentifier(uid)}
	ad, err := resp.AssociatedData(1)
	require.NoError(t, err)
	aead, err := NewAEAD(testAEAD, s2c)
	require.NoError(t, err)
	auth, err := SealAuthenticator(aead, ad, nil)
	require.NoError(t, err)
	// An extension field after the authenticator is unauthenticated and must be rejected.
	resp.ExtensionFields = append(resp.ExtensionFields, auth, NewCookiePlaceholder(16))
	b, err := resp.Bytes()
	require.NoError(t, err)

	_, _, err = VerifyNTSResponse(b, testAEAD, s2c, uid)
	require.Error(t, err)
}
