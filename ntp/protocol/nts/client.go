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
	"errors"
	"fmt"

	"github.com/facebook/time/ntp/protocol"
)

// RequestParams is everything a client needs to build one NTS-protected request.
type RequestParams struct {
	AEAD         protocol.AEADAlgorithm
	C2S          []byte // client-to-server key from the KE handshake
	Cookie       []byte // one cookie to spend
	UniqueID     []byte // fresh nonce, >= MinUniqueIdentifierLen octets
	Placeholders int    // NTSCookiePlaceholder EFs requesting extra fresh cookies
}

// BuildNTSRequest returns wire-ready bytes for an NTS-protected NTPv4 request:
// header + UID -> Cookie -> N placeholders -> Authenticator (sealed with C2S).
func BuildNTSRequest(base protocol.Packet, p RequestParams) ([]byte, error) {
	if len(p.UniqueID) < MinUniqueIdentifierLen {
		return nil, fmt.Errorf("nts: unique identifier must be >= %d octets", MinUniqueIdentifierLen)
	}
	if len(p.Cookie) == 0 {
		return nil, errors.New("nts: cookie is required")
	}
	if p.Placeholders < 0 {
		return nil, errors.New("nts: placeholders must be >= 0")
	}
	aead, err := NewAEAD(p.AEAD, p.C2S)
	if err != nil {
		return nil, fmt.Errorf("nts: building C2S AEAD: %w", err)
	}

	efs := make([]protocol.ExtensionField, 0, 3+p.Placeholders)
	efs = append(efs, NewUniqueIdentifier(p.UniqueID), NewCookie(p.Cookie))
	for range p.Placeholders {
		efs = append(efs, NewCookiePlaceholder(len(p.Cookie)))
	}
	base.ExtensionFields = efs

	// Associated data is the header plus every EF preceding the authenticator.
	ad, err := base.AssociatedData(len(efs))
	if err != nil {
		return nil, fmt.Errorf("nts: reconstructing request associated data: %w", err)
	}
	// A request carries no encrypted extension fields, so the plaintext is empty.
	auth, err := SealAuthenticator(aead, ad, nil)
	if err != nil {
		return nil, fmt.Errorf("nts: sealing request authenticator: %w", err)
	}
	base.ExtensionFields = append(base.ExtensionFields, auth)
	return base.Bytes()
}

// VerifyNTSResponse checks the echoed UniqueIdentifier matches reqUID, verifies
// the authenticator with S2C, and returns the packet plus the fresh cookies.
func VerifyNTSResponse(resp []byte, aeadID protocol.AEADAlgorithm, s2c, reqUID []byte) (*protocol.Packet, [][]byte, error) {
	pkt := &protocol.Packet{}
	if err := pkt.UnmarshalBinary(resp); err != nil {
		return nil, nil, fmt.Errorf("nts: parsing response: %w", err)
	}

	var (
		uidBody []byte
		authEF  *protocol.ExtensionField
		authIdx = -1
	)
	for i := range pkt.ExtensionFields {
		ef := &pkt.ExtensionFields[i]
		if ef.Type == protocol.UniqueIdentifier {
			uidBody = ef.Body
		}
		if ef.Type == protocol.NTSAuthenticator {
			authEF = ef
			authIdx = i
			break
		}
	}
	switch {
	case uidBody == nil:
		return nil, nil, errors.New("nts: response missing unique identifier")
	case !bytes.Equal(uidBody, reqUID):
		return nil, nil, errors.New("nts: response unique identifier does not match request")
	case authEF == nil:
		return nil, nil, errors.New("nts: response missing authenticator")
	case authIdx != len(pkt.ExtensionFields)-1:
		// RFC 8915 §5.6: the authenticator must be the last EF, otherwise any
		// trailing extension fields would be returned unauthenticated.
		return nil, nil, errors.New("nts: authenticator must be the last extension field")
	}

	ad, err := pkt.AssociatedData(authIdx)
	if err != nil {
		return nil, nil, fmt.Errorf("nts: reconstructing response associated data: %w", err)
	}
	aead, err := NewAEAD(aeadID, s2c)
	if err != nil {
		return nil, nil, fmt.Errorf("nts: building S2C AEAD: %w", err)
	}
	// The fresh cookies travel encrypted inside the authenticator (RFC 8915 §5.7).
	plaintext, err := OpenAuthenticator(aead, ad, *authEF)
	if err != nil {
		return nil, nil, fmt.Errorf("nts: verifying response authenticator: %w", err)
	}
	encEFs, err := protocol.ParseExtensionFields(plaintext)
	if err != nil {
		return nil, nil, fmt.Errorf("nts: parsing encrypted cookies: %w", err)
	}
	var cookies [][]byte
	for _, ef := range encEFs {
		if ef.Type == protocol.NTSCookie {
			cookies = append(cookies, ef.Body)
		}
	}
	return pkt, cookies, nil
}
