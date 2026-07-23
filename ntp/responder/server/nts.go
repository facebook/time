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
	"errors"
	"fmt"

	"github.com/facebook/time/ntp/ntske"
	ntp "github.com/facebook/time/ntp/protocol"
	"github.com/facebook/time/ntp/protocol/nts"
)

// maxResponseCookies caps how many fresh cookies a single response carries. This
// bounds the amplification factor regardless of how many placeholders a client
// stuffs into its request.
const maxResponseCookies = 32 //nolint:unused // used by the NTS request path

// Failure classes for the NTS request path. Callers use errors.Is to branch on
// them (e.g. for stats); the wrapped error carries the detail.
var (
	ErrNoExtensionFields = errors.New("nts: request carries no extension fields")
	ErrExtensionOrder    = errors.New("nts: extension fields out of order")
	ErrMissingUniqueID   = errors.New("nts: missing UniqueIdentifier extension field")
	ErrShortUniqueID     = errors.New("nts: UniqueIdentifier shorter than 32 octets")
	ErrMissingCookie     = errors.New("nts: missing NTSCookie extension field")
	ErrMissingAuth       = errors.New("nts: missing NTSAuthenticator extension field")
	ErrCookieOpen        = errors.New("nts: cookie decryption failed")
	ErrAuthVerify        = errors.New("nts: request authenticator verification failed")
)

// processNTSRequest authenticates an NTS-protected NTP request and fills the
// response's extension fields (UniqueID echo + sealed authenticator). req is the
// parsed request; resp is the response whose header is already set. The caller
// marshals resp via Bytes(), so both the NTS and plain paths share one marshal.
func processNTSRequest(ks ntske.Keystore, req, resp *ntp.Packet) error { //nolint:unused // used by the NTS request path
	if len(req.ExtensionFields) == 0 {
		return ErrNoExtensionFields
	}

	efs := req.ExtensionFields

	var (
		uidBody          []byte
		cookieBody       []byte
		authEF           *ntp.ExtensionField
		authIdx          = -1
		placeholderCount int
		seenUID          bool
		seenCookie       bool
	)

	for i := range efs {
		ef := &efs[i]
		switch ef.Type {
		case ntp.UniqueIdentifier:
			if seenUID || seenCookie {
				return ErrExtensionOrder
			}
			if len(ef.Body) < nts.MinUniqueIdentifierLen {
				return ErrShortUniqueID
			}
			uidBody = ef.Body
			seenUID = true
		case ntp.NTSCookie:
			if !seenUID || seenCookie {
				return ErrExtensionOrder
			}
			cookieBody = ef.Body
			seenCookie = true
		case ntp.NTSCookiePlaceholder:
			if !seenCookie {
				return ErrExtensionOrder
			}
			// RFC 8915 §5.5: a placeholder's body MUST be the same length as a
			// cookie, so the response cannot grow larger than the request.
			if len(ef.Body) == len(cookieBody) {
				placeholderCount++
			}
		case ntp.NTSAuthenticator:
			if !seenCookie {
				return ErrMissingCookie
			}
			// First authenticator EF: record its index (associated data is the
			// header + all preceding EFs, RFC 8915 §5.6.1) and stop. Trailing EFs
			// after it are unauthenticated; §5.7 says discard them (we break, so
			// they never reach the response), not reject the packet.
			authEF = ef
			authIdx = i
		}
		if ef.Type == ntp.NTSAuthenticator {
			break
		}
	}

	switch {
	case !seenUID:
		return ErrMissingUniqueID
	case !seenCookie:
		return ErrMissingCookie
	case authEF == nil:
		return ErrMissingAuth
	}

	// Recover the AEAD algorithm and the two session keys from the cookie.
	aeadID, c2s, s2c, err := ks.OpenCookie(cookieBody)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCookieOpen, err)
	}

	// Reconstruct the bytes the client authenticated over: the NTP header plus
	// every extension field preceding the authenticator, re-marshaled from the
	// parsed packet. RFC 8915 §5.4 requires unrecognized EFs to be covered too,
	// so AssociatedData re-emits them faithfully (see encodeExtensionFields).
	reqAD, err := req.AssociatedData(authIdx)
	if err != nil {
		return fmt.Errorf("nts: reconstructing request associated data: %w", err)
	}

	// Verify the request authenticator, keyed with the client-to-server key.
	c2sAEAD, err := nts.NewAEAD(aeadID, c2s)
	if err != nil {
		return fmt.Errorf("nts: building C2S AEAD: %w", err)
	}
	if _, err := nts.OpenAuthenticator(c2sAEAD, reqAD, *authEF); err != nil {
		return fmt.Errorf("%w: %w", ErrAuthVerify, err)
	}

	// Mint fresh cookies: one to replace the cookie the client just spent, plus
	// one for each placeholder it sent.
	n := min(1+placeholderCount, maxResponseCookies)
	cookieEFs := make([]ntp.ExtensionField, 0, n)
	for range n {
		cookie, err := ks.SealCookie(aeadID, c2s, s2c)
		if err != nil {
			return fmt.Errorf("nts: sealing cookie: %w", err)
		}
		cookieEFs = append(cookieEFs, nts.NewCookie(cookie))
	}

	// The fresh cookies travel encrypted inside the authenticator (RFC 8915
	// §5.7); the UniqueID echo travels in the clear ahead of it (§5.6).
	encEFs, err := ntp.MarshalExtensionFields(cookieEFs)
	if err != nil {
		return fmt.Errorf("nts: marshaling cookie extension fields: %w", err)
	}

	// Put the UniqueID echo on the response, then take the associated data
	// straight from the response packet (header + echo) so it matches the final
	// wire bytes exactly. The authenticator is appended after sealing, so the
	// signed prefix stays byte-identical to what Bytes() emits.
	resp.ExtensionFields = []ntp.ExtensionField{nts.NewUniqueIdentifier(uidBody)}
	ad, err := resp.AssociatedData(1)
	if err != nil {
		return fmt.Errorf("nts: reconstructing response associated data: %w", err)
	}

	s2cAEAD, err := nts.NewAEAD(aeadID, s2c)
	if err != nil {
		return fmt.Errorf("nts: building S2C AEAD: %w", err)
	}
	auth, err := nts.SealAuthenticator(s2cAEAD, ad, encEFs)
	if err != nil {
		return fmt.Errorf("nts: sealing response authenticator: %w", err)
	}
	resp.ExtensionFields = append(resp.ExtensionFields, auth)
	return nil
}
