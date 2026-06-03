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

package rtcm

import "encoding/binary"

// AntennaDescriptor holds parameters for generating RTCM 1033
// (Receiver and Antenna Descriptor) messages.
type AntennaDescriptor struct {
	AntennaType    string
	AntennaSerial  string
	ReceiverType   string
	ReceiverFW     string
	ReceiverSerial string
	StationID      uint16
	AntennaSetupID uint8
}

// Encode1033 generates a complete RTCM 1033 frame (header + payload + CRC).
func Encode1033(desc AntennaDescriptor) []byte {
	// Build the bit-packed payload.
	// RTCM 1033 fields:
	//   DF002: Message Number (12 bits) = 1033
	//   DF003: Reference Station ID (12 bits)
	//   DF227: Descriptor Counter N (8 bits)
	//   DF228: Antenna Descriptor (N bytes)
	//   DF229: Antenna Setup ID (8 bits)
	//   DF230: Serial Number Counter M (8 bits)
	//   DF231: Antenna Serial Number (M bytes)
	//   DF232: Receiver Type Counter I (8 bits)
	//   DF233: Receiver Type (I bytes)
	//   DF234: Firmware Counter J (8 bits)
	//   DF235: Receiver Firmware (J bytes)
	//   DF236: Serial Counter K (8 bits)
	//   DF237: Receiver Serial (K bytes)

	payloadSize := 3 + // 12+12 bits = 24 bits = 3 bytes for msg number + station ID
		1 + len(desc.AntennaType) +
		1 + // setup ID
		1 + len(desc.AntennaSerial) +
		1 + len(desc.ReceiverType) +
		1 + len(desc.ReceiverFW) +
		1 + len(desc.ReceiverSerial)

	payload := make([]byte, payloadSize)
	offset := 0

	// DF002 (12 bits) + DF003 (12 bits) = 24 bits = 3 bytes
	// Message number 1033 = 0x409 in upper 12 bits
	// Station ID in lower 12 bits
	val := (uint32(1033) << 12) | uint32(desc.StationID&0x0FFF)
	payload[0] = byte((val >> 16) & 0xFF)
	payload[1] = byte((val >> 8) & 0xFF)
	payload[2] = byte(val & 0xFF)
	offset = 3

	// Antenna descriptor
	payload[offset] = byte(len(desc.AntennaType) & 0xFF)
	offset++
	copy(payload[offset:], desc.AntennaType)
	offset += len(desc.AntennaType)

	// Antenna setup ID
	payload[offset] = desc.AntennaSetupID
	offset++

	// Antenna serial number
	payload[offset] = byte(len(desc.AntennaSerial) & 0xFF)
	offset++
	copy(payload[offset:], desc.AntennaSerial)
	offset += len(desc.AntennaSerial)

	// Receiver type
	payload[offset] = byte(len(desc.ReceiverType) & 0xFF)
	offset++
	copy(payload[offset:], desc.ReceiverType)
	offset += len(desc.ReceiverType)

	// Receiver firmware
	payload[offset] = byte(len(desc.ReceiverFW) & 0xFF)
	offset++
	copy(payload[offset:], desc.ReceiverFW)
	offset += len(desc.ReceiverFW)

	// Receiver serial
	payload[offset] = byte(len(desc.ReceiverSerial) & 0xFF)
	offset++
	copy(payload[offset:], desc.ReceiverSerial)

	// Build complete frame: header (3) + payload + CRC (3)
	frameLen := HeaderSize + len(payload) + CRCSize
	frame := make([]byte, frameLen)

	// Header: preamble + 6 reserved bits (0) + 10-bit length
	frame[0] = Preamble
	binary.BigEndian.PutUint16(frame[1:3], uint16(len(payload)&0xFFFF))

	// Payload
	copy(frame[HeaderSize:], payload)

	putCRC(frame)
	return frame
}
