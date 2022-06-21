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
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeConn gives us fake io.ReadWriter interacted implementation for which we can provide fake outputs
type fakeConn struct {
	readCount int
	inputs    [][]byte
	outputs   []*bytes.Buffer
}

func newConn(outputs []*bytes.Buffer) *fakeConn {
	return &fakeConn{
		readCount: 0,
		outputs:   outputs,
		inputs:    [][]byte{},
	}
}

func (c *fakeConn) Read(p []byte) (n int, err error) {
	pos := c.readCount
	if c.readCount < len(c.outputs) {
		c.readCount++
		return c.outputs[pos].Read(p)
	}
	return 0, fmt.Errorf("EOF")
}

func (c *fakeConn) Write(p []byte) (n int, err error) {
	c.inputs = append(c.inputs, p)
	return 0, nil
}

func prepareTestClient(t *testing.T, packet Packet) (*fakeConn, *MgmtClient) {
	buf := &bytes.Buffer{}
	b, err := Bytes(packet)
	require.NoError(t, err)
	err = binary.Write(buf, binary.BigEndian, b)
	require.NoError(t, err)
	conn := newConn([]*bytes.Buffer{
		buf,
	})
	return conn, &MgmtClient{Sequence: 1, Connection: conn}
}

// Test if we have errors when there is nothing on the line to read
func TestMgmtClientCommunicateEOF(t *testing.T) {
	conn := newConn([]*bytes.Buffer{
		bytes.NewBuffer([]byte{}),
	})
	client := MgmtClient{Sequence: 1, Connection: conn}
	_, err := client.Communicate(CurrentDataSetRequest())
	require.Error(t, err)
}

