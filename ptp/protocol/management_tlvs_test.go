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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseParentDataSet(t *testing.T) {
	raw := []uint8("\x0d\x12\x00\x56\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x48\x57\xdd\xff\xfe\x0e\x91\xda\x00\x00\x00\x00\x04\x7f\x00\x00\x00\x00\x00\x00\x00\x00\xc4\xbf\x00\x00\x02\x00\x00\x01\x00\x22\x20\x02\xb8\xce\xf6\xff\xfe\x02\x10\xdc\x00\x01\x00\x00\xff\xff\x7f\xff\xff\xff\x80\x06\x22\x59\xe0\x80\xb8\xce\xf6\xff\xfe\x02\x10\xdc\x00\x00")
	packet := new(Management)
	err := FromBytes(raw, packet)
	require.Nil(t, err)
	want := Management{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:     NewSdoIDAndMsgType(MessageManagement, 0),
				Version:             Version,
				MessageLength:       uint16(len(raw) - 2),
				DomainNumber:        0,
				MinorSdoID:          0,
				FlagField:           0,
				CorrectionField:     0,
				MessageTypeSpecific: 0,
				SourcePortIdentity: PortIdentity{
					PortNumber:    0,
					ClockIdentity: 5212879185253405146,
				},
				SequenceID:         0,
				ControlField:       4,
				LogMessageInterval: 0x7f,
			},
			TargetPortIdentity: PortIdentity{
				PortNumber:    50367,
				ClockIdentity: 0,
			},
			ActionField: RESPONSE,
		},
		TLV: &ParentDataSetTLV{
			ManagementTLVHead: ManagementTLVHead{
				TLVHead: TLVHead{
					TLVType:     TLVManagement,
					LengthField: 34,
				},
				ManagementID: IDParentDataSet,
			},
			ParentPortIdentity: PortIdentity{
				PortNumber:    1,
				ClockIdentity: 13316852727519776988,
			},
			ObservedParentOffsetScaledLogVariance: 65535,
			ObservedParentClockPhaseChangeRate:    2147483647,
			GrandmasterPriority1:                  128,
			GrandmasterClockQuality: ClockQuality{
				ClockClass:              ClockClass6,
				ClockAccuracy:           ClockAccuracyNanosecond250,
				OffsetScaledLogVariance: 23008,
			},
			GrandmasterPriority2: 128,
			GrandmasterIdentity:  13316852727519776988,
		},
	}
	require.Equal(t, want, *packet)
	b, err := Bytes(packet)
	require.Nil(t, err)
	assert.Equal(t, raw, b)
}

func TestParseCurrentDataSet(t *testing.T) {
	raw := []uint8("\x0d\x12\x00\x48\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x48\x57\xdd\xff\xfe\x0e\x91\xda\x00\x00\x00\x00\x04\x7f\x00\x00\x00\x00\x00\x00\x00\x00\xc2\x92\x00\x00\x02\x00\x00\x01\x00\x14\x20\x01\x00\x01\xff\xff\xff\xf4\x45\x74\x00\x00\x00\x00\x02\x81\x12\xf6\x00\x00\x00\x00")
	packet := new(Management)
	err := FromBytes(raw, packet)
	require.Nil(t, err)
	want := Management{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:     NewSdoIDAndMsgType(MessageManagement, 0),
				Version:             Version,
				MessageLength:       uint16(len(raw) - 2),
				DomainNumber:        0,
				MinorSdoID:          0,
				FlagField:           0,
				CorrectionField:     0,
				MessageTypeSpecific: 0,
				SourcePortIdentity: PortIdentity{
					PortNumber:    0,
					ClockIdentity: 5212879185253405146,
				},
				SequenceID:         0,
				ControlField:       4,
				LogMessageInterval: 0x7f,
			},
			TargetPortIdentity: PortIdentity{
				PortNumber:    49810,
				ClockIdentity: 0,
			},
			ActionField: RESPONSE,
		},
		TLV: &CurrentDataSetTLV{
			ManagementTLVHead: ManagementTLVHead{
				TLVHead: TLVHead{
					TLVType:     TLVManagement,
					LengthField: 20,
				},
				ManagementID: IDCurrentDataSet,
			},
			StepsRemoved:     1,
			OffsetFromMaster: NewTimeInterval(-768652.0),
			MeanPathDelay:    NewTimeInterval(42013430.0),
		},
	}
	require.Equal(t, want, *packet)
	b, err := Bytes(packet)
	require.Nil(t, err)
	assert.Equal(t, raw, b)
}

