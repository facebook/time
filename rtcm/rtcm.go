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

// Package rtcm provides parsing of RTCM 10403.x (RTCM3) binary frames
// used for GNSS correction data.
package rtcm

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	// Preamble is the sync byte that starts every RTCM3 frame.
	Preamble byte = 0xD3
	// HeaderSize is the size of the frame header (preamble + 2 bytes).
	HeaderSize = 3
	// CRCSize is the size of the CRC-24Q checksum.
	CRCSize = 3
	// FrameOverhead is the total overhead per frame (header + CRC).
	FrameOverhead = HeaderSize + CRCSize
	// MaxPayloadLen is the maximum RTCM3 payload length (10-bit field).
	MaxPayloadLen = 1023
	// MaxFrameSize is the maximum total frame size.
	MaxFrameSize = MaxPayloadLen + FrameOverhead
)

// Well-known RTCM3 message types for GNSS corrections.
const (
	TypeStationARP    uint16 = 1005 // Stationary RTK reference station ARP
	TypeGPSMSM7       uint16 = 1077 // GPS MSM7
	TypeGLONASSMSM7   uint16 = 1087 // GLONASS MSM7
	TypeGalileoMSM7   uint16 = 1097 // Galileo MSM7
	TypeBeiDouMSM7    uint16 = 1127 // BeiDou MSM7
	TypeGLONASSBiases uint16 = 1230 // GLONASS code-phase biases
)

var (
	ErrInvalidPreamble = errors.New("invalid preamble")
	ErrPayloadTooLarge = errors.New("payload length exceeds maximum")
	ErrFrameTooShort   = errors.New("frame data too short")
	ErrCRCMismatch     = errors.New("CRC-24Q mismatch")
)

// Frame represents a parsed RTCM3 frame.
type Frame struct {
	// MessageType is the 12-bit RTCM3 message type ID.
	MessageType uint16
	// Payload is the frame payload (without header and CRC).
	Payload []byte
	// Raw is the complete frame bytes including header, payload, and CRC.
	Raw []byte
}

// PayloadLen extracts the 10-bit payload length from a 3-byte RTCM3 header.
func PayloadLen(header []byte) int {
	return int(binary.BigEndian.Uint16(header[1:3]) & 0x03FF)
}

// MessageTypeFromPayload extracts the 12-bit message type from the first
// two bytes of an RTCM3 payload.
func MessageTypeFromPayload(payload []byte) uint16 {
	return (uint16(payload[0]) << 4) | (uint16(payload[1]) >> 4)
}

// ParseFrame validates and parses a complete RTCM3 frame from raw bytes.
// The input must contain the full frame including preamble, header, payload, and CRC.
func ParseFrame(data []byte) (Frame, error) {
	if len(data) < FrameOverhead {
		return Frame{}, ErrFrameTooShort
	}
	if data[0] != Preamble {
		return Frame{}, ErrInvalidPreamble
	}

	payloadLen := PayloadLen(data[:HeaderSize])
	if payloadLen > MaxPayloadLen {
		return Frame{}, ErrPayloadTooLarge
	}

	frameLen := payloadLen + FrameOverhead
	if len(data) < frameLen {
		return Frame{}, ErrFrameTooShort
	}

	// CRC is computed over header + payload (everything except the 3-byte CRC itself).
	crcData := data[:HeaderSize+payloadLen]
	crcGot := CRC24Q(crcData)

	crcExpect := uint32(data[frameLen-3])<<16 |
		uint32(data[frameLen-2])<<8 |
		uint32(data[frameLen-1])
	if crcGot != crcExpect {
		return Frame{}, fmt.Errorf(
			"%w: computed 0x%06X, expected 0x%06X", ErrCRCMismatch, crcGot, crcExpect,
		)
	}

	payload := data[HeaderSize : HeaderSize+payloadLen]
	var msgType uint16
	if payloadLen >= 2 {
		msgType = MessageTypeFromPayload(payload)
	}

	raw := make([]byte, frameLen)
	copy(raw, data[:frameLen])

	return Frame{
		MessageType: msgType,
		Payload:     payload,
		Raw:         raw,
	}, nil
}

// crc24qTable is the precomputed CRC-24Q lookup table for the RTCM3
// polynomial 0x1864CFB (used by RTCM 10403.x, SBAS, and other GNSS standards).
var crc24qTable [256]uint32

func init() {
	const poly = 0x1864CFB
	for i := range 256 {
		crc := uint32(i) << 16
		for range 8 {
			crc <<= 1
			if crc&0x1000000 != 0 {
				crc ^= poly
			}
		}
		crc24qTable[i] = crc & 0xFFFFFF
	}
}

// CRC24Q computes the CRC-24Q checksum used in RTCM3 frames.
func CRC24Q(data []byte) uint32 {
	var crc uint32
	for _, b := range data {
		crc = (crc << 8) ^ crc24qTable[((crc>>16)^uint32(b))&0xFF]
	}
	return crc & 0xFFFFFF
}