func TestMgmtClientCommunicateError(t *testing.T) {
	var err error
	packet := &ManagementMsgErrorStatus{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:     NewSdoIDAndMsgType(MessageManagement, 0),
				Version:             MajorVersion,
				MessageLength:       62,
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
	_, client := prepareTestClient(t, packet)
	_, err = client.Communicate(CurrentDataSetRequest())
	require.EqualError(t, err, "got Management Error in response: NOT_SUPPORTED")
}

func TestMgmtClientCommunicateOK(t *testing.T) {
	var err error
	packet := &Management{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:     NewSdoIDAndMsgType(MessageManagement, 0),
				Version:             Version,
				MessageLength:       74,
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
	conn, client := prepareTestClient(t, packet)
	req := CurrentDataSetRequest()
	got, err := client.Communicate(req)
	require.NoError(t, err)
	require.Equal(t, packet, got)

	// check that we received proper request
	b, err := req.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, 1, len(conn.inputs))
	require.Equal(t, conn.inputs[0], b)
}

func TestMgmtClientCurrentDataSet(t *testing.T) {
	var err error
	packet := &Management{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:     NewSdoIDAndMsgType(MessageManagement, 0),
				Version:             Version,
				MessageLength:       74,
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
	conn, client := prepareTestClient(t, packet)
	got, err := client.CurrentDataSet()
	require.NoError(t, err)
	require.Equal(t, packet.TLV, got)

	// check that we received proper request
	req := CurrentDataSetRequest()
	req.SetSequence(client.Sequence)
	b, err := req.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, 1, len(conn.inputs))
	require.Equal(t, conn.inputs[0], b)
}

func TestMgmtClientParentDataSet(t *testing.T) {
	var err error
	packet := &Management{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:     NewSdoIDAndMsgType(MessageManagement, 0),
				Version:             Version,
				MessageLength:       uint16(0x56),
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
	conn, client := prepareTestClient(t, packet)
	got, err := client.ParentDataSet()
	require.NoError(t, err)
	require.Equal(t, packet.TLV, got)

	// check that we received proper request
	req := ParentDataSetRequest()
	req.SetSequence(client.Sequence)
	b, err := req.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, 1, len(conn.inputs))
	require.Equal(t, conn.inputs[0], b)
}

func TestMgmtClientDefaultDataSet(t *testing.T) {
	var err error
	packet := &Management{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:     NewSdoIDAndMsgType(MessageManagement, 0),
				Version:             Version,
				MessageLength:       uint16(0x4a),
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
	conn, client := prepareTestClient(t, packet)
	got, err := client.DefaultDataSet()
	require.NoError(t, err)
	require.Equal(t, packet.TLV, got)

	// check that we received proper request
	req := DefaultDataSetRequest()
	req.SetSequence(client.Sequence)
	b, err := req.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, 1, len(conn.inputs))
	require.Equal(t, conn.inputs[0], b)
}

func TestMgmtClientClockAccuracy(t *testing.T) {
	var err error
	packet := &Management{
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
	conn, client := prepareTestClient(t, packet)
	got, err := client.ClockAccuracy()
	require.NoError(t, err)
	require.Equal(t, packet.TLV, got)

	// check that we received proper request
	req := ClockAccuracyRequest()
	req.SetSequence(client.Sequence)
	b, err := req.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, 1, len(conn.inputs))
	require.Equal(t, conn.inputs[0], b)
}

func TestMgmtClientTimeStatusNP(t *testing.T) {
	var err error
	packet := &Management{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:     NewSdoIDAndMsgType(MessageManagement, 0),
				Version:             MajorVersion,
				MessageLength:       104,
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
	conn, client := prepareTestClient(t, packet)
	got, err := client.TimeStatusNP()
	require.NoError(t, err)
	require.Equal(t, packet.TLV, got)

	// check that we received proper request
	req := TimeStatusNPRequest()
	req.SetSequence(client.Sequence)
	b, err := req.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, 1, len(conn.inputs))
	require.Equal(t, conn.inputs[0], b)
}

func TestMgmtClientPortStatsNP(t *testing.T) {
	var err error
	packet := &Management{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:     NewSdoIDAndMsgType(MessageManagement, 0),
				Version:             Version,
				MessageLength:       0x40,
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
	conn, client := prepareTestClient(t, packet)
	got, err := client.PortStatsNP()
	require.NoError(t, err)
	require.Equal(t, packet.TLV, got)

	// check that we received proper request
	req := PortStatsNPRequest()
	req.SetSequence(client.Sequence)
	b, err := req.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, 1, len(conn.inputs))
	require.Equal(t, conn.inputs[0], b)
}

func TestMgmtClientPortServiceStatsNP(t *testing.T) {
	var err error
	packet := &Management{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:     NewSdoIDAndMsgType(MessageManagement, 0),
				Version:             Version,
				MessageLength:       0x90,
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
	conn, client := prepareTestClient(t, packet)
	got, err := client.PortServiceStatsNP()
	require.NoError(t, err)
	require.Equal(t, packet.TLV, got)

	// check that we received proper request
	req := PortServiceStatsNPRequest()
	req.SetSequence(client.Sequence)
	b, err := req.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, 1, len(conn.inputs))
	require.Equal(t, conn.inputs[0], b)
}

func TestMgmtClientPortPropertiesNP(t *testing.T) {
	var err error
	packet := &Management{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:     NewSdoIDAndMsgType(MessageManagement, 0),
				Version:             Version,
				MessageLength:       0x48,
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
	conn, client := prepareTestClient(t, packet)
	got, err := client.PortPropertiesNP()
	require.NoError(t, err)
	require.Equal(t, packet.TLV, got)

	// check that we received proper request
	req := PortPropertiesNPRequest()
	req.SetSequence(client.Sequence)
	b, err := req.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, 1, len(conn.inputs))
	require.Equal(t, conn.inputs[0], b)
}

func TestMgmtClientUnicastMasterTableNP(t *testing.T) {
	var err error
	packet := &Management{
		ManagementMsgHead: ManagementMsgHead{
			Header: Header{
				SdoIDAndMsgType:     NewSdoIDAndMsgType(MessageManagement, 0),
				Version:             Version,
				MessageLength:       68,
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
					LengthField: 22,
				},
				ManagementID: IDUnicastMasterTableNP,
			},
			UnicastMasterTable: UnicastMasterTable{
				ActualTableSize: 1,
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
						Address:   []byte{192, 168, 0, 10},
					},
				},
			},
		},
	}
	conn, client := prepareTestClient(t, packet)
	got, err := client.UnicastMasterTableNP()
	require.NoError(t, err)
	require.Equal(t, packet.TLV, got)

	// check that we received proper request
	req := UnicastMasterTableNPRequest()
	req.SetSequence(client.Sequence)
	b, err := req.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, 1, len(conn.inputs))
	require.Equal(t, conn.inputs[0], b)
}
