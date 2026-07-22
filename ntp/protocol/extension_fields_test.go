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

package protocol

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodedSize(t *testing.T) {
	cases := []struct {
		name     string
		body     []byte
		expected int
	}{
		{"empty body rounds up to minimum", nil, 4},
		{"4-byte body produces 8 octets", make([]byte, 4), 8},
		{"5-byte body pads to 12 octets", make([]byte, 5), 12},
		{"7-byte body pads to 12 octets", make([]byte, 7), 12},
		{"8-byte body produces 12 octets", make([]byte, 8), 12},
		{"32-byte body produces 36 octets", make([]byte, 32), 36},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ef := ExtensionField{Type: 0x0104, Body: tc.body}
			require.Equal(t, tc.expected, ef.EncodedSize())
		})
	}
}

func TestMarshalSingleField(t *testing.T) {
	ef := ExtensionField{Type: 0x0104, Body: bytes.Repeat([]byte{0xab}, 8)}
	wire, err := ef.Encode()
	require.NoError(t, err)
	require.Len(t, wire, 12)
	require.Equal(t, byte(0x01), wire[0])
	require.Equal(t, byte(0x04), wire[1])
	require.Equal(t, byte(0x00), wire[2])
	require.Equal(t, byte(0x0c), wire[3])
	require.Equal(t, bytes.Repeat([]byte{0xab}, 8), wire[4:])
}

func TestMarshalSingleFieldPadded(t *testing.T) {
	// 5-byte body should pad to 12 total octets (4 header + 5 body + 3 padding).
	ef := ExtensionField{Type: 0x0204, Body: []byte{1, 2, 3, 4, 5}}
	wire, err := ef.Encode()
	require.NoError(t, err)
	require.Len(t, wire, 12)
	require.Equal(t, byte(0x02), wire[0])
	require.Equal(t, byte(0x04), wire[1])
	require.Equal(t, byte(0x00), wire[2])
	require.Equal(t, byte(0x0c), wire[3])
	require.Equal(t, []byte{1, 2, 3, 4, 5, 0, 0, 0}, wire[4:])
}

func TestMarshalMultipleFields(t *testing.T) {
	efs := []ExtensionField{
		{Type: 0x0104, Body: bytes.Repeat([]byte{0xaa}, 4)},
		{Type: 0x0204, Body: bytes.Repeat([]byte{0xbb}, 8)},
	}
	wire, err := MarshalExtensionFields(efs)
	require.NoError(t, err)
	// First field: 4 + 4 = 8; second: 4 + 8 = 12; total 20.
	require.Len(t, wire, 20)
}

func TestMarshalEmptyBody(t *testing.T) {
	ef := ExtensionField{Type: 0x0304}
	wire, err := ef.Encode()
	require.NoError(t, err)
	require.Equal(t, []byte{0x03, 0x04, 0x00, 0x04}, wire)
}

func TestMarshalRejectsOversizedBody(t *testing.T) {
	ef := ExtensionField{Type: 0x0104, Body: make([]byte, ExtensionMaxBodySize+1)}
	_, err := ef.Encode()
	require.ErrorIs(t, err, ErrExtensionBodyTooLarge)
}

// TestMarshalAtMaxBodySize exercises the boundary at ExtensionMaxBodySize.
// Verifies the largest allowed body marshals to a wire length that fits in
// uint16 and round-trips correctly — guards against the silent uint16 wrap
// that occurred when the bound was 0xFFFF-header (allowing bodies that
// padded up to 0x10000 on encode).
func TestMarshalAtMaxBodySize(t *testing.T) {
	body := bytes.Repeat([]byte{0xcd}, ExtensionMaxBodySize)
	ef := ExtensionField{Type: 0x0104, Body: body}
	wire, err := ef.Encode()
	require.NoError(t, err)
	require.Len(t, wire, 0xFFFC, "wire length must be 0xFFFC at max body")
	require.Equal(t, byte(0xFF), wire[2])
	require.Equal(t, byte(0xFC), wire[3])

	parsed, err := ParseExtensionFields(wire)
	require.NoError(t, err)
	require.Len(t, parsed, 1)
	require.Equal(t, ExtensionFieldType(0x0104), parsed[0].Type)
	require.Equal(t, body, parsed[0].Body)
}

func TestParseSingleField(t *testing.T) {
	wire := []byte{0x01, 0x04, 0x00, 0x0c, 1, 2, 3, 4, 5, 6, 7, 8}
	efs, err := ParseExtensionFields(wire)
	require.NoError(t, err)
	require.Len(t, efs, 1)
	require.Equal(t, ExtensionFieldType(0x0104), efs[0].Type)
	require.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}, efs[0].Body)
}

func TestParseMultipleFields(t *testing.T) {
	first, err := (ExtensionField{Type: 0x0104, Body: bytes.Repeat([]byte{0xaa}, 8)}).Encode()
	require.NoError(t, err)
	second, err := (ExtensionField{Type: 0x0204, Body: bytes.Repeat([]byte{0xbb}, 16)}).Encode()
	require.NoError(t, err)
	wire := append(first, second...)

	efs, err := ParseExtensionFields(wire)
	require.NoError(t, err)
	require.Len(t, efs, 2)
	require.Equal(t, ExtensionFieldType(0x0104), efs[0].Type)
	require.Equal(t, ExtensionFieldType(0x0204), efs[1].Type)
}

