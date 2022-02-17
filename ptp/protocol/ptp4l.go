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

// Support has been included for some non-standard extensions provided by the ptp4l implementation; the TLVs IDPortStatsNP and IDTimeStatusNP
// Implemented as present in linuxptp master d95f4cd6e4a7c6c51a220c58903110a2326885e7

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// ptp4l-specific management TLV ids
const (
	IDTimeStatusNP       ManagementID = 0xC000
	IDPortPropertiesNP   ManagementID = 0xC004
	IDPortStatsNP        ManagementID = 0xC005
	IDPortServiceStatsNP ManagementID = 0xC007
)

// Timestamping is a ptp4l-specific enum describing timestamping type
type Timestamping uint8

const (
	TimestampingSoftware Timestamping = iota
	TimestampingHardware
	TimestampingLegacyHW
	TimestampingOneStep
	TimestampingP2P1Step
)

// PortStats is a ptp4l struct containing port statistics
type PortStats struct {
	RXMsgType [16]uint64
	TXMsgType [16]uint64
}

// PortStatsNPTLV is a ptp4l struct containing port identinity and statistics
type PortStatsNPTLV struct {
	ManagementTLVHead

	PortIdentity PortIdentity
	PortStats    PortStats
}

// MarshalBinary converts packet to []bytes
func (p *PortStatsNPTLV) MarshalBinary() ([]byte, error) {
	var bytes bytes.Buffer
	if err := binary.Write(&bytes, binary.BigEndian, p.ManagementTLVHead); err != nil {
		return nil, err
	}
	if err := binary.Write(&bytes, binary.BigEndian, p.PortIdentity); err != nil {
		return nil, err
	}
	if err := binary.Write(&bytes, binary.LittleEndian, p.PortStats); err != nil {
		return nil, err
	}
	return bytes.Bytes(), nil
}

// ScaledNS is some struct used by ptp4l to report phase change
type ScaledNS struct {
	NanosecondsMSB        uint16
	NanosecondsLSB        uint64
	FractionalNanoseconds uint16
}

// TimeStatusNPTLV is a ptp4l struct containing actually useful instance metrics
type TimeStatusNPTLV struct {
	ManagementTLVHead

	MasterOffsetNS             int64
	IngressTimeNS              int64 // this is PHC time
	CumulativeScaledRateOffset int32
	ScaledLastGmPhaseChange    int32
	GMTimeBaseIndicator        uint16
	LastGmPhaseChange          ScaledNS
	GMPresent                  int32
	GMIdentity                 ClockIdentity
}

type PortPropertiesNPTLV struct {
	ManagementTLVHead

	PortIdentity PortIdentity
	PortState    PortState
	Timestamping Timestamping
	Interface    PTPText
}

// MarshalBinary converts packet to []bytes
func (p *PortPropertiesNPTLV) MarshalBinary() ([]byte, error) {
	var bytes bytes.Buffer
	if err := binary.Write(&bytes, binary.BigEndian, p.ManagementTLVHead); err != nil {
		return nil, err
	}
	if err := binary.Write(&bytes, binary.BigEndian, p.PortIdentity); err != nil {
		return nil, err
	}
	if err := binary.Write(&bytes, binary.LittleEndian, p.PortState); err != nil {
		return nil, err
	}
	if err := binary.Write(&bytes, binary.LittleEndian, p.Timestamping); err != nil {
		return nil, err
	}
	interfaceBytes, err := p.Interface.MarshalBinary()
	if err != nil {
		return nil, err
	}
	if err := binary.Write(&bytes, binary.LittleEndian, interfaceBytes); err != nil {
		return nil, err
	}
	return bytes.Bytes(), nil
}

