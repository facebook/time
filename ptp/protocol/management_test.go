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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagementErrorIDString(t *testing.T) {
	require.Equal(t, "RESPONSE_TOO_BIG", ErrorResponseTooBig.String())
	require.Equal(t, "NO_SUCH_ID", ErrorNoSuchID.String())
	require.Equal(t, "WRONG_LENGTH", ErrorWrongLength.String())
	require.Equal(t, "WRONG_VALUE", ErrorWrongValue.String())
	require.Equal(t, "NOT_SETABLE", ErrorNotSetable.String())
	require.Equal(t, "NOT_SUPPORTED", ErrorNotSupported.String())
	require.Equal(t, "UNPOPULATED", ErrorUnpopulated.String())
	require.Equal(t, "GENERAL_ERROR", ErrorGeneralError.String())
	require.Equal(t, "UNKNOWN_ERROR_ID=123", ManagementErrorID(123).String())
}

func TestManagementMsgHeadAction(t *testing.T) {
	head := ManagementMsgHead{
		Header: Header{
			SdoIDAndMsgType: NewSdoIDAndMsgType(MessageManagement, 0),
			Version:         MajorVersion,
			MessageLength:   10,
			SourcePortIdentity: PortIdentity{
				PortNumber:    0,
				ClockIdentity: 5212879185253000328,
			},
			SequenceID:         1,
			ControlField:       4,
			LogMessageInterval: 0x7f,
		},
		TargetPortIdentity: PortIdentity{
			PortNumber:    56428,
			ClockIdentity: 0,
		},
		ActionField: RESPONSE,
	}
	require.Equal(t, RESPONSE, head.Action())
}

func TestManagementTLVHeadMgmtID(t *testing.T) {
	head := ManagementTLVHead{
		TLVHead: TLVHead{
			TLVType:     TLVManagement,
			LengthField: 4,
		},
		ManagementID: IDClockAccuracy,
	}
	require.Equal(t, IDClockAccuracy, head.MgmtID())
}

func TestParseManagementBadTLV(t *testing.T) {
	raw := []uint8("\x0d\x12\x00\x56\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x48\x57\xdd\xff\xfe\x0e\x91\xda\x00\x00\x00\x00\x04\x7f\x00\x00\x00\x00\x00\x00\x00\x00\xc4\xbf\x00\x00\x02\x00\x00\x05\x00\x22\x20\x02\xb8\xce\xf6\xff\xfe\x02\x10\xdc\x00\x01\x00\x00\xff\xff\x7f\xff\xff\xff\x80\x06\x22\x59\xe0\x80\xb8\xce\xf6\xff\xfe\x02\x10\xdc\x00\x00")
	packet := new(Management)
	err := FromBytes(raw, packet)
	require.EqualError(t, err, "got TLV type \"GRANT_UNICAST_TRANSMISSION\" (0x05) instead of \"MANAGEMENT\" (0x01)")
}

func TestParseMsgErrorStatus(t *testing.T) {
	raw := []uint8{0x0d, 0x02, 0x00, 0x3c, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x48, 0x57, 0xdd, 0xff, 0xfe, 0x08, 0x64, 0x88, 0x00, 0x00,
		0x00, 0x01, 0x04, 0x7f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xdc, 0x6c, 0x00, 0x00, 0x02, 0x00, 0x00, 0x02,
		0x00, 0x08, 0x00, 0x06, 0x20, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00,
	}
	packet := new(ManagementMsgErrorStatus)
	err := FromBytes(raw, packet)
	require.Nil(t, err)
	want := ManagementMsgErrorStatus{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:     NewSdoIDAndMsgType(MessageManagement, 0),
				Version:             MajorVersion,
				MessageLength:       uint16(len(raw) - 2),
				DomainNumber:        0,
				MinorSdoID:          0,
				FlagField:           0,
				CorrectionField:     0,
				MessageTypeSpecific: 0,
				SourcePortIdentity: PortIdentity{
					PortNumber:    0,
					ClockIdentity: 5212879185253000328,
				},
				SequenceID:         1,
				ControlField:       4,
				LogMessageInterval: 0x7f,
			},
			TargetPortIdentity: PortIdentity{
				PortNumber:    56428,
				ClockIdentity: 0,
			},
			ActionField: RESPONSE,
		},
		ManagementErrorStatusTLV: ManagementErrorStatusTLV{
			TLVHead: TLVHead{
				TLVType:     TLVManagementErrorStatus,
				LengthField: 8,
			},
			ManagementErrorID: ErrorNotSupported,
			ManagementID:      IDCurrentDataSet,
		},
	}
	require.Equal(t, want, *packet)
	b, err := Bytes(packet)
	require.Nil(t, err)
	assert.Equal(t, raw, b)

	// test generic DecodePacket as well
	pp, err := DecodePacket(raw)
	require.Nil(t, err)
	assert.Equal(t, &want, pp)
}

