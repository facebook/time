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
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMarshalParseRoundTrip exercises the critical-bit set/unset paths and
// confirms a record survives a Marshal -> Parse round trip unchanged.
func TestMarshalParseRoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		record   Record
		wantWire []byte
	}{
		{
			name:     "critical bit set, empty body",
			record:   Record{Critical: true, Type: RecordEndOfMessage, Body: []byte{}},
			wantWire: []byte{0x80, 0x00, 0x00, 0x00},
		},
		{
			name:     "critical bit unset, empty body",
			record:   Record{Critical: false, Type: RecordNewCookie, Body: []byte{}},
			wantWire: []byte{0x00, 0x05, 0x00, 0x00},
		},
		{
			name:     "critical bit set, non-empty body",
			record:   Record{Critical: true, Type: RecordNextProtocol, Body: []byte{0x00, 0x00}},
			wantWire: []byte{0x80, 0x01, 0x00, 0x02, 0x00, 0x00},
		},
		{
			name:     "critical bit unset, non-empty body",
			record:   Record{Critical: false, Type: RecordServerNegotiation, Body: []byte("ab")},
			wantWire: []byte{0x00, 0x06, 0x00, 0x02, 'a', 'b'},
		},
		{
			name:     "high record type does not collide with critical bit",
			record:   Record{Critical: false, Type: 0x7FFF, Body: []byte{}},
			wantWire: []byte{0x7f, 0xff, 0x00, 0x00},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wire, err := tc.record.Marshal()
			require.NoError(t, err)
			require.Equal(t, tc.wantWire, wire)

			got, err := Parse(wire)
			require.NoError(t, err)
			require.Len(t, got, 1)
			require.Equal(t, tc.record.Critical, got[0].Critical)
			require.Equal(t, tc.record.Type, got[0].Type)
			require.Equal(t, tc.record.Body, got[0].Body)
		})
	}
}

// TestParseMultiRecordBuffer confirms several concatenated records decode back
// into the original sequence, in order.
func TestParseMultiRecordBuffer(t *testing.T) {
	records := []Record{
		NewNextProtocol(0),
		NewAEADAlgorithm(15),
		NewCookie([]byte{0xde, 0xad, 0xbe, 0xef}),
		NewEndOfMessage(),
	}
	wire, err := MarshalRecords(records)
	require.NoError(t, err)

	got, err := Parse(wire)
	require.NoError(t, err)
	require.Equal(t, records, got)
}

// TestParseEmptyBuffer confirms an empty buffer yields no records and no error.
func TestParseEmptyBuffer(t *testing.T) {
	got, err := Parse(nil)
	require.NoError(t, err)
	require.Empty(t, got)
}

// TestParseTruncatedHeader rejects buffers with fewer than recordHeaderLen bytes.
func TestParseTruncatedHeader(t *testing.T) {
	cases := []struct {
		name string
		wire []byte
	}{
		{"one byte", []byte{0x80}},
		{"three bytes", []byte{0x80, 0x01, 0x00}},
		{"trailing partial header after a valid record",
			[]byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x01}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.wire)
			require.ErrorIs(t, err, ErrHeaderTruncated)
		})
	}
}

// TestParseTruncatedBody rejects buffers whose declared body length exceeds the
// bytes actually present.
func TestParseTruncatedBody(t *testing.T) {
	cases := []struct {
		name string
		wire []byte
	}{
		{"declares 4 body bytes, has none",
			[]byte{0x00, 0x05, 0x00, 0x04}},
		{"declares 8 body bytes, has 2",
			[]byte{0x00, 0x05, 0x00, 0x08, 0xaa, 0xbb}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.wire)
			require.ErrorIs(t, err, ErrBodyTruncated)
		})
	}
}

// TestMarshalBodyLengthOverflow rejects bodies larger than the 16-bit length
// field can represent.
func TestMarshalBodyLengthOverflow(t *testing.T) {
	r := Record{Type: RecordNewCookie, Body: make([]byte, maxBodyLen+1)}
	_, err := r.Marshal()
	require.ErrorIs(t, err, ErrBodyTooLarge)

	// A body exactly at the limit is accepted.
	r.Body = make([]byte, maxBodyLen)
	_, err = r.Marshal()
	require.NoError(t, err)

	// MarshalRecords surfaces the same error from an offending record.
	_, err = MarshalRecords([]Record{
		NewEndOfMessage(),
		{Type: RecordNewCookie, Body: make([]byte, maxBodyLen+1)},
	})
	require.ErrorIs(t, err, ErrBodyTooLarge)
}

// EndOfMessage (type 0, empty body, critical) followed by another record:
// check the stream advance correctly after the first record
func TestParseZeroLengthBodyMidStream(t *testing.T) {
	rec := Record{Critical: true, Type: RecordPortNegotiation, Body: []byte{0x01, 0x23}}
	buf, err := MarshalRecords([]Record{NewEndOfMessage(), rec})
	require.NoError(t, err)

	got, err := Parse(buf)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, RecordEndOfMessage, got[0].Type)
	require.Empty(t, got[0].Body)
	require.Equal(t, rec.Body, got[1].Body)
}

// TestConstructors covers all eight defined record-type constructor helpers,
// checking the record type, critical bit, and body encoding of each.
func TestConstructors(t *testing.T) {
	cases := []struct {
		name         string
		record       Record
		wantType     uint16
		wantCritical bool
		wantBody     []byte
	}{
		{"end of message", NewEndOfMessage(), RecordEndOfMessage, true, []byte{}},
		{"next protocol", NewNextProtocol(0, 1), RecordNextProtocol, true, []byte{0x00, 0x00, 0x00, 0x01}},
		{"error", NewError(1), RecordError, true, []byte{0x00, 0x01}},
		{"warning", NewWarning(2), RecordWarning, true, []byte{0x00, 0x02}},
		{"aead algorithm", NewAEADAlgorithm(15), RecordAEADAlgorithm, false, []byte{0x00, 0x0f}},
		{"new cookie", NewCookie([]byte{0xab, 0xcd}), RecordNewCookie, false, []byte{0xab, 0xcd}},
		{"server negotiation", NewServerNegotiation("ntp.example"), RecordServerNegotiation, false, []byte("ntp.example")},
		{"port negotiation", NewPortNegotiation(123), RecordPortNegotiation, false, []byte{0x00, 0x7b}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.wantType, tc.record.Type)
			require.Equal(t, tc.wantCritical, tc.record.Critical)
			require.Equal(t, tc.wantBody, tc.record.Body)

			// Every constructor must produce a record that round-trips.
			wire, err := tc.record.Marshal()
			require.NoError(t, err)
			got, err := Parse(wire)
			require.NoError(t, err)
			require.Len(t, got, 1)
			require.Equal(t, tc.record.Type, got[0].Type)
			require.Equal(t, tc.record.Critical, got[0].Critical)
		})
	}
}