func TestParseEmptyBuffer(t *testing.T) {
	efs, err := ParseExtensionFields(nil)
	require.NoError(t, err)
	require.Empty(t, efs)
}

func TestParseRejectsTruncatedHeader(t *testing.T) {
	_, err := ParseExtensionFields([]byte{0x01, 0x04, 0x00})
	require.ErrorIs(t, err, ErrExtensionTruncated)
}

func TestParseRejectsLengthBelowMinimum(t *testing.T) {
	_, err := ParseExtensionFields([]byte{0x01, 0x04, 0x00, 0x00})
	require.ErrorIs(t, err, ErrExtensionLengthInvalid)
}

func TestParseRejectsLengthNotMultipleOf4(t *testing.T) {
	_, err := ParseExtensionFields([]byte{0x01, 0x04, 0x00, 0x06, 0, 0})
	require.ErrorIs(t, err, ErrExtensionLengthInvalid)
}

func TestParseRejectsLengthExceedingBuffer(t *testing.T) {
	// Length advertises 16 octets but only 8 are present.
	_, err := ParseExtensionFields([]byte{0x01, 0x04, 0x00, 0x10, 1, 2, 3, 4})
	require.ErrorIs(t, err, ErrExtensionTruncated)
}

func TestRoundTripPreservesValueAndPadding(t *testing.T) {
	original := []ExtensionField{
		{Type: 0x0104, Body: []byte{1, 2, 3, 4, 5, 6, 7, 8}},
		{Type: 0x0204, Body: []byte{0xff}},
		{Type: 0x0304},
	}
	wire, err := MarshalExtensionFields(original)
	require.NoError(t, err)
	parsed, err := ParseExtensionFields(wire)
	require.NoError(t, err)
	require.Len(t, parsed, len(original))
	for i, ef := range original {
		require.Equal(t, ef.Type, parsed[i].Type, "field %d type", i)
		// First len(ef.Body) bytes of parsed Body equal original Body; trailing bytes are zero padding.
		require.True(t, bytes.Equal(ef.Body, parsed[i].Body[:len(ef.Body)]), "field %d value", i)
		for j := len(ef.Body); j < len(parsed[i].Body); j++ {
			require.Equal(t, byte(0), parsed[i].Body[j], "field %d padding byte %d not zero", i, j)
		}
	}
}

func TestExtensionFieldTypeString(t *testing.T) {
	cases := []struct {
		name string
		typ  ExtensionFieldType
		want string
	}{
		{"unique identifier", 0x0104, "Unique Identifier"},
		{"nts cookie", 0x0204, "NTS Cookie"},
		{"nts cookie placeholder", 0x0304, "NTS Cookie Placeholder"},
		{"nts authenticator", 0x0404, "NTS Authenticator and Encrypted Extension Fields"},
		{"unknown type", 0xABCD, "Unknown(0xabcd)"},
		{"zero", 0x0000, "Unknown(0x0000)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.typ.String())
		})
	}
}

func TestExtensionFieldValidate(t *testing.T) {
	cases := []struct {
		name    string
		ef      ExtensionField
		wantErr error
	}{
		{"valid nts type", ExtensionField{Type: 0x0104, Body: []byte{1, 2}}, nil},
		{"body at max", ExtensionField{Type: 0x0104, Body: make([]byte, ExtensionMaxBodySize)}, nil},
		{"body over max", ExtensionField{Type: 0x0104, Body: make([]byte, ExtensionMaxBodySize+1)}, ErrExtensionBodyTooLarge},
		{"reserved low bound", ExtensionField{Type: 0xF000}, ErrExtensionTypeReserved},
		{"reserved high bound", ExtensionField{Type: 0xFFFF}, ErrExtensionTypeReserved},
		{"just below reserved", ExtensionField{Type: 0xEFFF}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.ef.validate()
			if tc.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}

func TestMarshalExtensionFieldsRejectsInvalid(t *testing.T) {
	_, err := MarshalExtensionFields([]ExtensionField{{Type: 0xF000}})
	require.ErrorIs(t, err, ErrExtensionTypeReserved)
}

// TestAEADAlgorithmValues pins the negotiated AEAD identifiers to their IANA
// "AEAD Algorithms" registry values; drift here would silently break NTS-KE
// interop with peers.
func TestAEADAlgorithmValues(t *testing.T) {
	require.Equal(t, AEADAlgorithm(17), AEADAESSIVCMAC512)
	require.Equal(t, AEADAlgorithm(30), AEADAES128GCMSIV)
}

// FuzzEFRoundTrip locks the parse/encode mutual-inverse that NTS request auth
// relies on: the server reconstructs the signed bytes by re-serializing the
// parsed EFs (Packet.AssociatedData / encodeExtensionFields), so encode(parse(b))
// MUST equal b byte-for-byte for every buffer that parses.
func FuzzEFRoundTrip(f *testing.F) {
	// Seeds: empty, a single known EF, a reserved-type EF with non-zero body.
	f.Add([]byte{})
	f.Add([]byte{0x01, 0x04, 0x00, 0x08, 1, 2, 3, 4})
	f.Add([]byte{0xF0, 0x01, 0x00, 0x08, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, b []byte) {
		efs, err := ParseExtensionFields(b)
		if err != nil {
			return // only well-formed buffers are subject to the identity
		}
		reenc, err := encodeExtensionFields(efs)
		require.NoError(t, err)
		require.Equal(t, b, reenc) // NTS verification depends on this identity
	})
}
