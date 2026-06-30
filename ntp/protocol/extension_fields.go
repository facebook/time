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
	"encoding/binary"
	"errors"
	"fmt"
)

/*
NTP Extension Field framing per RFC 7822 §7.5.

	0                   1                   2                   3
	0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
  +-------------------------------+-------------------------------+
  |          Field Type           |            Length             |
  +-------------------------------+-------------------------------+
  |                                                               |
  .                            Value                              .
  .                                                               .
  |                                                               |
  +---------------------------------------------------------------+
  |                       Padding (as needed)                     |
  +---------------------------------------------------------------+

  - Field Type: 16-bit IANA-assigned Extension Field type
  - Length:     16-bit total length in octets of {type, length, value, padding},
                MUST be a multiple of 4 octets
  - Value:      variable-length payload
  - Padding:    zero octets so that the total length is a multiple of 4

For NTS (RFC 8915 §5.1) the minimum total length is 4 octets, which overrides
the 16-octet minimum from RFC 7822 §7.5.1.2. We follow the NTS profile.
*/

// ExtensionFieldType is the 16-bit IANA-assigned NTP extension field type
// (RFC 7822 §7.5 and the IANA "NTP Extension Field Types" registry).
type ExtensionFieldType uint16

// ExtensionField is a parsed NTP extension field. Body holds the value plus
// any padding bytes as they appeared on the wire; the caller is responsible
// for interpreting the value boundary based on the field type.
type ExtensionField struct {
	Type ExtensionFieldType
	Body []byte
}

const (
	// ExtensionHeaderSize is the size of the field-type+length prefix in octets.
	ExtensionHeaderSize = 4
	// ExtensionMinSize is the minimum total length of an extension field in octets,
	// per RFC 8915 §5.1 (NTS profile).
	ExtensionMinSize = 4
	// ExtensionAlignment is the field-length alignment in octets per RFC 7822 §7.5.
	ExtensionAlignment = 4
	// ExtensionMaxBodySize is the largest body that, after prepending the
	// 4-octet header and rounding up to a 4-octet alignment boundary, still
	// fits in the 16-bit length field. The largest aligned total that fits
	// in uint16 is 0xFFFC; subtracting the header gives the body bound.
	// Bodies larger than this would silently wrap the uint16 length on the
	// wire even though they pass a naïve `< 0xFFFF` check.
	ExtensionMaxBodySize = 0xFFFC - ExtensionHeaderSize
)

// NTS extension field types defined by RFC 8915 §5.1.1.
const (
	UniqueIdentifier     ExtensionFieldType = 0x0104
	NTSCookie            ExtensionFieldType = 0x0204
	NTSCookiePlaceholder ExtensionFieldType = 0x0304
	NTSAuthenticator     ExtensionFieldType = 0x0404
)

// Sentinel errors returned by extension-field parsing.
var (
	ErrExtensionTruncated     = errors.New("extension field truncated")
	ErrExtensionLengthInvalid = errors.New("extension field length invalid")
	ErrExtensionBodyTooLarge  = errors.New("extension field body too large")
)

// padTo returns the smallest multiple of align >= n. align must be a power of two.
func padTo(n, align int) int {
	return (n + align - 1) &^ (align - 1)
}

// EncodedSize returns the wire-format size in octets, including padding, that
// MarshalExtensionFields would emit for this field.
func (ef ExtensionField) EncodedSize() int {
	total := padTo(ExtensionHeaderSize+len(ef.Body), ExtensionAlignment)
	return max(total, ExtensionMinSize)
}

// Encode returns the wire-format bytes of this extension field, padded per RFC 7822.
func (ef ExtensionField) Encode() ([]byte, error) {
	return MarshalExtensionFields([]ExtensionField{ef})
}

// String returns the IANA-registered name for known NTP extension field
// types and "Unknown(0xXXXX)" otherwise. See RFC 8915 §5 and the IANA "NTP Extension
// Field Types" registry.
func (t ExtensionFieldType) String() string {
	switch t {
	case UniqueIdentifier:
		return "Unique Identifier"
	case NTSCookie:
		return "NTS Cookie"
	case NTSCookiePlaceholder:
		return "NTS Cookie Placeholder"
	case NTSAuthenticator:
		return "NTS Authenticator and Encrypted Extension Fields"
	default:
		return fmt.Sprintf("Unknown(0x%04x)", uint16(t))
	}
}

// MarshalExtensionFields encodes a slice of extension fields into the wire
// format defined by RFC 7822 §7.5. Each field is padded with zeros to a
// multiple of 4 octets and to at least ExtensionMinSize.
func MarshalExtensionFields(efs []ExtensionField) ([]byte, error) {
	totalSize := 0
	for _, ef := range efs {
		if len(ef.Body) > ExtensionMaxBodySize {
			return nil, fmt.Errorf("%w: type=%#x body=%d max=%d",
				ErrExtensionBodyTooLarge, ef.Type, len(ef.Body), ExtensionMaxBodySize)
		}
		totalSize += ef.EncodedSize()
	}
	out := make([]byte, totalSize)
	offset := 0
	for _, ef := range efs {
		flen := ef.EncodedSize()
		binary.BigEndian.PutUint16(out[offset:offset+2], uint16(ef.Type))
		binary.BigEndian.PutUint16(out[offset+2:offset+4], uint16(flen)) // #nosec G115 -- bounded by ExtensionMaxBodySize check above
		copy(out[offset+ExtensionHeaderSize:], ef.Body)
		offset += flen
	}
	return out, nil
}

// ParseExtensionFields parses a buffer of zero or more concatenated extension
// fields per RFC 7822 §7.5. The buffer must contain only extension fields;
// any legacy NTPv3/v4 MAC bytes must be stripped before calling.
//
// The returned ExtensionField.Body contains the value plus any wire padding;
// the caller interprets the value boundary based on the field type. Bodies
// are copied out of the input buffer so the returned EFs remain valid even
// if the caller mutates or reuses the input (e.g. a UDP read loop that
// overwrites a single receive buffer per packet).
func ParseExtensionFields(b []byte) ([]ExtensionField, error) {
	var efs []ExtensionField
	for len(b) > 0 {
		if len(b) < ExtensionHeaderSize {
			return nil, fmt.Errorf("%w: need %d header bytes, have %d",
				ErrExtensionTruncated, ExtensionHeaderSize, len(b))
		}
		ftype := ExtensionFieldType(binary.BigEndian.Uint16(b[0:2]))
		flen := int(binary.BigEndian.Uint16(b[2:4]))
		if flen < ExtensionMinSize || flen%ExtensionAlignment != 0 {
			return nil, fmt.Errorf("%w: type=%#x length=%d", ErrExtensionLengthInvalid, ftype, flen)
		}
		if flen > len(b) {
			return nil, fmt.Errorf("%w: type=%#x length=%d remaining=%d",
				ErrExtensionTruncated, ftype, flen, len(b))
		}
		// Copy so the body outlives any reuse of b.
		body := make([]byte, flen-ExtensionHeaderSize)
		copy(body, b[ExtensionHeaderSize:flen])
		efs = append(efs, ExtensionField{Type: ftype, Body: body})
		b = b[flen:]
	}
	return efs, nil
}
