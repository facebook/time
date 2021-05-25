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
	"os"
)

var identity PortIdentity

// base struct sizes
const (
	headerSize  uint16 = 54
	tlvBaseSize uint16 = 2
)

func init() {
	// store our PID as identity that we use to talk to ptp daemon
	identity.PortNumber = uint16(os.Getpid())
}

// ManagementTLVHead Spec Table 58 - Management TLV fields
type ManagementTLVHead struct {
	TLVHead

	ManagementID ManagementID
}

// ManagementMsgHead Spec Table 56 - Management message fields
type ManagementMsgHead struct {
	Header

	TargetPortIdentity   PortIdentity
	StartingBoundaryHops uint8
	BoundaryHops         uint8
	ActionField          Action
	Reserved             uint8
}

// Action returns ActionField
func (p *ManagementMsgHead) Action() Action {
	return p.ActionField
}

// MgmtID returns ManagementID
func (p *ManagementTLVHead) MgmtID() ManagementID {
	return p.ManagementID
}

// CurrentDataSetTLV Spec Table 84 - CURRENT_DATA_SET management TLV data field
// size = 18 bytes
type CurrentDataSetTLV struct {
	StepsRemoved     uint16
	OffsetFromMaster TimeInterval
	MeanPathDelay    TimeInterval
}

// ManagementMsgCurrentDataSet is header + CurrentDataSet
type ManagementMsgCurrentDataSet struct {
	ManagementMsgHead
	ManagementTLVHead
	CurrentDataSetTLV
}

// DefaultDataSetTLV Spec Table 69 - DEFAULT_DATA_SET management TLV data field
// size = 20 bytes
type DefaultDataSetTLV struct {
	SoTSC         uint8
	Reserved0     uint8
	NumberPorts   uint16
	Priority1     uint8
	ClockQuality  ClockQuality
	Priority2     uint8
	ClockIdentity ClockIdentity
	DomainNumber  uint8
	Reserved1     uint8
}

// ManagementMsgDefaultDataSet is header + DefaultDataSet
type ManagementMsgDefaultDataSet struct {
	ManagementMsgHead
	ManagementTLVHead
	DefaultDataSetTLV
}

// ParentDataSetTLV Spec Table 85 - PARENT_DATA_SET management TLV data field
// size = 32 bytes
type ParentDataSetTLV struct {
	ParentPortIdentity                    PortIdentity
	PS                                    uint8
	Reserved                              uint8
	ObservedParentOffsetScaledLogVariance uint16
	ObservedParentClockPhaseChangeRate    uint32
	GrandmasterPriority1                  uint8
	GrandmasterClockQuality               ClockQuality
	GrandmasterPriority2                  uint8
	GrandmasterIdentity                   ClockIdentity
}

// ManagementMsgParentDataSet is header + ParentDataSet
type ManagementMsgParentDataSet struct {
	ManagementMsgHead
	ManagementTLVHead
	ParentDataSetTLV
}

// ManagementErrorStatusTLV spec Table 108 MANAGEMENT_ERROR_STATUS TLV format
type ManagementErrorStatusTLV struct {
	TLVHead

	ManagementErrorID ManagementErrorID
	ManagementID      ManagementID
	Reserved          int32
	DisplayData       PTPText
}

// ManagementMsgErrorStatus is header + ManagementErrorStatusTLV
type ManagementMsgErrorStatus struct {
	ManagementMsgHead
	ManagementErrorStatusTLV
}

