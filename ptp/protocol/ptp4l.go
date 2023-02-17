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
	"net"

	"github.com/facebook/time/hostendian"
)

// PTP4lSock is the default path to PTP4L socket
const PTP4lSock = "/var/run/ptp4l"

// ptp4l-specific management TLV ids
const (
	IDTimeStatusNP         ManagementID = 0xC000
	IDPortPropertiesNP     ManagementID = 0xC004
	IDPortStatsNP          ManagementID = 0xC005
	IDPortServiceStatsNP   ManagementID = 0xC007
	IDUnicastMasterTableNP ManagementID = 0xC008
)

// UnicastMasterState is a enum describing the unicast master state in ptp4l unicast master table
type UnicastMasterState uint8

// possible states of unicast master in ptp4l unicast master table
const (
	UnicastMasterStateWait UnicastMasterState = iota
	UnicastMasterStateHaveAnnounce
	UnicastMasterStateNeedSYDY
	UnicastMasterStateHaveSYDY
)

// UnicastMasterStateToString is a map from UnicastMasterState to string
var UnicastMasterStateToString = map[UnicastMasterState]string{
	UnicastMasterStateWait:         "WAIT",
	UnicastMasterStateHaveAnnounce: "HAVE_ANN",
	UnicastMasterStateNeedSYDY:     "NEED_SYDY",
	UnicastMasterStateHaveSYDY:     "HAVE_SYDY",
}

func (t UnicastMasterState) String() string {
	return UnicastMasterStateToString[t]
}

// Timestamping is a ptp4l-specific enum describing timestamping type
type Timestamping uint8

