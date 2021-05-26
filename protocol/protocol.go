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

// all references are given for IEEE 1588-2019 Standard

import (
	"bytes"
	"encoding"
	"encoding/binary"
	"fmt"
)

// Version is what version of PTP protocol we implement
const Version uint8 = 2

/* UDP port numbers
The UDP destination port of a PTP event message shall be 319.
The UDP destination port of a multicast PTP general message shall be 320.
The UDP destination port of a unicast PTP general message that is addressed to a PTP Instance shall be 320.
The UDP destination port of a unicast PTP general message that is addressed to a manager shall be the UDP source
port value of the PTP message to which this is a response.
*/
const (
	PortEvent   = 319
	PortGeneral = 320
)

const mgmtLogMessageInterval LogInterval = 0x7f // as per Table 42 Values of logMessageInterval field

var defaultTargetPortIdentity = PortIdentity{
	ClockIdentity: 0xffffffffffffffff,
	PortNumber:    0xffff,
}

// Header Table 35 Common PTP message header
type Header struct {
	SdoIDAndMsgType     SdoIDAndMsgType // first 4 bits is SdoId, next 4 bytes are msgtype
	Version             uint8
	MessageLength       uint16
	DomainNumber        uint8
	MinorSdoID          uint8
	FlagField           uint16
	CorrectionField     Correction
	MessageTypeSpecific uint32
	SourcePortIdentity  PortIdentity
	SequenceID          uint16
	ControlField        uint8       // the use of this field is obsolete according to IEEE, unless it's ipv4
	LogMessageInterval  LogInterval // see Table 42 Values of logMessageInterval field
}

// MessageType returns MessageType
func (p *Header) MessageType() MessageType {
	return p.SdoIDAndMsgType.MsgType()
}

// SetSequence populates sequence field
func (p *Header) SetSequence(sequence uint16) {
	p.SequenceID = sequence
}

// flags used in FlagField as per Table 37 Values of flagField
const (
	// first octet
	FlagAlternateMaster  uint16 = 1 << (8 + 0)
	FlagTwoStep          uint16 = 1 << (8 + 1)
	FlagUnicast          uint16 = 1 << (8 + 2)
	FlagProfileSpecific1 uint16 = 1 << (8 + 5)
	FlagProfileSpecific2 uint16 = 1 << (8 + 6)
	// second octet
	FlagLeap61                   uint16 = 1 << 0
	FlagLeap59                   uint16 = 1 << 1
	FlagCurrentUtcOffsetValid    uint16 = 1 << 2
	FlagPTPTimescale             uint16 = 1 << 3
	FlagTimeTraceable            uint16 = 1 << 4
	FlagFrequencyTraceable       uint16 = 1 << 5
	FlagSynchronizationUncertain uint16 = 1 << 6
)

// General PTP messages

// All packets are split in two parts: Header (which is common) and body that is unique
// for most packets (both in length and structure).
// The idea is that anything using this library to read packets will have to do roughly this:
// * receive raw packets as bytes, create bytes.Reader from it
// * use binary.Read to parse Header from this reader
// * analyze header fields and switch on MessageType
// * parse rest of the data into one of Body structs, with exact struct being chosen according to header MessageType.

// AnnounceBody Table 43 Announce message fields
type AnnounceBody struct {
	OriginTimestamp         Timestamp
	CurrentUTCOffset        int16
	Reserved                uint8
	GrandmasterPriority1    uint8
	GrandmasterClockQuality ClockQuality
	GrandmasterPriority2    uint8
	GrandmasterIdentity     ClockIdentity
	StepsRemoved            uint16
	TimeSource              TimeSource
}

// Announce is a full Announce packet
type Announce struct {
	Header
	AnnounceBody
}

// SyncDelayReqBody Table 44 Sync and Delay_Req message fields
type SyncDelayReqBody struct {
	OriginTimestamp Timestamp
}

// SyncDelayReq is a full Sync/Delay_Req packet
type SyncDelayReq struct {
	Header
	SyncDelayReqBody
}

// FollowUpBody Table 45 Follow_Up message fields
type FollowUpBody struct {
	PreciseOriginTimestamp Timestamp
}