// PortServiceStats is a ptp4l struct containing counters for different port events, which we added in linuxptp cfbb8bdb50f5a38687fcddccbe6a264c6a078bbd
type PortServiceStats struct {
	AnnounceTimeout       uint64 `json:"ptp.servicestats.announce_timeout"`
	SyncTimeout           uint64 `json:"ptp.servicestats.sync_timeout"`
	DelayTimeout          uint64 `json:"ptp.servicestats.delay_timeout"`
	UnicastServiceTimeout uint64 `json:"ptp.servicestats.unicast_service_timeout"`
	UnicastRequestTimeout uint64 `json:"ptp.servicestats.unicast_request_timeout"`
	MasterAnnounceTimeout uint64 `json:"ptp.servicestats.master_announce_timeout"`
	MasterSyncTimeout     uint64 `json:"ptp.servicestats.master_sync_timeout"`
	QualificationTimeout  uint64 `json:"ptp.servicestats.qualification_timeout"`
	SyncMismatch          uint64 `json:"ptp.servicestats.sync_mismatch"`
	FollowupMismatch      uint64 `json:"ptp.servicestats.followup_mismatch"`
}

// PortServiceStatsNPTLV is a management TLV added in linuxptp cfbb8bdb50f5a38687fcddccbe6a264c6a078bbd
type PortServiceStatsNPTLV struct {
	ManagementTLVHead

	PortIdentity     PortIdentity
	PortServiceStats PortServiceStats
}

// MarshalBinary converts packet to []bytes
func (p *PortServiceStatsNPTLV) MarshalBinary() ([]byte, error) {
	var bytes bytes.Buffer
	if err := binary.Write(&bytes, binary.BigEndian, p.ManagementTLVHead); err != nil {
		return nil, err
	}
	if err := binary.Write(&bytes, binary.BigEndian, p.PortIdentity); err != nil {
		return nil, err
	}
	if err := binary.Write(&bytes, binary.LittleEndian, p.PortServiceStats); err != nil {
		return nil, err
	}
	return bytes.Bytes(), nil
}

// PortStatsNPRequest prepares request packet for PORT_STATS_NP request
func PortStatsNPRequest() *Management {
	headerSize := uint16(binary.Size(ManagementMsgHead{}))
	tlvHeadSize := uint16(binary.Size(TLVHead{}))
	// we send request with no portStats data just like pmc does
	return &Management{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:    NewSdoIDAndMsgType(MessageManagement, 0),
				Version:            Version,
				MessageLength:      headerSize + tlvHeadSize + 2,
				SourcePortIdentity: identity,
				LogMessageInterval: MgmtLogMessageInterval,
			},
			TargetPortIdentity:   DefaultTargetPortIdentity,
			StartingBoundaryHops: 0,
			BoundaryHops:         0,
			ActionField:          GET,
		},
		TLV: &ManagementTLVHead{
			TLVHead: TLVHead{
				TLVType:     TLVManagement,
				LengthField: 2,
			},
			ManagementID: IDPortStatsNP,
		},
	}
}

// PortStatsNP sends PORT_STATS_NP request and returns response
func (c *MgmtClient) PortStatsNP() (*PortStatsNPTLV, error) {
	req := PortStatsNPRequest()
	p, err := c.Communicate(req)
	if err != nil {
		return nil, err
	}
	tlv, ok := p.TLV.(*PortStatsNPTLV)
	if !ok {
		return nil, fmt.Errorf("got unexpected management TLV %T, wanted %T", p.TLV, tlv)
	}
	return tlv, nil
}

// TimeStatusNPRequest prepares request packet for TIME_STATUS_NP request
func TimeStatusNPRequest() *Management {
	headerSize := uint16(binary.Size(ManagementMsgHead{}))
	tlvHeadSize := uint16(binary.Size(TLVHead{}))
	// we send request with no TimeStatusNP data just like pmc does
	return &Management{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:    NewSdoIDAndMsgType(MessageManagement, 0),
				Version:            Version,
				MessageLength:      headerSize + tlvHeadSize + 2,
				SourcePortIdentity: identity,
				LogMessageInterval: MgmtLogMessageInterval,
			},
			TargetPortIdentity:   DefaultTargetPortIdentity,
			StartingBoundaryHops: 0,
			BoundaryHops:         0,
			ActionField:          GET,
		},
		TLV: &ManagementTLVHead{
			TLVHead: TLVHead{
				TLVType:     TLVManagement,
				LengthField: 2,
			},
			ManagementID: IDTimeStatusNP,
		},
	}
}

