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
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/facebook/time/ntp/protocol"
	"github.com/tink-crypto/tink-go/v2/daead/subtle"
)

/*
NTS Authenticator and Encrypted Extension Fields (RFC 8915 §5.6).

	0                   1                   2                   3
	0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
  +-------------------------------+-------------------------------+
  |    Field Type = 0x0404        |          Length               |
  +-------------------------------+-------------------------------+
  |          Nonce Length         |       Ciphertext Length       |
  +-------------------------------+-------------------------------+
  |                                                               |
  ~                            Nonce                              ~
  |                                                               |
  +---------------------------------------------------------------+
  |                                                               |
  ~                         Ciphertext                            ~
  |                                                               |
  +---------------------------------------------------------------+
  |                  Additional Padding (if any)                  |
  +---------------------------------------------------------------+

The associated data is the entire NTP packet from the start of the header
through the end of the extension field immediately preceding this one
(RFC 8915 §5.6.1). The plaintext, when present, is itself a sequence of
NTP extension fields ("encrypted EFs") which the receiver appends to its
parsed extension-field list.

RFC 8915 §5.6 specifies that the Nonce field is "as required by the negotiated
AEAD algorithm". RFC 5297 AES-SIV-CMAC is a deterministic AEAD that does not
require a separate nonce — the synthetic IV is computed over the AD list and
returned as the first 16 octets of the AEAD output. Matching chrony's wire
choice (nts_ntp_auth.c) we set Nonce Length = 0 on SIV-protected packets and
carry the synthetic IV at the head of the Ciphertext field.
*/

// AuthenticatorBody is the parsed body of an NTS Authenticator extension field.
type AuthenticatorBody struct {
	Nonce      []byte
	Ciphertext []byte
}

const authenticatorHeaderLen = 4

// Sentinel errors.
var (
	ErrAuthenticatorTruncated = errors.New("nts authenticator truncated")
	ErrAuthenticatorMalformed = errors.New("nts authenticator malformed")
	// ErrAuthenticatorVerify is returned when AEAD verification fails on
	// OpenAuthenticator. It is deliberately the only thing callers can
	// errors.Is on for verification failures — the underlying tink error
	// is included as text for debug visibility but not as part of the
	// error chain, to insulate callers from tink internals (defense in
	// depth in case a future tink revision adds sensitive detail to its
	// error messages, and to follow the crypto/cipher.GCM stdlib pattern
	// of "cipher: message authentication failed").
	ErrAuthenticatorVerify = errors.New("nts authenticator verification failed")
)

// padTo4 returns the smallest multiple of 4 >= n.
func padTo4(n int) int {
	return (n + 3) &^ 3
}

// MarshalAuthenticatorBody serializes nonce and ciphertext into the
// authenticator EF body wire format defined by RFC 8915 §5.6. Nonce and
// ciphertext are each padded with zeros to a 4-octet boundary.
func MarshalAuthenticatorBody(nonce, ciphertext []byte) ([]byte, error) {
	if len(nonce) > 0xFFFF {
		return nil, fmt.Errorf("nts: nonce length %d exceeds uint16", len(nonce))
	}
	if len(ciphertext) > 0xFFFF {
		return nil, fmt.Errorf("nts: ciphertext length %d exceeds uint16", len(ciphertext))
	}
	paddedNonceLen := padTo4(len(nonce))
	paddedCipherLen := padTo4(len(ciphertext))
	// RFC 8915 §5.6 requires the padding octets after nonce and ciphertext
	// to be zero. We rely on Go's make() zero-initialisation to satisfy this
	// implicitly — any future refactor that reuses a buffer (e.g. via
	// sync.Pool) MUST explicitly zero the padding region before writing the
	// nonce and ciphertext, or invalid frames will go on the wire.
	out := make([]byte, authenticatorHeaderLen+paddedNonceLen+paddedCipherLen)
	binary.BigEndian.PutUint16(out[0:2], uint16(len(nonce)))      // #nosec G115 -- bounds checked above
	binary.BigEndian.PutUint16(out[2:4], uint16(len(ciphertext))) // #nosec G115 -- bounds checked above
	copy(out[authenticatorHeaderLen:], nonce)
	copy(out[authenticatorHeaderLen+paddedNonceLen:], ciphertext)
	return out, nil
}