// UnmarshalBinary parses []byte and populates struct fields
func (p *ManagementMsgErrorStatus) UnmarshalBinary(rawBytes []byte) error {
	reader := bytes.NewReader(rawBytes)
	be := binary.BigEndian
	if err := binary.Read(reader, be, &p.ManagementMsgHead); err != nil {
		return fmt.Errorf("reading ManagementMsgErrorStatus ManagementMsgHead: %w", err)
	}
	if err := binary.Read(reader, be, &p.ManagementErrorStatusTLV.TLVHead); err != nil {
		return fmt.Errorf("reading ManagementMsgErrorStatus TLVHead: %w", err)
	}
	if err := binary.Read(reader, be, &p.ManagementErrorStatusTLV.ManagementErrorID); err != nil {
		return fmt.Errorf("reading ManagementMsgErrorStatus ManagementErrorID: %w", err)
	}
	if err := binary.Read(reader, be, &p.ManagementErrorStatusTLV.ManagementID); err != nil {
		return fmt.Errorf("reading ManagementMsgErrorStatus ManagementID: %w", err)
	}
	if err := binary.Read(reader, be, &p.ManagementErrorStatusTLV.Reserved); err != nil {
		return fmt.Errorf("reading ManagementMsgErrorStatus Reserved: %w", err)
	}
	// packet can have trailing bytes, let's make sure we don't try to read past given length
	toRead := int(p.ManagementMsgHead.Header.MessageLength)
	toRead -= binary.Size(p.ManagementMsgHead)
	toRead -= binary.Size(p.ManagementErrorStatusTLV.TLVHead)
	toRead -= binary.Size(p.ManagementErrorStatusTLV.ManagementErrorID)
	toRead -= binary.Size(p.ManagementErrorStatusTLV.ManagementID)
	toRead -= binary.Size(p.ManagementErrorStatusTLV.Reserved)

	if reader.Len() == 0 || toRead <= 0 {
		// DisplayData is completely optional
		return nil
	}
	data := make([]byte, reader.Len())
	if _, err := io.ReadFull(reader, data); err != nil {
		return err
	}
	if err := p.DisplayData.UnmarshalBinary(data); err != nil {
		return fmt.Errorf("reading ManagementMsgErrorStatus DisplayData: %w", err)
	}
	return nil
}

// MarshalBinary converts packet to []bytes
func (p *ManagementMsgErrorStatus) MarshalBinary() ([]byte, error) {
	var bytes bytes.Buffer
	be := binary.BigEndian
	if err := binary.Write(&bytes, be, &p.ManagementMsgHead); err != nil {
		return nil, fmt.Errorf("writing ManagementMsgErrorStatus ManagementMsgHead: %w", err)
	}
	if err := binary.Write(&bytes, be, &p.ManagementErrorStatusTLV.TLVHead); err != nil {
		return nil, fmt.Errorf("writing ManagementMsgErrorStatus TLVHead: %w", err)
	}
	if err := binary.Write(&bytes, be, &p.ManagementErrorStatusTLV.ManagementErrorID); err != nil {
		return nil, fmt.Errorf("writing ManagementMsgErrorStatus ManagementErrorID: %w", err)
	}
	if err := binary.Write(&bytes, be, &p.ManagementErrorStatusTLV.ManagementID); err != nil {
		return nil, fmt.Errorf("writing ManagementMsgErrorStatus ManagementID: %w", err)
	}
	if err := binary.Write(&bytes, be, &p.ManagementErrorStatusTLV.Reserved); err != nil {
		return nil, fmt.Errorf("writing ManagementMsgErrorStatus Reserved: %w", err)
	}
	if p.DisplayData != "" {
		dd, err := p.DisplayData.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("writing ManagementMsgErrorStatus DisplayData: %w", err)
		}
		bytes.Write(dd)
	}
	return bytes.Bytes(), nil
}

// Action indicate the action to be taken on receipt of the PTP message as defined in Table 57
type Action uint8

// actions as in Table 57 Values of the actionField
const (
	GET Action = iota
	SET
	RESPONSE
	COMMAND
	ACKNOWLEDGE
)

// ManagementID is type for Management IDs
type ManagementID uint16

// Management IDs we support, from Table 59 managementId values
const (
	IDNullPTPManagement        ManagementID = 0x0000
	IDClockDescription         ManagementID = 0x0001
	IDUserDescription          ManagementID = 0x0002
	IDSaveInNonVolatileStorage ManagementID = 0x0003
	IDResetNonVolatileStorage  ManagementID = 0x0004
	IDInitialize               ManagementID = 0x0005
	IDFaultLog                 ManagementID = 0x0006
	IDFaultLogReset            ManagementID = 0x0007

	IDDefaultDataSet        ManagementID = 0x2000
	IDCurrentDataSet        ManagementID = 0x2001
	IDParentDataSet         ManagementID = 0x2002
	IDTimePropertiesDataSet ManagementID = 0x2003
	IDPortDataSet           ManagementID = 0x2004
	// rest of Management IDs that we don't implement yet

)

