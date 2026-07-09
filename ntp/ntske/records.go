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
	"encoding/binary"
	"errors"
	"fmt"
)

/*
RFC 8915 §4.1.2.  NTS-KE Record wire format

    0                   1                   2                   3
    0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1

+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|C|        Record Type (15 bit)  |        Body Length (16 bit)  |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                        Record Body ...                       |

- Critical bit (C): the most significant bit of the first 16-bit word. If
    it is set and the receiver does not recognize the record type, it MUST
    abort. The encoder ORs it into the type field (type | 0x8000); the
    decoder extracts it (raw & 0x8000) and recovers the type (raw & 0x7FFF).

- Body Length: 16-bit big-endian, the length of the body in octets.

- Body: the raw octets. Unlike NTP extension fields, NTS-KE records are
    NOT padded to a 4-octet boundary — the length is exact.
*/

type Record struct {
	Critical bool
	Type     uint16
	Body     []byte
}

const (
	// Record type numbers
	RecordEndOfMessage      uint16 = 0
	RecordNextProtocol      uint16 = 1
	RecordError             uint16 = 2
	RecordWarning           uint16 = 3
	RecordAEADAlgorithm     uint16 = 4
	RecordNewCookie         uint16 = 5
	RecordServerNegotiation uint16 = 6
	RecordPortNegotiation   uint16 = 7
	// RecordCompliant128GCMExport  negotiates AES-128-GCM-SIV compliant export. It
	// is a non-standard (IANA-unassigned) type; non-critical, with an empty body.
	RecordCompliant128GCMExport uint16 = 1024
)

const (
	recordHeaderLen = 4      // Critical+Type (2 octets) + Body Length (2 octets)
	criticalBit     = 0x8000 // MSB of the first 16-bit word
	typeMask        = 0x7FFF // the remaining 15 bits hold the type
	maxBodyLen      = 0xFFFF // largest value the 16-bit Body Length can hold
)

// Sentinel errors returned by Parse. Compare with errors.Is.
var (
	ErrHeaderTruncated = errors.New("ntske: record header truncated")
	ErrBodyTruncated   = errors.New("ntske: record body truncated")
	ErrBodyTooLarge    = errors.New("ntske: record body exceeds 16-bit length field")
	ErrTypeTooLarge    = errors.New("ntske: record type exceeds 15-bit field")
	ErrOddLengthBody   = errors.New("ntske: odd-length uint16 body")
)

// Marshal encodes a single record into its wire format (RFC 8915 §4).
func (r Record) Marshal() ([]byte, error) {
	if len(r.Body) > maxBodyLen {
		return nil, fmt.Errorf("%w: type=%d body=%d max=%d",
			ErrBodyTooLarge, r.Type, len(r.Body), maxBodyLen)
	}
	if r.Type > typeMask {
		return nil, fmt.Errorf("%w: type=%d max=%d", ErrTypeTooLarge, r.Type, typeMask)
	}
	out := make([]byte, recordHeaderLen+len(r.Body))
	typeWord := r.Type & typeMask
	if r.Critical {
		typeWord |= criticalBit
	}
	binary.BigEndian.PutUint16(out[0:2], typeWord)
	binary.BigEndian.PutUint16(out[2:4], uint16(len(r.Body))) // #nosec G115 -- bounded by maxBodyLen check above
	copy(out[recordHeaderLen:], r.Body)
	return out, nil
}

// MarshalRecords concatenates the wire encodings of multiple records, in order.
func MarshalRecords(records []Record) ([]byte, error) {
	total := 0
	for _, r := range records {
		if len(r.Body) > maxBodyLen {
			return nil, fmt.Errorf("%w: type=%d body=%d max=%d",
				ErrBodyTooLarge, r.Type, len(r.Body), maxBodyLen)
		}
		if r.Type > typeMask {
			return nil, fmt.Errorf("%w: type=%d max=%d", ErrTypeTooLarge, r.Type, typeMask)
		}
		total += recordHeaderLen + len(r.Body)
	}
	out := make([]byte, total)
	off := 0
	for _, r := range records {
		// Write header + body directly into out to avoid per-record allocation
		// and duplicate validation (validation already done in pre-pass above).
		typeWord := r.Type & typeMask
		if r.Critical {
			typeWord |= criticalBit
		}
		binary.BigEndian.PutUint16(out[off:off+2], typeWord)
		binary.BigEndian.PutUint16(out[off+2:off+4], uint16(len(r.Body))) // #nosec G115 -- bounded by maxBodyLen check above
		copy(out[off+recordHeaderLen:], r.Body)
		off += recordHeaderLen + len(r.Body)
	}
	return out, nil
}

