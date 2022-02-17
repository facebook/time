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

func Test_parseTimeStatusNP(t *testing.T) {
	raw := []uint8{
		13, 2, 0, 104, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		72, 87, 221, 255, 254, 8, 100, 136, 0, 0, 0, 5, 4, 127, 0, 0, 0, 0, 0, 0, 0, 0,
		26, 22, 0, 0, 2, 0, 0, 1, 0, 52, 192, 0, 0, 0, 0, 0, 1, 118, 101, 201, 22, 107,
		79, 96, 119, 80, 118, 15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		1, 36, 138, 7, 255, 254, 63, 48, 154, 0, 0,
	}
	packet := new(Management)
	err := FromBytes(raw, packet)
	require.Nil(t, err)
	want := Management{
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
				SequenceID:         5,
				ControlField:       4,
				LogMessageInterval: 0x7f,
			},
			TargetPortIdentity: PortIdentity{
				PortNumber:    6678,
				ClockIdentity: 0,
			},
			ActionField: RESPONSE,
		},
		TLV: &TimeStatusNPTLV{
			ManagementTLVHead: ManagementTLVHead{
				TLVHead: TLVHead{
					TLVType:     TLVManagement,
					LengthField: 52,
				},
				ManagementID: IDTimeStatusNP,
			},
			MasterOffsetNS:             24536521,
			IngressTimeNS:              1615472167079671311,
			CumulativeScaledRateOffset: 0,
			ScaledLastGmPhaseChange:    0,
			GMTimeBaseIndicator:        0,
			LastGmPhaseChange: ScaledNS{
				NanosecondsMSB:        0,
				NanosecondsLSB:        0,
				FractionalNanoseconds: 0,
			},
			GMPresent:  1,
			GMIdentity: 2632925728215085210, // 248a07.fffe.3f309a,
		},
	}
	require.Equal(t, want, *packet)
	b, err := Bytes(packet)
	require.Nil(t, err)
	assert.Equal(t, raw, b)
}

func Test_parsePortStatsNP(t *testing.T) {
	raw := []uint8("\x0d\x12\x01\x40\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x48\x57\xdd\xff\xfe\x0e\x91\xda\x00\x01\x00\x00\x04\x7f\x00\x00\x00\x00\x00\x00\x00\x00\x0b\x8a\x00\x00\x02\x00\x00\x01\x01\x0c\xc0\x05\x48\x57\xdd\xff\xfe\x0e\x91\xda\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x51\x0f\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x51\x0f\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xaa\x07\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
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
					PortNumber:    1,
					ClockIdentity: 5212879185253405146,
				},
				SequenceID:         0,
				ControlField:       4,
				LogMessageInterval: 0x7f,
			},
			TargetPortIdentity: PortIdentity{
				PortNumber:    2954,
				ClockIdentity: 0,
			},
			ActionField: RESPONSE,
		},
		TLV: &PortStatsNPTLV{
			ManagementTLVHead: ManagementTLVHead{
				TLVHead: TLVHead{
					TLVType:     TLVManagement,
					LengthField: 268,
				},
				ManagementID: IDPortStatsNP,
			},
			PortIdentity: PortIdentity{ // 4857dd.fffe.0e91da-1
				ClockIdentity: 5212879185253405146,
				PortNumber:    1,
			},
			PortStats: PortStats{
				RXMsgType: [16]uint64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
				TXMsgType: [16]uint64{3921, 0, 0, 0, 0, 0, 0, 0, 3921, 0, 0, 1962, 0, 0, 0, 0},
			},
		},
	}
	require.Equal(t, want, *packet)
	b, err := Bytes(packet)
	require.Nil(t, err)
	assert.Equal(t, raw, b)
}