// ManagementErrorID is an enum for possible management errors
type ManagementErrorID uint16

// Table 109 ManagementErrorID enumeration
const (
	ErrorResponseTooBig ManagementErrorID = 0x0001 // The requested operation could not fit in a single response message
	ErrorNoSuchID       ManagementErrorID = 0x0002 // The managementId is not recognized
	ErrorWrongLength    ManagementErrorID = 0x0003 // The managementId was identified but the length of the data was wrong
	ErrorWrongValue     ManagementErrorID = 0x0004 // The managementId and length were correct but one or more values were wrong
	ErrorNotSetable     ManagementErrorID = 0x0005 // Some of the variables in the set command were not updated because they are not configurable
	ErrorNotSupported   ManagementErrorID = 0x0006 // The requested operation is not supported in this PTP Instance
	ErrorUnpopulated    ManagementErrorID = 0x0007 // The targetPortIdentity of the PTP management message refers to an entity that is not present in the PTP Instance at the time of the request
	// some reserved and provile-specific ranges
	ErrorGeneralError ManagementErrorID = 0xFFFE //An error occurred that is not covered by other ManagementErrorID values
)

// ManagementErrorIDToString is a map from ManagementErrorID to string
var ManagementErrorIDToString = map[ManagementErrorID]string{
	ErrorResponseTooBig: "RESPONSE_TOO_BIG",
	ErrorNoSuchID:       "NO_SUCH_ID",
	ErrorWrongLength:    "WRONG_LENGTH",
	ErrorWrongValue:     "WRONG_VALUE",
	ErrorNotSetable:     "NOT_SETABLE",
	ErrorNotSupported:   "NOT_SUPPORTED",
	ErrorUnpopulated:    "UNPOPULATED",
	ErrorGeneralError:   "GENERAL_ERROR",
}

func (t ManagementErrorID) String() string {
	s := ManagementErrorIDToString[t]
	if s == "" {
		return fmt.Sprintf("UNKNOWN_ERROR_ID=%d", t)
	}
	return s
}

func (t ManagementErrorID) Error() string {
	return t.String()
}

// ManagementPacket is an iterface to abstract all different management packets
type ManagementPacket interface {
	Packet

	Action() Action
	MgmtID() ManagementID
}

// CurrentDataSetRequest prepares request packet for CURRENT_DATA_SET request
func CurrentDataSetRequest() *ManagementMsgCurrentDataSet {
	size := uint16(binary.Size(CurrentDataSetTLV{}))
	return &ManagementMsgCurrentDataSet{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:    NewSdoIDAndMsgType(MessageManagement, 0),
				Version:            Version,
				MessageLength:      headerSize + size,
				SourcePortIdentity: identity,
				LogMessageInterval: mgmtLogMessageInterval,
			},
			TargetPortIdentity:   defaultTargetPortIdentity,
			StartingBoundaryHops: 0,
			BoundaryHops:         0,
			ActionField:          GET,
		},
		ManagementTLVHead: ManagementTLVHead{
			TLVHead: TLVHead{
				TLVType:     TLVManagement,
				LengthField: tlvBaseSize + size,
			},
			ManagementID: IDCurrentDataSet,
		},
		CurrentDataSetTLV: CurrentDataSetTLV{},
	}
}

// DefaultDataSetRequest prepares request packet for DEFAULT_DATA_SET request
func DefaultDataSetRequest() *ManagementMsgDefaultDataSet {
	size := uint16(binary.Size(DefaultDataSetTLV{}))
	return &ManagementMsgDefaultDataSet{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:    NewSdoIDAndMsgType(MessageManagement, 0),
				Version:            Version,
				MessageLength:      headerSize + size,
				SourcePortIdentity: identity,
				LogMessageInterval: mgmtLogMessageInterval,
			},
			TargetPortIdentity:   defaultTargetPortIdentity,
			StartingBoundaryHops: 0,
			BoundaryHops:         0,
			ActionField:          GET,
		},
		ManagementTLVHead: ManagementTLVHead{
			TLVHead: TLVHead{
				TLVType:     TLVManagement,
				LengthField: tlvBaseSize + size,
			},
			ManagementID: IDDefaultDataSet,
		},
		DefaultDataSetTLV: DefaultDataSetTLV{},
	}
}