func TestParseDefaultDataSet(t *testing.T) {
	raw := []uint8("\x0d\x12\x00\x4a\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x48\x57\xdd\xff\xfe\x0e\x91\xda\x00\x00\x00\x00\x04\x7f\x00\x00\x00\x00\x00\x00\x00\x00\xb7\x5f\x00\x00\x02\x00\x00\x01\x00\x16\x20\x00\x03\x00\x00\x01\x80\xff\xfe\xff\xff\x80\x48\x57\xdd\xff\xfe\x0e\x91\xda\x00\x00\x00\x00")
	packet := new(Management)
	err := FromBytes(raw, packet)
	require.Nil(t, err)
	want := Management{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:     NewSdoIDAndMsgType(MessageManagement, 0),
				Version:             Version,
				MessageLength:       uint16(len(raw) - 2),
				DomainNumber:        0,
				MinorSdoID:          0,
				FlagField:           0,
				CorrectionField:     0,
				MessageTypeSpecific: 0,
				SourcePortIdentity: PortIdentity{
					PortNumber:    0,
					ClockIdentity: 5212879185253405146,
				},
				SequenceID:         0,
				ControlField:       4,
				LogMessageInterval: 0x7f,
			},
			TargetPortIdentity: PortIdentity{
				PortNumber:    46943,
				ClockIdentity: 0,
			},
			ActionField: RESPONSE,
		},
		TLV: &DefaultDataSetTLV{
			ManagementTLVHead: ManagementTLVHead{
				TLVHead: TLVHead{
					TLVType:     TLVManagement,
					LengthField: 22,
				},
				ManagementID: IDDefaultDataSet,
			},
			SoTSC:       3,
			NumberPorts: 1,
			Priority1:   128,
			ClockQuality: ClockQuality{
				ClockClass:              ClockClassSlaveOnly,
				ClockAccuracy:           ClockAccuracyUnknown,
				OffsetScaledLogVariance: 65535,
			},
			Priority2:     128,
			ClockIdentity: 5212879185253405146,
			DomainNumber:  0,
		},
	}
	require.Equal(t, want, *packet)
	b, err := Bytes(packet)
	require.Nil(t, err)
	assert.Equal(t, raw, b)
}

func TestParseClockAccuracy(t *testing.T) {
	raw := []uint8{0x0d, 0x02, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x48, 0x57, 0xdd, 0xff, 0xfe, 0x08, 0x64, 0x88, 0x00, 0x00,
		0x00, 0x01, 0x04, 0x7f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xdc, 0x6c, 0x00, 0x00, 0x02, 0x00, 0x00, 0x01,
		0x00, 0x04, 0x20, 0x10, 0x21, 0x00, 0x00, 0x00,
	}
	packet := new(Management)
	err := FromBytes(raw, packet)
	require.Nil(t, err)
	want := Management{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:     NewSdoIDAndMsgType(MessageManagement, 0),
				Version:             MajorVersion,
				MessageLength:       8,
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
		TLV: &ClockAccuracyTLV{
			ManagementTLVHead: ManagementTLVHead{
				TLVHead: TLVHead{
					TLVType:     TLVManagement,
					LengthField: 4,
				},
				ManagementID: IDClockAccuracy,
			},
			ClockAccuracy: ClockAccuracyNanosecond100,
			Reserved:      0,
		},
	}

	require.Equal(t, want, *packet)
	b, err := Bytes(packet)
	require.Nil(t, err)
	assert.Equal(t, raw, b)
}