// FollowUp is a full Follow_Up packet
type FollowUp struct {
	Header
	FollowUpBody
}

// DelayRespBody Table 46 Delay_Resp message fields
type DelayRespBody struct {
	ReceiveTimestamp       Timestamp
	RequestingPortIdentity PortIdentity
}

// DelayResp is a full Delay_Resp packet
type DelayResp struct {
	Header
	DelayRespBody
}

// PDelayReqBody Table 47 Pdelay_Req message fields
type PDelayReqBody struct {
	OriginTimestamp Timestamp
	Reserved        [10]uint8
}

// PDelayReq is a full Pdelay_Req packet
type PDelayReq struct {
	Header
	PDelayReqBody
}

// PDelayRespBody Table 48 Pdelay_Resp message fields
type PDelayRespBody struct {
	RequestReceiptTimestamp Timestamp
	RequestingPortIdentity  PortIdentity
}

// PDelayResp is a full Pdelay_Resp packet
type PDelayResp struct {
	Header
	PDelayRespBody
}

// PDelayRespFollowUpBody Table 49 Pdelay_Resp_Follow_Up message fields
type PDelayRespFollowUpBody struct {
	ResponseOriginTimestamp Timestamp
	RequestingPortIdentity  PortIdentity
}

// PDelayRespFollowUp is a full Pdelay_Resp_Follow_Up packet
type PDelayRespFollowUp struct {
	Header
	PDelayRespFollowUpBody
}

// Packet is an iterface to abstract all different packets
type Packet interface {
	MessageType() MessageType
	SetSequence(uint16)
}

// Bytes converts any packet to []bytes
// PTP over UDPv6 requires adding extra two bytes that
// may be modified by the initiator or an intermediate PTP Instance to ensure that the UDP checksum
// remains uncompromised after any modification of PTP fields.
// We simply always add them - in worst case they add extra 2 unused bytes when used over UDPv4.
func Bytes(p Packet) ([]byte, error) {
	// interface smuggling
	if pp, ok := p.(encoding.BinaryMarshaler); ok {
		b, err := pp.MarshalBinary()
		return append(b, []byte{0, 0}...), err
	}
	var bytes bytes.Buffer
	var err error
	err = binary.Write(&bytes, binary.BigEndian, p)
	if err != nil {
		return nil, err
	}
	err = binary.Write(&bytes, binary.BigEndian, []byte{0, 0})
	return bytes.Bytes(), err
}

// FromBytes parses []byte into any packet
func FromBytes(rawBytes []byte, p Packet) error {
	// interface smuggling
	if pp, ok := p.(encoding.BinaryUnmarshaler); ok {
		return pp.UnmarshalBinary(rawBytes)
	}
	reader := bytes.NewReader(rawBytes)
	return binary.Read(reader, binary.BigEndian, p)
}

// DecodePacket provides single entry point to try and decode any []bytes to PTPv2 packet.
// It can be used for easy integration with anything that provides UDP packet payload as bytes.
// Resulting Packet user can then either switch based on MessageType(), or just with type switch.
func DecodePacket(b []byte) (Packet, error) {
	r := bytes.NewReader(b)
	head := &Header{}
	if err := binary.Read(r, binary.BigEndian, head); err != nil {
		return nil, err
	}
	msgType := head.MessageType()
	var p Packet
	switch msgType {
	case MessageSync, MessageDelayReq:
		p = &SyncDelayReq{}
	case MessagePDelayReq:
		p = &PDelayReq{}
	case MessagePDelayResp:
		p = &PDelayResp{}
	case MessageFollowUp:
		p = &FollowUp{}
	case MessageDelayResp:
		p = &DelayResp{}
	case MessagePDelayRespFollowUp:
		p = &PDelayRespFollowUp{}
	case MessageAnnounce:
		p = &Announce{}
	case MessageSignaling:
		p = &Signaling{}
	case MessageManagement:
		return decodeMgmtPacket(b)
	default:
		return nil, fmt.Errorf("unsupported type %s", msgType)
	}

	if err := FromBytes(b, p); err != nil {
		return nil, err
	}
	return p, nil
}