// ParentDataSetRequest prepares request packet for PARENT_DATA_SET request
func ParentDataSetRequest() *ManagementMsgParentDataSet {
	size := uint16(binary.Size(ParentDataSetTLV{}))
	return &ManagementMsgParentDataSet{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:    NewSdoIDAndMsgType(MessageManagement, 0),
				Version:            Version,
				MessageLength:      headerSize + size,
				SourcePortIdentity: identity,
				LogMessageInterval: mgmtLogMessageInterval,
			},
			TargetPortIdentity:   defaultTargetPortIdentity,
			StartingBoundaryHops: 0,
			BoundaryHops:         0,
			ActionField:          GET,
		},
		ManagementTLVHead: ManagementTLVHead{
			TLVHead: TLVHead{
				TLVType:     TLVManagement,
				LengthField: tlvBaseSize + size,
			},
			ManagementID: IDParentDataSet,
		},
		ParentDataSetTLV: ParentDataSetTLV{},
	}
}

func decodeMgmtPacket(data []byte) (Packet, error) {
	var err error
	head := ManagementMsgHead{}
	tlvHead := ManagementTLVHead{}
	r := bytes.NewReader(data)
	if err = binary.Read(r, binary.BigEndian, &head); err != nil {
		return nil, err
	}
	if err = binary.Read(r, binary.BigEndian, &tlvHead.TLVHead); err != nil {
		return nil, err
	}
	if tlvHead.TLVType == TLVManagementErrorStatus {
		errorPacket := new(ManagementMsgErrorStatus)
		if err := errorPacket.UnmarshalBinary(data); err != nil {
			return nil, fmt.Errorf("got Management Error in response but failed to decode it: %w", err)
		}
		return errorPacket, nil
	}

	if tlvHead.TLVType != TLVManagement {
		return nil, fmt.Errorf("got TLV type 0x%x instead of 0x%x", tlvHead.TLVType, TLVManagement)
	}

	if err = binary.Read(r, binary.BigEndian, &tlvHead.ManagementID); err != nil {
		return nil, err
	}
	switch tlvHead.ManagementID {
	case IDDefaultDataSet:
		tlv := &DefaultDataSetTLV{}
		if err := binary.Read(r, binary.BigEndian, tlv); err != nil {
			return nil, err
		}
		return &ManagementMsgDefaultDataSet{
			ManagementMsgHead: head,
			ManagementTLVHead: tlvHead,
			DefaultDataSetTLV: *tlv,
		}, nil
	case IDCurrentDataSet:
		tlv := &CurrentDataSetTLV{}
		if err := binary.Read(r, binary.BigEndian, tlv); err != nil {
			return nil, err
		}
		return &ManagementMsgCurrentDataSet{
			ManagementMsgHead: head,
			ManagementTLVHead: tlvHead,
			CurrentDataSetTLV: *tlv,
		}, nil
	case IDParentDataSet:
		tlv := &ParentDataSetTLV{}
		if err := binary.Read(r, binary.BigEndian, tlv); err != nil {
			return nil, err
		}
		return &ManagementMsgParentDataSet{
			ManagementMsgHead: head,
			ManagementTLVHead: tlvHead,
			ParentDataSetTLV:  *tlv,
		}, nil
	case IDPortStatsNP:
		tlv := &PortStatsNP{}
		if err := binary.Read(r, binary.BigEndian, &tlv.PortIdentity); err != nil {
			return nil, err
		}
		// fun part that cost me few hours, this is sent over wire as LittlEndian, while EVERYTHING ELSE is BigEndian.
		if err := binary.Read(r, binary.LittleEndian, &tlv.PortStats); err != nil {
			return nil, err
		}
		return &ManagementMsgPortStatsNP{
			ManagementMsgHead: head,
			ManagementTLVHead: tlvHead,
			PortStatsNP:       *tlv,
		}, nil
	case IDTimeStatusNP:
		tlv := &TimeStatusNP{}
		if err := binary.Read(r, binary.BigEndian, tlv); err != nil {
			return nil, err
		}
		return &ManagementMsgTimeStatusNP{
			ManagementMsgHead: head,
			ManagementTLVHead: tlvHead,
			TimeStatusNP:      *tlv,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported management TLV 0x%x", tlvHead.ManagementID)
	}
}
