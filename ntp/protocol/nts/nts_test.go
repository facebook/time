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
	"testing"

	"github.com/facebook/time/ntp/protocol"
	"github.com/stretchr/testify/require"
)

// TestConstructorsRoundTripThroughExtensionFramework verifies that the NTS
// constructors produce extension fields that survive a full
// MarshalExtensionFields → ParseExtensionFields round-trip with the correct
// types preserved on the wire. This is the production use case: the client
// builds an EF sequence into an NTP request, the server parses and dispatches
// by type. Validates the protocol contract end-to-end rather than the trivial
// struct-field assignments the constructors do.
func TestConstructorsRoundTripThroughExtensionFramework(t *testing.T) {
	nonce := make([]byte, MinUniqueIdentifierLen)
	_, err := rand.Read(nonce)
	require.NoError(t, err)

	cookie := []byte("opaque-server-cookie-bytes-0001")
	const placeholderLen = 64

	efs := []protocol.ExtensionField{
		NewUniqueIdentifier(nonce),
		NewCookie(cookie),
		NewCookiePlaceholder(placeholderLen),
	}

	wire, err := protocol.MarshalExtensionFields(efs)
	require.NoError(t, err)
	parsed, err := protocol.ParseExtensionFields(wire)
	require.NoError(t, err)
	require.Len(t, parsed, 3)

	// Types preserved on the wire — this is the dispatch contract the
	// responder relies on to route EFs by NTS field type.
	require.Equal(t, protocol.UniqueIdentifier, parsed[0].Type)
	require.Equal(t, protocol.NTSCookie, parsed[1].Type)
	require.Equal(t, protocol.NTSCookiePlaceholder, parsed[2].Type)

	// UID nonce round-trips exactly (32 octets is already 4-aligned, no padding).
	require.Equal(t, nonce, parsed[0].Body)

	// Cookie value preserved as a prefix of the parsed body — the parsed body
	// may carry trailing zero padding (see ExtensionField godoc on
	// value-vs-padding ambiguity).
	require.True(t, bytes.HasPrefix(parsed[1].Body, cookie))
}

// TestCookiePlaceholderSizeIsTheMessage verifies the RFC 8915 §5.5 contract:
// a Cookie Placeholder EF signals to the server "issue me a cookie of THIS
// size" via the body length. The body contents are irrelevant; what matters
// is that the length round-trips through the EF framework so the server can
// read it back to determine the cookie size to mint.
func TestCookiePlaceholderSizeIsTheMessage(t *testing.T) {
	// Sizes the server would actually see (matches the chrony-compatible
	// cookie format: one entry per supported AEAD algorithm).
	for _, size := range []int{68, 100, 148, 164} {
		ef := NewCookiePlaceholder(size)

		wire, err := protocol.MarshalExtensionFields([]protocol.ExtensionField{ef})
		require.NoError(t, err)
		parsed, err := protocol.ParseExtensionFields(wire)
		require.NoError(t, err)
		require.Len(t, parsed, 1)
		require.Equal(t, protocol.NTSCookiePlaceholder, parsed[0].Type)
		// Server reads len(parsed[0].Body) to determine the cookie size to
		// issue. All our supported cookie sizes are 4-aligned so the wire
		// length is exact (no padding stripping needed).
		require.Len(t, parsed[0].Body, size)
	}
}
