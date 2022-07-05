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
	"fmt"
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

// MarshalBinaryTo marshals bytes to Signaling
func (p *Signaling) MarshalBinaryTo(b []byte) (int, error) {
	if len(p.TLVs) == 0 {
		return 0, fmt.Errorf("no TLVs in Signaling message, at least one required")
	}
	n := headerMarshalBinaryTo(&p.Header, b)
	binary.BigEndian.PutUint64(b[n:], uint64(p.TargetPortIdentity.ClockIdentity))
	binary.BigEndian.PutUint16(b[n+8:], p.TargetPortIdentity.PortNumber)
	pos := n + 10
	tlvLen, err := writeTLVs(p.TLVs, b[pos:])
	return pos + tlvLen, err
}

// MarshalBinary converts packet to []bytes
func (p *Signaling) MarshalBinary() ([]byte, error) {
	buf := make([]byte, 508)
	n, err := p.MarshalBinaryTo(buf)
	return buf[:n], err
}

// UnmarshalBinary parses []byte and populates struct fields
func (p *Signaling) UnmarshalBinary(b []byte) error {
	if len(b) < headerSize+10+tlvHeadSize {
		return fmt.Errorf("not enough data to decode Signaling")
	}

	unmarshalHeader(&p.Header, b)
	if err := checkPacketLength(&p.Header, len(b)); err != nil {
		return err
	}

	if p.SdoIDAndMsgType.MsgType() != MessageSignaling {
		return fmt.Errorf("not a signaling message %v", b)
	}

	p.TargetPortIdentity.ClockIdentity = ClockIdentity(binary.BigEndian.Uint64(b[headerSize:]))
	p.TargetPortIdentity.PortNumber = binary.BigEndian.Uint16(b[headerSize+8:])

	pos := headerSize + 10
	var err error
	p.TLVs, err = readTLVs(p.TLVs, int(p.MessageLength)-pos, b[pos:])
	if err != nil {
		return err
	}
	if len(p.TLVs) == 0 {
		return fmt.Errorf("no TLVs read for Signaling message, at least one required")
	}
	return nil
}