func Test_parsePortServiceStatsNP(t *testing.T) {
	raw := []uint8("\x0d\x12\x00\x90\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x48\x57\xdd\xff\xfe\x0e\x91\xda\x00\x01\x00\x00\x04\x7f\x00\x00\x00\x00\x00\x00\x00\x00\x0e\xad\x00\x00\x02\x00\x00\x01\x00\x5c\xc0\x07\x48\x57\xdd\xff\xfe\x0e\x91\xda\x00\x01\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x92\x05\x00\x00\x00\x00\x00\x00\x21\x0b\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
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
					PortNumber:    1,
					ClockIdentity: 5212879185253405146,
				},
				SequenceID:         0,
				ControlField:       4,
				LogMessageInterval: 0x7f,
			},
			TargetPortIdentity: PortIdentity{
				PortNumber:    3757,
				ClockIdentity: 0,
			},
			ActionField: RESPONSE,
		},
		TLV: &PortServiceStatsNPTLV{
			ManagementTLVHead: ManagementTLVHead{
				TLVHead: TLVHead{
					TLVType:     TLVManagement,
					LengthField: 92,
				},
				ManagementID: IDPortServiceStatsNP,
			},
			PortIdentity: PortIdentity{ // 4857dd.fffe.0e91da-1
				ClockIdentity: 5212879185253405146,
				PortNumber:    1,
			},
			PortServiceStats: PortServiceStats{
				AnnounceTimeout:       1,
				SyncTimeout:           0,
				DelayTimeout:          0,
				UnicastServiceTimeout: 0,
				UnicastRequestTimeout: 0,
				MasterAnnounceTimeout: 1426,
				MasterSyncTimeout:     2849,
				QualificationTimeout:  0,
				SyncMismatch:          0,
				FollowupMismatch:      0,
			},
		},
	}
	require.Equal(t, want, *packet)
	b, err := Bytes(packet)
	require.Nil(t, err)
	assert.Equal(t, raw, b)
}

func Test_parsePortPropertiesNP(t *testing.T) {
	raw := []uint8("\x0d\x12\x00\x48\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x48\x57\xdd\xff\xfe\x0e\x91\xda\x00\x01\x00\x00\x04\x7f\x00\x00\x00\x00\x00\x00\x00\x00\x1f\xf2\x00\x00\x02\x00\x00\x01\x00\x14\xc0\x04\x48\x57\xdd\xff\xfe\x0e\x91\xda\x00\x01\x09\x00\x04\x65\x74\x68\x30\x00\x00")
	packet := new(Management)
	err := FromBytes(raw, packet)
	require.Nil(t, err)
	want := Management{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:     NewSdoIDAndMsgType(MessageManagement, 0),
				Version:             Version,
				MessageLength:       uint16(len(raw) - 1),
				DomainNumber:        0,
				MinorSdoID:          0,
				FlagField:           0,
				CorrectionField:     0,
				MessageTypeSpecific: 0,
				SourcePortIdentity: PortIdentity{
					PortNumber:    1,
					ClockIdentity: 5212879185253405146,
				},
				SequenceID:         0,
				ControlField:       4,
				LogMessageInterval: 0x7f,
			},
			TargetPortIdentity: PortIdentity{
				PortNumber:    8178,
				ClockIdentity: 0,
			},
			ActionField: RESPONSE,
		},
		TLV: &PortPropertiesNPTLV{
			ManagementTLVHead: ManagementTLVHead{
				TLVHead: TLVHead{
					TLVType:     TLVManagement,
					LengthField: 20,
				},
				ManagementID: IDPortPropertiesNP,
			},
			PortIdentity: PortIdentity{ // 4857dd.fffe.0e91da-1
				ClockIdentity: 5212879185253405146,
				PortNumber:    1,
			},
			PortState:    PortStateSlave,
			Timestamping: TimestampingSoftware,
			Interface:    "eth0",
		},
	}
	require.Equal(t, want, *packet)
	b, err := Bytes(packet)
	require.Nil(t, err)
	assert.Equal(t, raw, b)
}