// Parse decodes a buffer of concatenated NTS-KE records (RFC 8915 §4). Bodies
// are copied out of b so the returned records stay valid if the caller reuses
// or mutates b. Parse performs no semantic validation (e.g. presence of an End
// of Message record, or rejection of unrecognized critical records); that is
// the responsibility of the layer that consumes the records.
func Parse(b []byte) ([]Record, error) {
	var records []Record
	for len(b) > 0 {
		if len(b) < recordHeaderLen {
			return nil, fmt.Errorf("%w: need %d header bytes, have %d",
				ErrHeaderTruncated, recordHeaderLen, len(b))
		}
		typeWord := binary.BigEndian.Uint16(b[0:2])
		bodyLen := int(binary.BigEndian.Uint16(b[2:4]))
		if recordHeaderLen+bodyLen > len(b) {
			return nil, fmt.Errorf("%w: need %d body bytes, have %d",
				ErrBodyTruncated, bodyLen, len(b)-recordHeaderLen)
		}
		body := make([]byte, bodyLen)
		if bodyLen > 0 {
			copy(body, b[recordHeaderLen:recordHeaderLen+bodyLen])
		}

		records = append(records, Record{
			Critical: typeWord&criticalBit != 0,
			Type:     typeWord & typeMask,
			Body:     body,
		})
		b = b[recordHeaderLen+bodyLen:]
	}
	return records, nil
}

// --- Constructor helpers, one per record type ---
// Critical-bit defaults follow the spec for a client building a request: types
// 0-3 MUST set it; types 4-7 default to unset (MAY / SHOULD NOT). Callers that
// build server responses can override Critical on the returned record.
// NewEndOfMessage returns the End of Message record : empty
// body, Critical Bit set.
func NewEndOfMessage() Record {
	return Record{Critical: true, Type: RecordEndOfMessage, Body: []byte{}}
}

// NewNextProtocol returns a Next Protocol Negotiation record carrying the given
// protocol IDs. The Critical Bit MUST be set.
func NewNextProtocol(protocolIDs ...uint16) Record {
	return Record{Critical: true, Type: RecordNextProtocol, Body: MarshalUint16s(protocolIDs)}
}

// NewError returns an Error record carrying the given error code
// The Critical Bit MUST be set.
func NewError(code uint16) Record {
	return Record{Critical: true, Type: RecordError, Body: MarshalUint16s([]uint16{code})}
}

// NewWarning returns a Warning record carrying the given warning code
// The Critical Bit MUST be set.
func NewWarning(code uint16) Record {
	return Record{Critical: true, Type: RecordWarning, Body: MarshalUint16s([]uint16{code})}
}

// NewAEADAlgorithm returns an AEAD Algorithm Negotiation record carrying the
// given algorithm IDs. The Critical Bit MAY be set; it is
// left unset here.
func NewAEADAlgorithm(algorithmIDs ...uint16) Record {
	return Record{Type: RecordAEADAlgorithm, Body: MarshalUint16s(algorithmIDs)}
}

// NewCookie returns a New Cookie for NTPv4 record carrying the opaque cookie
// The Critical Bit SHOULD NOT be set.
func NewCookie(cookie []byte) Record {
	b := append([]byte(nil), cookie...)
	return Record{Type: RecordNewCookie, Body: b}
}

// NewServerNegotiation returns an NTPv4 Server Negotiation record carrying the
// ASCII server address or hostname (RFC 8915 4.1.7).
func NewServerNegotiation(server string) Record {
	return Record{Type: RecordServerNegotiation, Body: []byte(server)}
}

// NewPortNegotiation returns an NTPv4 Port Negotiation record carrying the UDP
// port number (RFC 8915 §4.1.8).
func NewPortNegotiation(port uint16) Record {
	return Record{Type: RecordPortNegotiation, Body: MarshalUint16s([]uint16{port})}
}

// NewCompliant128GCMExport returns an AES-128-GCM-SIV compliant-export record.
// The Critical Bit is not set and the body is empty.
func NewCompliant128GCMExport() Record {
	return Record{Type: RecordCompliant128GCMExport, Body: []byte{}}
}

// MarshalUint16s encodes a sequence of 16-bit integers in network byte order.
func MarshalUint16s(values []uint16) []byte {
	out := make([]byte, 2*len(values))
	for i, v := range values {
		binary.BigEndian.PutUint16(out[2*i:], v)
	}
	return out
}

// ParseUint16s decodes a body of packed 16-bit network-order integers.
// Returns ErrOddLengthBody if len is odd — faulty NTS-KE body per RFC 8915.
func ParseUint16s(b []byte) ([]uint16, error) {
	if len(b)%2 != 0 {
		return nil, ErrOddLengthBody
	}
	out := make([]uint16, len(b)/2)
	for i := 0; i < len(b); i += 2 {
		out[i/2] = binary.BigEndian.Uint16(b[i:])
	}
	return out, nil
}