func TestParseMsgErrorStatusWithText(t *testing.T) {
	raw := []uint8{0x0d, 0x02, 0x00, 0x41, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x48, 0x57, 0xdd, 0xff, 0xfe, 0x08, 0x64, 0x88, 0x00, 0x00,
		0x00, 0x01, 0x04, 0x7f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xdc, 0x6c, 0x00, 0x00, 0x02, 0x00, 0x00, 0x02,
		0x00, 0x08, 0x00, 0x06, 0x20, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x04, 0x41, 0x6c, 0x65, 0x78, 0x00, 0x00,
	}
	packet := new(ManagementMsgErrorStatus)
	err := FromBytes(raw, packet)
	require.Nil(t, err)
	want := ManagementMsgErrorStatus{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:     NewSdoIDAndMsgType(MessageManagement, 0),
				Version:             MajorVersion,
				MessageLength:       uint16(len(raw) - 2),
				DomainNumber:        0,
				MinorSdoID:          0,
				FlagField:           0,
				CorrectionField:     0,
				MessageTypeSpecific: 0,
				SourcePortIdentity: PortIdentity{
					PortNumber:    0,
					ClockIdentity: 5212879185253000328,
				},
				SequenceID:         1,
				ControlField:       4,
				LogMessageInterval: 0x7f,
			},
			TargetPortIdentity: PortIdentity{
				PortNumber:    56428,
				ClockIdentity: 0,
			},
			ActionField: RESPONSE,
		},
		ManagementErrorStatusTLV: ManagementErrorStatusTLV{
			TLVHead: TLVHead{
				TLVType:     TLVManagementErrorStatus,
				LengthField: 8,
			},
			ManagementErrorID: ErrorNotSupported,
			ManagementID:      IDCurrentDataSet,
			DisplayData:       PTPText("Alex"),
		},
	}
	require.Equal(t, want, *packet)
	b, err := Bytes(packet)
	require.Nil(t, err)
	assert.Equal(t, raw, b)

	// test generic DecodePacket as well
	pp, err := DecodePacket(raw)
	require.Nil(t, err)
	assert.Equal(t, &want, pp)
}

// io.Writer with limited space
type bbuf struct {
	b   []byte
	pos int
}

func (b *bbuf) Write(p []byte) (int, error) {
	if b.pos+len(p) > len(b.b) {
		return 0, fmt.Errorf("data of size %d won't fit into %d from %d", len(p), len(b.b), p)
	}
	for i, v := range p {
		b.b[b.pos+i] = v
	}
	b.pos += len(p)
	return len(p), nil
}

func TestDecodeManagementPacketPartial(t *testing.T) {
	managementDefaultDataSet := []uint8{
		0x0d, 0x12, 0x00, 0x4a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x48, 0x57, 0xdd, 0xff, 0xfe, 0x0e, 0x91, 0xda, 0x00, 0x00,
		0x00, 0x00, 0x04, 0x7f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xb7, 0x5f, 0x00, 0x00, 0x02, 0x00, 0x00, 0x01,
		0x00, 0x16, 0x20, 0x00, 0x03, 0x00, 0x00, 0x01, 0x80, 0xff,
		0xfe, 0xff, 0xff, 0x80, 0x48, 0x57, 0xdd, 0xff, 0xfe, 0x0e,
		0x91, 0xda, 0x00, 0x00, 0x00, 0x00,
	}
	l := len(managementDefaultDataSet)
	original := &Management{}
	err := original.UnmarshalBinary(managementDefaultDataSet)
	require.NoError(t, err)

	for i := 1; i < l-2; i++ {
		b := managementDefaultDataSet[:i]
		packet := &Management{}
		err = packet.UnmarshalBinary(b)
		require.Error(t, err)
		buf := &bbuf{b: make([]byte, i), pos: 0}
		// make sure we can't fit original packet here
		err = original.MarshalBinaryToBuf(buf)
		require.Error(t, err)
	}
}

func TestDecodeManagementErrorStatusPacketPartial(t *testing.T) {
	managementErrorStatus := []uint8{0x0d, 0x02, 0x00, 0x3c, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x48, 0x57, 0xdd, 0xff, 0xfe, 0x08, 0x64, 0x88, 0x00, 0x00,
		0x00, 0x01, 0x04, 0x7f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xdc, 0x6c, 0x00, 0x00, 0x02, 0x00, 0x00, 0x02,
		0x00, 0x08, 0x00, 0x06, 0x20, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00,
	}
	l := len(managementErrorStatus)
	original := &ManagementMsgErrorStatus{}
	err := original.UnmarshalBinary(managementErrorStatus)
	require.NoError(t, err)

	for i := 1; i < l-2; i++ {
		p := &ManagementMsgErrorStatus{}
		b := managementErrorStatus[:i]
		err := p.UnmarshalBinary(b)
		require.Error(t, err)
		buf := &bbuf{b: make([]byte, i), pos: 0}
		// make sure we can't fit original packet here
		err = original.MarshalBinaryToBuf(buf)
		require.Error(t, err)
	}
}