const (
	// TimestampingSoftware is a software timestamp const
	TimestampingSoftware Timestamping = iota
	// TimestampingHardware is a hardware timestamp const
	TimestampingHardware
	// TimestampingLegacyHW is a legacy hardware timestamp const
	TimestampingLegacyHW
	// TimestampingOneStep is a one step timestamp const
	TimestampingOneStep
	// TimestampingP2P1Step is a P2P one step timestamp const
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
	if err := binary.Write(&bytes, hostendian.Order, p.PortStats); err != nil {
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

// PortPropertiesNPTLV is a ptp4l struct containing port properties
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
	if err := binary.Write(&bytes, hostendian.Order, p.PortState); err != nil {
		return nil, err
	}
	if err := binary.Write(&bytes, hostendian.Order, p.Timestamping); err != nil {
		return nil, err
	}
	interfaceBytes, err := p.Interface.MarshalBinary()
	if err != nil {
		return nil, err
	}
	if err := binary.Write(&bytes, hostendian.Order, interfaceBytes); err != nil {
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
	if err := binary.Write(&bytes, hostendian.Order, p.PortServiceStats); err != nil {
		return nil, err
	}
	return bytes.Bytes(), nil
}

// UnicastMasterEntry is an entry in UnicastMasterTable that ptp4l exports via management TLV
type UnicastMasterEntry struct {
	PortIdentity PortIdentity
	ClockQuality ClockQuality
	Selected     bool
	PortState    UnicastMasterState
	Priority1    uint8
	Priority2    uint8
	Address      net.IP
}

// UnmarshalBinary implements Unmarshaller interface
func (e *UnicastMasterEntry) UnmarshalBinary(b []byte) error {
	var err error
	if len(b) < 26 { // 22 byte for struct, at least 4 for address)
		return fmt.Errorf("not enough data to decode UnicastMasterEntry")
	}
	e.PortIdentity.ClockIdentity = ClockIdentity(binary.BigEndian.Uint64(b[0:]))
	e.PortIdentity.PortNumber = binary.BigEndian.Uint16(b[8:])
	e.ClockQuality.ClockClass = ClockClass(b[10])
	e.ClockQuality.ClockAccuracy = ClockAccuracy(b[11])
	e.ClockQuality.OffsetScaledLogVariance = binary.BigEndian.Uint16(b[12:])
	if b[14] == 0 {
		e.Selected = false
	} else if b[14] == 1 {
		e.Selected = true
	} else {
		return fmt.Errorf("unexpected 'selected' value %d", b[14])
	}
	e.PortState = UnicastMasterState(b[15])
	e.Priority1 = b[16]
	e.Priority2 = b[17]

	pa := &PortAddress{}
	if err := pa.UnmarshalBinary(b[18:]); err != nil {
		return err
	}
	e.Address, err = pa.IP()
	if err != nil {
		return err
	}
	return nil
}

// MarshalBinary converts UnicastMasterEntry to []bytes
func (e *UnicastMasterEntry) MarshalBinary() ([]byte, error) {
	var bytes bytes.Buffer
	if err := binary.Write(&bytes, binary.BigEndian, e.PortIdentity); err != nil {
		return nil, err
	}
	if err := binary.Write(&bytes, binary.BigEndian, e.ClockQuality); err != nil {
		return nil, err
	}
	var selectedBin uint8
	if e.Selected {
		selectedBin = 1
	}
	if err := binary.Write(&bytes, binary.BigEndian, selectedBin); err != nil {
		return nil, err
	}
	if err := binary.Write(&bytes, binary.BigEndian, e.PortState); err != nil {
		return nil, err
	}
	if err := binary.Write(&bytes, binary.BigEndian, e.Priority1); err != nil {
		return nil, err
	}
	if err := binary.Write(&bytes, binary.BigEndian, e.Priority2); err != nil {
		return nil, err
	}
	var pa PortAddress
	asIPv4 := e.Address.To4()
	if asIPv4 != nil {
		pa = PortAddress{
			NetworkProtocol: TransportTypeUDPIPV4,
			AddressLength:   4,
			AddressField:    asIPv4,
		}
	} else {
		pa = PortAddress{
			NetworkProtocol: TransportTypeUDPIPV6,
			AddressLength:   16,
			AddressField:    e.Address,
		}
	}
	portBytes, err := pa.MarshalBinary()
	if err != nil {
		return nil, err
	}
	if err := binary.Write(&bytes, binary.BigEndian, portBytes); err != nil {
		return nil, err
	}
	return bytes.Bytes(), nil
}

// UnicastMasterTable is a table of UnicastMasterEntries
type UnicastMasterTable struct {
	ActualTableSize uint16
	UnicastMasters  []UnicastMasterEntry
}

// UnicastMasterTableNPTLV is a custom management packet that exports unicast master table state
type UnicastMasterTableNPTLV struct {
	ManagementTLVHead

	UnicastMasterTable UnicastMasterTable
}

// MarshalBinary converts packet to []bytes
func (p *UnicastMasterTableNPTLV) MarshalBinary() ([]byte, error) {
	var bytes bytes.Buffer
	if err := binary.Write(&bytes, binary.BigEndian, p.ManagementTLVHead); err != nil {
		return nil, err
	}
	if err := binary.Write(&bytes, binary.BigEndian, p.UnicastMasterTable.ActualTableSize); err != nil {
		return nil, err
	}
	for _, e := range p.UnicastMasterTable.UnicastMasters {
		entryBytes, err := e.MarshalBinary()
		if err != nil {
			return nil, err
		}
		if err := binary.Write(&bytes, binary.BigEndian, entryBytes); err != nil {
			return nil, err
		}
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

// UnicastMasterTableNPRequest creates new packet with UNICAST_MASTER_TABLE_NP request
func UnicastMasterTableNPRequest() *Management {
	headerSize := uint16(binary.Size(ManagementMsgHead{}))
	tlvHeadSize := uint16(binary.Size(TLVHead{}))
	// we send request with no data just like pmc does
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
			ManagementID: IDUnicastMasterTableNP,
		},
	}
}

// UnicastMasterTableNP request UNICAST_MASTER_TABLE_NP from ptp4l, and returns the result
func (c *MgmtClient) UnicastMasterTableNP() (*UnicastMasterTableNPTLV, error) {
	req := UnicastMasterTableNPRequest()
	p, err := c.Communicate(req)
	if err != nil {
		return nil, err
	}
	tlv, ok := p.TLV.(*UnicastMasterTableNPTLV)
	if !ok {
		return nil, fmt.Errorf("got unexpected management TLV %T, wanted %T", p.TLV, tlv)
	}
	return tlv, nil
}
