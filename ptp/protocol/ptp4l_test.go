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
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTimeStatusNP(t *testing.T) {
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

func TestTimeStatusNPRequest(t *testing.T) {
	req := TimeStatusNPRequest()
	// it's normally generated from PID, set to know value
	req.ManagementMsgHead.Header.SourcePortIdentity.PortNumber = 12345

	raw, err := Bytes(req)
	want := []byte{
		0xd, 0x12, 0x0, 0x36, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x30, 0x39, 0x0, 0x0, 0x0, 0x7f, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x2, 0xc0, 0x0, 0x0, 0x0}
	require.Nil(t, err)
	require.Equal(t, want, raw)
}

func TestParsePortStatsNP(t *testing.T) {
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

func TestPortStatsNPRequest(t *testing.T) {
	req := PortStatsNPRequest()
	// it's normally generated from PID, set to know value
	req.ManagementMsgHead.Header.SourcePortIdentity.PortNumber = 12345

	raw, err := Bytes(req)
	want := []byte{
		0xd, 0x12, 0x0, 0x36, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x30, 0x39, 0x0, 0x0, 0x0, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x2, 0xc0, 0x5, 0x0, 0x0}
	require.Nil(t, err)
	require.Equal(t, want, raw)
}

func TestParsePortServiceStatsNP(t *testing.T) {
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

func TestPortServiceStatsNPRequest(t *testing.T) {
	req := PortServiceStatsNPRequest()
	// it's normally generated from PID, set to know value
	req.ManagementMsgHead.Header.SourcePortIdentity.PortNumber = 12345

	raw, err := Bytes(req)
	want := []byte{
		0xd, 0x12, 0x0, 0x36, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x30, 0x39, 0x0, 0x0, 0x0, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x2, 0xc0, 0x7, 0x0, 0x0}
	require.Nil(t, err)
	require.Equal(t, want, raw)
}

func TestParsePortPropertiesNP(t *testing.T) {
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

func TestPortPropertiesNPRequest(t *testing.T) {
	req := PortPropertiesNPRequest()
	// it's normally generated from PID, set to know value
	req.ManagementMsgHead.Header.SourcePortIdentity.PortNumber = 12345

	raw, err := Bytes(req)
	want := []byte{
		0xd, 0x12, 0x0, 0x36, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x30, 0x39, 0x0, 0x0, 0x0, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x2, 0xc0, 0x4, 0x0, 0x0}
	require.Nil(t, err)
	require.Equal(t, want, raw)
}

func TestParseUnicastMasterTableNP(t *testing.T) {
	raw := []uint8("\x0d\x12\x01\x82\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x48\x57\xdd\xff\xfe\x0e\x91\xda\x00\x01\x00\x00\x04\x7f\x00\x00\x00\x00\x00\x00\x00\x00\xf7\xb0\x00\x00\x02\x00\x00\x01\x01\x4e\xc0\x08\x00\x09\xb8\xce\xf6\xff\xfe\x73\x49\xd4\x00\x01\x06\x21\x59\xe0\x00\x01\x80\x80\x00\x02\x00\x10\x24\x01\xdb\x00\x25\x15\xf0\x01\xfa\xce\x00\x00\x02\xa3\x00\x00\xb8\xce\xf6\xff\xfe\x02\x10\xe4\x00\x01\x06\x21\x59\xe0\x00\x01\x80\x80\x00\x02\x00\x10\x24\x01\xdb\x00\x25\x15\xf0\x01\xfa\xce\x00\x00\x03\xd1\x00\x00\xb8\xce\xf6\xff\xfe\x05\x7e\x20\x00\x01\x06\x21\x59\xe0\x00\x01\x80\x80\x00\x02\x00\x10\x24\x01\xdb\x00\x25\x15\xf0\x01\xfa\xce\x00\x00\x03\xfa\x00\x00\xb8\xce\xf6\xff\xfe\x73\x49\xdc\x00\x01\x06\x21\x59\xe0\x00\x01\x80\x80\x00\x02\x00\x10\x24\x01\xdb\x00\x25\x15\xf0\x01\xfa\xce\x00\x00\x00\xda\x00\x00\xb8\xce\xf6\xff\xfe\x02\x10\xdc\x00\x01\x06\x21\x59\xe0\x01\x02\x80\x80\x00\x02\x00\x10\x24\x01\xdb\x00\x25\x15\xf0\x02\xfa\xce\x00\x00\x01\x1b\x00\x00\xb8\xce\xf6\xff\xfe\x73\x49\xc4\x00\x01\x06\x21\x59\xe0\x00\x01\x80\x80\x00\x02\x00\x10\x24\x01\xdb\x00\x25\x15\xf0\x02\xfa\xce\x00\x00\x01\xec\x00\x00\xb8\xce\xf6\xff\xfe\x73\x49\xcc\x00\x01\x06\x21\x59\xe0\x00\x01\x80\x80\x00\x02\x00\x10\x24\x01\xdb\x00\x25\x15\xf0\x02\xfa\xce\x00\x00\x00\x94\x00\x00\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01\x00\x04\xc0\xa8\x00\x01\xb8\xce\xf6\xff\xfe\x73\x49\xc8\x00\x01\x06\x21\x59\xe0\x00\x01\x80\x80\x00\x02\x00\x10\x24\x01\xdb\x00\x25\x15\xf0\x02\xfa\xce\x00\x00\x00\xb7\x00\x00\x00\x00")
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
				PortNumber:    63408,
				ClockIdentity: 0,
			},
			ActionField: RESPONSE,
		},
		TLV: &UnicastMasterTableNPTLV{
			ManagementTLVHead: ManagementTLVHead{
				TLVHead: TLVHead{
					TLVType:     TLVManagement,
					LengthField: 334,
				},
				ManagementID: IDUnicastMasterTableNP,
			},
			UnicastMasterTable: UnicastMasterTable{
				ActualTableSize: 9,
				UnicastMasters: []UnicastMasterEntry{
					{
						PortIdentity: PortIdentity{
							ClockIdentity: 13316852727527197140,
							PortNumber:    1,
						},
						ClockQuality: ClockQuality{
							ClockClass:              6,
							ClockAccuracy:           33,
							OffsetScaledLogVariance: 23008,
						},
						Selected:  false,
						PortState: UnicastMasterStateHaveAnnounce,
						Priority1: 128,
						Priority2: 128,
						Address:   net.ParseIP("2401:db00:2515:f001:face:0:2a3:0"),
					},
					{
						PortIdentity: PortIdentity{
							ClockIdentity: 13316852727519776996,
							PortNumber:    1,
						},
						ClockQuality: ClockQuality{
							ClockClass:              6,
							ClockAccuracy:           33,
							OffsetScaledLogVariance: 23008,
						},
						Selected:  false,
						PortState: UnicastMasterStateHaveAnnounce,
						Priority1: 128,
						Priority2: 128,
						Address:   net.ParseIP("2401:db00:2515:f001:face:0:3d1:0"),
					},
					{
						PortIdentity: PortIdentity{
							ClockIdentity: 13316852727520001568,
							PortNumber:    1,
						},
						ClockQuality: ClockQuality{
							ClockClass:              6,
							ClockAccuracy:           33,
							OffsetScaledLogVariance: 23008,
						},
						Selected:  false,
						PortState: UnicastMasterStateHaveAnnounce,
						Priority1: 128,
						Priority2: 128,
						Address:   net.ParseIP("2401:db00:2515:f001:face:0:3fa:0"),
					},
					{
						PortIdentity: PortIdentity{
							ClockIdentity: 13316852727527197148,
							PortNumber:    1,
						},
						ClockQuality: ClockQuality{
							ClockClass:              6,
							ClockAccuracy:           33,
							OffsetScaledLogVariance: 23008,
						},
						Selected:  false,
						PortState: UnicastMasterStateHaveAnnounce,
						Priority1: 128,
						Priority2: 128,
						Address:   net.ParseIP("2401:db00:2515:f001:face:0:da:0"),
					},
					{
						PortIdentity: PortIdentity{
							ClockIdentity: 13316852727519776988,
							PortNumber:    1,
						},
						ClockQuality: ClockQuality{
							ClockClass:              6,
							ClockAccuracy:           33,
							OffsetScaledLogVariance: 23008,
						},
						Selected:  true,
						PortState: UnicastMasterStateNeedSYDY,
						Priority1: 128,
						Priority2: 128,
						Address:   net.ParseIP("2401:db00:2515:f002:face:0:11b:0"),
					},
					{
						PortIdentity: PortIdentity{
							ClockIdentity: 13316852727527197124,
							PortNumber:    1,
						},
						ClockQuality: ClockQuality{
							ClockClass:              6,
							ClockAccuracy:           33,
							OffsetScaledLogVariance: 23008,
						},
						Selected:  false,
						PortState: UnicastMasterStateHaveAnnounce,
						Priority1: 128,
						Priority2: 128,
						Address:   net.ParseIP("2401:db00:2515:f002:face:0:1ec:0"),
					},
					{
						PortIdentity: PortIdentity{
							ClockIdentity: 13316852727527197132,
							PortNumber:    1,
						},
						ClockQuality: ClockQuality{
							ClockClass:              6,
							ClockAccuracy:           33,
							OffsetScaledLogVariance: 23008,
						},
						Selected:  false,
						PortState: UnicastMasterStateHaveAnnounce,
						Priority1: 128,
						Priority2: 128,
						Address:   net.ParseIP("2401:db00:2515:f002:face:0:94:0"),
					},
					{
						PortIdentity: PortIdentity{
							ClockIdentity: 18446744073709551615,
							PortNumber:    65535,
						},
						ClockQuality: ClockQuality{
							ClockClass:              0,
							ClockAccuracy:           0,
							OffsetScaledLogVariance: 0,
						},
						Selected:  false,
						PortState: UnicastMasterStateWait,
						Priority1: 0,
						Priority2: 0,
						Address:   net.ParseIP("192.168.0.1").To4(),
					},
					{
						PortIdentity: PortIdentity{
							ClockIdentity: 13316852727527197128,
							PortNumber:    1,
						},
						ClockQuality: ClockQuality{
							ClockClass:              6,
							ClockAccuracy:           33,
							OffsetScaledLogVariance: 23008,
						},
						Selected:  false,
						PortState: UnicastMasterStateHaveAnnounce,
						Priority1: 128,
						Priority2: 128,
						Address:   net.ParseIP("2401:db00:2515:f002:face:0:b7:0"),
					},
				},
			},
		},
	}
	require.Equal(t, want, *packet)
	b, err := Bytes(packet)
	require.Nil(t, err)
	require.Equal(t, raw, b)
}

func TestUnicastMasterTableNPRequest(t *testing.T) {
	req := UnicastMasterTableNPRequest()
	// it's normally generated from PID, set to know value
	req.ManagementMsgHead.Header.SourcePortIdentity.PortNumber = 12345

	raw, err := Bytes(req)
	want := []byte{
		0xd, 0x12, 0x0, 0x36, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0,
		0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x30, 0x39, 0x0, 0x0, 0x0, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x2, 0xc0, 0x8, 0x0, 0x0}
	require.Nil(t, err)
	require.Equal(t, want, raw)
}