// ParseAuthenticatorBody decodes the body of an NTS Authenticator EF.
//
// Trailing bytes past the declared nonce + ciphertext are silently accepted:
// RFC 8915 §5.6 explicitly allows "Additional Padding (if any)" after the
// ciphertext, and we have no way to distinguish legitimate padding from
// stray bytes without protocol-specific knowledge that doesn't belong at
// this layer.
func ParseAuthenticatorBody(body []byte) (AuthenticatorBody, error) {
	var ab AuthenticatorBody
	if len(body) < authenticatorHeaderLen {
		return ab, fmt.Errorf("%w: header needs %d bytes, have %d",
			ErrAuthenticatorTruncated, authenticatorHeaderLen, len(body))
	}
	nonceLen := int(binary.BigEndian.Uint16(body[0:2]))
	cipherLen := int(binary.BigEndian.Uint16(body[2:4]))
	paddedNonceLen := padTo4(nonceLen)
	paddedCipherLen := padTo4(cipherLen)
	required := authenticatorHeaderLen + paddedNonceLen + paddedCipherLen
	if required > len(body) {
		return ab, fmt.Errorf("%w: required=%d body_len=%d nonce_len=%d cipher_len=%d",
			ErrAuthenticatorTruncated, required, len(body), nonceLen, cipherLen)
	}
	ab.Nonce = make([]byte, nonceLen)
	copy(ab.Nonce, body[authenticatorHeaderLen:authenticatorHeaderLen+nonceLen])
	ab.Ciphertext = make([]byte, cipherLen)
	copy(ab.Ciphertext, body[authenticatorHeaderLen+paddedNonceLen:authenticatorHeaderLen+paddedNonceLen+cipherLen])
	return ab, nil
}

// SealAuthenticator builds an NTS Authenticator extension field over the
// supplied associated data. The plaintext (typically empty for the common
// case of no encrypted EFs) is encrypted and authenticated using AES-SIV.
// The returned extension field is appended after the AD on the wire.
//
// Per RFC 8915 §5.6.1, the caller MUST pass `ad` as the NTP packet header
// (48 octets) followed by every extension field preceding this Authenticator,
// exactly as those bytes appear on the wire (including any wire padding).
// This package has no NTP-packet awareness, so it cannot validate the AD
// shape — that responsibility lives one layer up (the responder request
// path or the KE-side smoke client). If the caller constructs the wrong AD,
// the receiver's OpenAuthenticator will fail verification with
// ErrAuthenticatorVerify; bugs of this kind manifest as "every NTS request
// rejected" with no other signal, so AD-construction code should be reviewed
// carefully.
func SealAuthenticator(siv *subtle.AESSIV, ad, plaintext []byte) (protocol.ExtensionField, error) {
	ct, err := siv.EncryptDeterministically(plaintext, ad)
	if err != nil {
		return protocol.ExtensionField{}, fmt.Errorf("nts: SIV encrypt: %w", err)
	}
	body, err := MarshalAuthenticatorBody(nil, ct)
	if err != nil {
		return protocol.ExtensionField{}, err
	}
	return protocol.ExtensionField{Type: protocol.NTSAuthenticator, Body: body}, nil
}

// OpenAuthenticator verifies and decrypts an NTS Authenticator extension
// field. Returns the encrypted plaintext (zero or more encrypted EFs as a
// concatenated byte slice) on success.
func OpenAuthenticator(siv *subtle.AESSIV, ad []byte, ef protocol.ExtensionField) ([]byte, error) {
	if ef.Type != protocol.NTSAuthenticator {
		return nil, fmt.Errorf("%w: expected type %#x got %#x",
			ErrAuthenticatorMalformed, protocol.NTSAuthenticator, ef.Type)
	}
	body, err := ParseAuthenticatorBody(ef.Body)
	if err != nil {
		return nil, err
	}
	if len(body.Nonce) != 0 {
		// SIV-protected packets carry Nonce Length = 0 on the wire — see the
		// file-level comment above. Peers that send a non-empty Nonce would
		// require the bytes to be folded into the SIV S2V input list as a
		// trailing entry; tink's single-AD EncryptDeterministically/
		// DecryptDeterministically API doesn't expose that, and chrony (our
		// primary interop target) sets the Nonce to empty for SIV anyway.
		return nil, fmt.Errorf("%w: empty Nonce required for SIV, got %d bytes",
			ErrAuthenticatorMalformed, len(body.Nonce))
	}
	pt, err := siv.DecryptDeterministically(body.Ciphertext, ad)
	if err != nil {
		// Wrap ErrAuthenticatorVerify (not the tink error) so the tink err
		// stays out of the errors.Is chain — see the sentinel's godoc.
		// Pass err.Error() rather than err so errorlint doesn't "helpfully"
		// rewrite the %s back to %w, which would put the tink err in the
		// chain and silently break the contract.
		return nil, fmt.Errorf("%w: %s", ErrAuthenticatorVerify, err.Error())
	}
	return pt, nil
}