// TimeStatusNP sends TIME_STATUS_NP request and returns response
func (c *MgmtClient) TimeStatusNP() (*TimeStatusNPTLV, error) {
	req := TimeStatusNPRequest()
	p, err := c.Communicate(req)
	if err != nil {
		return nil, err
	}
	tlv, ok := p.TLV.(*TimeStatusNPTLV)
	if !ok {
		return nil, fmt.Errorf("got unexpected management TLV %T, wanted %T", p.TLV, tlv)
	}
	return tlv, nil
}

// PortServiceStatsNPRequest prepares request packet for PORT_SERVICE_STATS_NP request
func PortServiceStatsNPRequest() *Management {
	headerSize := uint16(binary.Size(ManagementMsgHead{}))
	tlvHeadSize := uint16(binary.Size(TLVHead{}))
	// we send request with no portServiceStats data just like pmc does
	return &Management{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:    NewSdoIDAndMsgType(MessageManagement, 0),
				Version:            Version,
				MessageLength:      headerSize + tlvHeadSize + 2,
				SourcePortIdentity: identity,
				LogMessageInterval: MgmtLogMessageInterval,
			},
			TargetPortIdentity:   DefaultTargetPortIdentity,
			StartingBoundaryHops: 0,
			BoundaryHops:         0,
			ActionField:          GET,
		},
		TLV: &ManagementTLVHead{
			TLVHead: TLVHead{
				TLVType:     TLVManagement,
				LengthField: 2,
			},
			ManagementID: IDPortServiceStatsNP,
		},
	}
}

// PortServiceStatsNP sends PORT_SERVICE_STATS_NP request and returns response
func (c *MgmtClient) PortServiceStatsNP() (*PortServiceStatsNPTLV, error) {
	req := PortServiceStatsNPRequest()
	p, err := c.Communicate(req)
	if err != nil {
		return nil, err
	}
	tlv, ok := p.TLV.(*PortServiceStatsNPTLV)
	if !ok {
		return nil, fmt.Errorf("got unexpected management TLV %T, wanted %T", p.TLV, tlv)
	}
	return tlv, nil
}

// PortPropertiesNPRequest prepares request packet for PORT_STATS_NP request
func PortPropertiesNPRequest() *Management {
	headerSize := uint16(binary.Size(ManagementMsgHead{}))
	tlvHeadSize := uint16(binary.Size(TLVHead{}))
	// we send request with no portStats data just like pmc does
	return &Management{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:    NewSdoIDAndMsgType(MessageManagement, 0),
				Version:            Version,
				MessageLength:      headerSize + tlvHeadSize + 2,
				SourcePortIdentity: identity,
				LogMessageInterval: MgmtLogMessageInterval,
			},
			TargetPortIdentity:   DefaultTargetPortIdentity,
			StartingBoundaryHops: 0,
			BoundaryHops:         0,
			ActionField:          GET,
		},
		TLV: &ManagementTLVHead{
			TLVHead: TLVHead{
				TLVType:     TLVManagement,
				LengthField: 2,
			},
			ManagementID: IDPortPropertiesNP,
		},
	}
}

// PortPropertiesNP sends PORT_PROPERTIES_NP request and returns response
func (c *MgmtClient) PortPropertiesNP() (*PortPropertiesNPTLV, error) {
	req := PortPropertiesNPRequest()
	p, err := c.Communicate(req)
	if err != nil {
		return nil, err
	}
	tlv, ok := p.TLV.(*PortPropertiesNPTLV)
	if !ok {
		return nil, fmt.Errorf("got unexpected management TLV %T, wanted %T", p.TLV, tlv)
	}
	return tlv, nil
}
