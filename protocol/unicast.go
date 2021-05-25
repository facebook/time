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
	"encoding/binary"
	"fmt"
	"io"
)

// UnicastMsgTypeAndFlags is a uint8 where first 4 bites contain MessageType and last 4 bits contain some flags
type UnicastMsgTypeAndFlags uint8

// MsgType extracts MessageType from UnicastMsgTypeAndFlags
func (m UnicastMsgTypeAndFlags) MsgType() MessageType {
	return MessageType(m >> 4)
}

// NewUnicastMsgTypeAndFlags builds new UnicastMsgTypeAndFlags from MessageType and flags
func NewUnicastMsgTypeAndFlags(msgType MessageType, flags uint8) UnicastMsgTypeAndFlags {
	return UnicastMsgTypeAndFlags(uint8(msgType)<<4 | (flags & 0x0f))
}

// Signaling packet. As it's of variable size, we cannot just binary.Read/Write it.
type Signaling struct {
	Header
	TargetPortIdentity PortIdentity
	TLVs               []TLV
}

// MarshalBinary converts packet to []bytes
func (p *Signaling) MarshalBinary() ([]byte, error) {
	if len(p.TLVs) == 0 {
		return nil, fmt.Errorf("no TLVs in Signaling message, at least one required")
	}
	var bytes bytes.Buffer
	if err := binary.Write(&bytes, binary.BigEndian, p.Header); err != nil {
		return nil, err
	}
	if err := binary.Write(&bytes, binary.BigEndian, p.TargetPortIdentity); err != nil {
		return nil, err
	}
	for _, tlv := range p.TLVs {
		if err := binary.Write(&bytes, binary.BigEndian, tlv); err != nil {
			return nil, err
		}
	}
	return bytes.Bytes(), nil
}

// UnmarshalBinary parses []byte and populates struct fields
func (p *Signaling) UnmarshalBinary(rawBytes []byte) error {
	reader := bytes.NewReader(rawBytes)
	if err := binary.Read(reader, binary.BigEndian, &p.Header); err != nil {
		return err
	}
	if p.SdoIDAndMsgType.MsgType() != MessageSignaling {
		return fmt.Errorf("not a signaling message %v", rawBytes)
	}
	if err := binary.Read(reader, binary.BigEndian, &p.TargetPortIdentity); err != nil {
		return err
	}
	// packet can have trailing bytes, let's make sure we don't try to read past given length
	toRead := int(p.Header.MessageLength) - binary.Size(p.Header) - binary.Size(p.TargetPortIdentity)
	for {
		if toRead <= 0 {
			break
		}
		head := TLVHead{}
		headSize := binary.Size(head)
		if reader.Len() < headSize {
			break
		}
		if err := binary.Read(reader, binary.BigEndian, &head); err != nil {
			return err
		}
		// update toRead with what we just read
		toRead = toRead - headSize - int(head.LengthField)

		// seek back so we can read whole TLV
		if _, err := reader.Seek(-int64(headSize), io.SeekCurrent); err != nil {
			return err
		}
		switch head.TLVType {
		case TLVAcknowledgeCancelUnicastTransmission:
			tlv := &AcknowledgeCancelUnicastTransmissionTLV{}
			if err := binary.Read(reader, binary.BigEndian, tlv); err != nil {
				return err
			}
			p.TLVs = append(p.TLVs, tlv)

		case TLVGrantUnicastTransmission:
			tlv := &GrantUnicastTransmissionTLV{}
			if err := binary.Read(reader, binary.BigEndian, tlv); err != nil {
				return err
			}
			p.TLVs = append(p.TLVs, tlv)

		case TLVRequestUnicastTransmission:
			tlv := &RequestUnicastTransmissionTLV{}
			if err := binary.Read(reader, binary.BigEndian, tlv); err != nil {
				return err
			}
			p.TLVs = append(p.TLVs, tlv)
		case TLVCancelUnicastTransmission:
			tlv := &CancelUnicastTransmissionTLV{}
			if err := binary.Read(reader, binary.BigEndian, tlv); err != nil {
				return err
			}
			p.TLVs = append(p.TLVs, tlv)
		default:
			return fmt.Errorf("reading TLV %s (%d) is not yet implemented", head.TLVType, head.TLVType)
		}
	}
	if len(p.TLVs) == 0 {
		return fmt.Errorf("no TLVs read for Signaling message, at least one required")
	}
	return nil
}

// Unicast TLVs

// RequestUnicastTransmissionTLV Table 110 REQUEST_UNICAST_TRANSMISSION TLV format
type RequestUnicastTransmissionTLV struct {
	TLVHead
	MsgTypeAndReserved    UnicastMsgTypeAndFlags // first 4 bits only, same enums as with normal message type
	LogInterMessagePeriod LogInterval
	DurationField         uint32
}

// GrantUnicastTransmissionTLV Table 111 GRANT_UNICAST_TRANSMISSION TLV format
type GrantUnicastTransmissionTLV struct {
	TLVHead
	MsgTypeAndReserved    UnicastMsgTypeAndFlags // first 4 bits only, same enums as with normal message type
	LogInterMessagePeriod LogInterval
	DurationField         uint32
	Reserved              uint8
	Renewal               uint8
}

// CancelUnicastTransmissionTLV Table 112 CANCEL_UNICAST_TRANSMISSION TLV format
type CancelUnicastTransmissionTLV struct {
	TLVHead
	MsgTypeAndFlags UnicastMsgTypeAndFlags // first 4 bits is msg type, then flags R and/or G
	Reserved        uint8
}

// AcknowledgeCancelUnicastTransmissionTLV Table 113 ACKNOWLEDGE_CANCEL_UNICAST_TRANSMISSION TLV format
type AcknowledgeCancelUnicastTransmissionTLV struct {
	TLVHead
	MsgTypeAndFlags UnicastMsgTypeAndFlags // first 4 bits is msg type, then flags R and/or G
	Reserved        uint8
}
