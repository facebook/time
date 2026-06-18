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

// Package nts implements the Network Time Security extension fields defined
// by RFC 8915 §5. It provides type constants, builders for the simple fields
// (Unique Identifier, NTS Cookie, NTS Cookie Placeholder), and an AES-SIV
// implementation of the NTS Authenticator and Encrypted Extension Fields field.
package nts

import (
	"github.com/facebook/time/ntp/protocol"
)

// NTS extension field types defined by RFC 8915 §5.1.1.
const (
	UniqueIdentifier     protocol.ExtensionFieldType = 0x0104
	NTSCookie            protocol.ExtensionFieldType = 0x0204
	NTSCookiePlaceholder protocol.ExtensionFieldType = 0x0304
	NTSAuthenticator     protocol.ExtensionFieldType = 0x0404
)

// MinUniqueIdentifierLen is the minimum length in octets of a Unique Identifier
// nonce per RFC 8915 §5.3.
const MinUniqueIdentifierLen = 32

// NewUniqueIdentifier returns a Unique Identifier extension field carrying the
// given nonce. The nonce MUST be at least MinUniqueIdentifierLen octets and
// MUST be unpredictable; callers should use crypto/rand to generate it.
//
// This function does NOT validate the nonce length or entropy: passing a
// short or predictable nonce silently produces an EF that breaks the
// replay-protection and request/response-correlation properties of RFC 8915
// §5.3. Trust-the-caller is the standard Go idiom for security-sensitive
// nonces (cf. crypto/rand, crypto/cipher.NewGCMWithNonceSize), and entropy
// cannot be checked from a single sample anyway. The contract lives in this
// godoc; the call site is responsible for honouring it.
func NewUniqueIdentifier(nonce []byte) protocol.ExtensionField {
	return protocol.ExtensionField{Type: UniqueIdentifier, Body: nonce}
}

// NewCookie returns an NTS Cookie extension field carrying the given opaque
// cookie bytes (issued previously by the server during NTS-KE or in a prior
// NTPv4 response).
func NewCookie(cookie []byte) protocol.ExtensionField {
	return protocol.ExtensionField{Type: NTSCookie, Body: cookie}
}

// NewCookiePlaceholder returns a Cookie Placeholder extension field of the
// requested length, used by the client to ask the server for an additional
// cookie of that size in its response (RFC 8915 §5.5).
func NewCookiePlaceholder(targetLen int) protocol.ExtensionField {
	return protocol.ExtensionField{Type: NTSCookiePlaceholder, Body: make([]byte, targetLen)}
}
