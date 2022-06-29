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

package checker

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/stretchr/testify/require"
)

// some PTP packets we'll use
var (
	managementError = &ptp.ManagementMsgErrorStatus{
		ManagementMsgHead: ptp.ManagementMsgHead{
			Header: ptp.Header{
				SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageManagement, 0),
				Version:         ptp.MajorVersion,
				MessageLength:   66,
				SourcePortIdentity: ptp.PortIdentity{
					PortNumber:    0,
					ClockIdentity: 5212879185253000328,
				},
				SequenceID:         1,
				ControlField:       4,
				LogMessageInterval: 0x7f,
			},
			TargetPortIdentity: ptp.PortIdentity{
				PortNumber:    56428,
				ClockIdentity: 0,
			},
			ActionField: ptp.RESPONSE,
		},
		ManagementErrorStatusTLV: ptp.ManagementErrorStatusTLV{
			TLVHead: ptp.TLVHead{
				TLVType:     ptp.TLVManagementErrorStatus,
				LengthField: 8,
			},
			ManagementErrorID: ptp.ErrorNotSupported,
			ManagementID:      ptp.IDCurrentDataSet,
			DisplayData:       ptp.PTPText("Ohno"),
		},
	}

	currentDataSet = &ptp.Management{
		ManagementMsgHead: ptp.ManagementMsgHead{
			Header: ptp.Header{
				SdoIDAndMsgType:     ptp.NewSdoIDAndMsgType(ptp.MessageManagement, 0),
				Version:             ptp.Version,
				MessageLength:       74,
				DomainNumber:        0,
				MinorSdoID:          0,
				FlagField:           0,
				CorrectionField:     0,
				MessageTypeSpecific: 0,
				SourcePortIdentity: ptp.PortIdentity{
					PortNumber:    0,
					ClockIdentity: 5212879185253405146,
				},
				SequenceID:         0,
				ControlField:       4,
				LogMessageInterval: 0x7f,
			},
			TargetPortIdentity: ptp.PortIdentity{
				PortNumber:    49810,
				ClockIdentity: 0,
			},
			ActionField: ptp.RESPONSE,
		},
		TLV: &ptp.CurrentDataSetTLV{
			ManagementTLVHead: ptp.ManagementTLVHead{
				TLVHead: ptp.TLVHead{
					TLVType:     ptp.TLVManagement,
					LengthField: 20,
				},
				ManagementID: ptp.IDCurrentDataSet,
			},
			StepsRemoved:     1,
			OffsetFromMaster: ptp.NewTimeInterval(-768652.0),
			MeanPathDelay:    ptp.NewTimeInterval(42013430.0),
		},
	}

	defaultDataSet = &ptp.Management{
		ManagementMsgHead: ptp.ManagementMsgHead{
			Header: ptp.Header{
				SdoIDAndMsgType:     ptp.NewSdoIDAndMsgType(ptp.MessageManagement, 0),
				Version:             ptp.Version,
				MessageLength:       0x4a,
				DomainNumber:        0,
				MinorSdoID:          0,
				FlagField:           0,
				CorrectionField:     0,
				MessageTypeSpecific: 0,
				SourcePortIdentity: ptp.PortIdentity{
					PortNumber:    0,
					ClockIdentity: 5212879185253405146,
				},
				SequenceID:         0,
				ControlField:       4,
				LogMessageInterval: 0x7f,
			},
			TargetPortIdentity: ptp.PortIdentity{
				PortNumber:    46943,
				ClockIdentity: 0,
			},
			ActionField: ptp.RESPONSE,
		},
		TLV: &ptp.DefaultDataSetTLV{
			ManagementTLVHead: ptp.ManagementTLVHead{
				TLVHead: ptp.TLVHead{
					TLVType:     ptp.TLVManagement,
					LengthField: 22,
				},
				ManagementID: ptp.IDDefaultDataSet,
			},
			SoTSC:       3,
			NumberPorts: 1,
			Priority1:   128,
			ClockQuality: ptp.ClockQuality{
				ClockClass:              ptp.ClockClassSlaveOnly,
				ClockAccuracy:           ptp.ClockAccuracyUnknown,
				OffsetScaledLogVariance: 65535,
			},
			Priority2:     128,
			ClockIdentity: 5212879185253405146,
			DomainNumber:  0,
		},
	}

	parentDataSet = &ptp.Management{
		ManagementMsgHead: ptp.ManagementMsgHead{
			Header: ptp.Header{
				SdoIDAndMsgType:     ptp.NewSdoIDAndMsgType(ptp.MessageManagement, 0),
				Version:             ptp.Version,
				MessageLength:       0x56,
				DomainNumber:        0,
				MinorSdoID:          0,
				FlagField:           0,
				CorrectionField:     0,
				MessageTypeSpecific: 0,
				SourcePortIdentity: ptp.PortIdentity{
					PortNumber:    0,
					ClockIdentity: 5212879185253405146,
				},
				SequenceID:         0,
				ControlField:       4,
				LogMessageInterval: 0x7f,
			},
			TargetPortIdentity: ptp.PortIdentity{
				PortNumber:    50367,
				ClockIdentity: 0,
			},
			ActionField: ptp.RESPONSE,
		},
		TLV: &ptp.ParentDataSetTLV{
			ManagementTLVHead: ptp.ManagementTLVHead{
				TLVHead: ptp.TLVHead{
					TLVType:     ptp.TLVManagement,
					LengthField: 34,
				},
				ManagementID: ptp.IDParentDataSet,
			},
			ParentPortIdentity: ptp.PortIdentity{
				PortNumber:    1,
				ClockIdentity: 13316852727519776988,
			},
			ObservedParentOffsetScaledLogVariance: 65535,
			ObservedParentClockPhaseChangeRate:    2147483647,
			GrandmasterPriority1:                  128,
			GrandmasterClockQuality: ptp.ClockQuality{
				ClockClass:              ptp.ClockClass6,
				ClockAccuracy:           ptp.ClockAccuracyNanosecond250,
				OffsetScaledLogVariance: 23008,
			},
			GrandmasterPriority2: 128,
			GrandmasterIdentity:  13316852727519776988,
		},
	}

	portStatsNP = &ptp.Management{
		ManagementMsgHead: ptp.ManagementMsgHead{
			Header: ptp.Header{
				SdoIDAndMsgType:     ptp.NewSdoIDAndMsgType(ptp.MessageManagement, 0),
				Version:             ptp.Version,
				MessageLength:       324,
				DomainNumber:        0,
				MinorSdoID:          0,
				FlagField:           0,
				CorrectionField:     0,
				MessageTypeSpecific: 0,
				SourcePortIdentity: ptp.PortIdentity{
					PortNumber:    1,
					ClockIdentity: 5212879185253405146,
				},
				SequenceID:         0,
				ControlField:       4,
				LogMessageInterval: 0x7f,
			},
			TargetPortIdentity: ptp.PortIdentity{
				PortNumber:    2954,
				ClockIdentity: 0,
			},
			ActionField: ptp.RESPONSE,
		},
		TLV: &ptp.PortStatsNPTLV{
			ManagementTLVHead: ptp.ManagementTLVHead{
				TLVHead: ptp.TLVHead{
					TLVType:     ptp.TLVManagement,
					LengthField: 268,
				},
				ManagementID: ptp.IDPortStatsNP,
			},
			PortIdentity: ptp.PortIdentity{ // 4857dd.fffe.0e91da-1
				ClockIdentity: 5212879185253405146,
				PortNumber:    1,
			},
			PortStats: ptp.PortStats{
				RXMsgType: [16]uint64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
				TXMsgType: [16]uint64{3921, 0, 0, 0, 0, 0, 0, 0, 3921, 0, 0, 1962, 0, 0, 0, 0},
			},
		},
	}

	timeStatusNP = &ptp.Management{
		ManagementMsgHead: ptp.ManagementMsgHead{
			Header: ptp.Header{
				SdoIDAndMsgType:     ptp.NewSdoIDAndMsgType(ptp.MessageManagement, 0),
				Version:             ptp.MajorVersion,
				MessageLength:       84,
				DomainNumber:        0,
				MinorSdoID:          0,
				FlagField:           0,
				CorrectionField:     0,
				MessageTypeSpecific: 0,
				SourcePortIdentity: ptp.PortIdentity{
					PortNumber:    0,
					ClockIdentity: 5212879185253000328,
				},
				SequenceID:         5,
				ControlField:       4,
				LogMessageInterval: 0x7f,
			},
			TargetPortIdentity: ptp.PortIdentity{
				PortNumber:    6678,
				ClockIdentity: 0,
			},
			ActionField: ptp.RESPONSE,
		},
		TLV: &ptp.TimeStatusNPTLV{
			ManagementTLVHead: ptp.ManagementTLVHead{
				TLVHead: ptp.TLVHead{
					TLVType:     ptp.TLVManagement,
					LengthField: 52,
				},
				ManagementID: ptp.IDTimeStatusNP,
			},
			MasterOffsetNS:             24536521,
			IngressTimeNS:              1615472167079671311,
			CumulativeScaledRateOffset: 0,
			ScaledLastGmPhaseChange:    0,
			GMTimeBaseIndicator:        0,
			LastGmPhaseChange: ptp.ScaledNS{
				NanosecondsMSB:        0,
				NanosecondsLSB:        0,
				FractionalNanoseconds: 0,
			},
			GMPresent:  1,
			GMIdentity: 2632925728215085210, // 248a07.fffe.3f309a,
		},
	}

	portServiceStatsNP = &ptp.Management{
		ManagementMsgHead: ptp.ManagementMsgHead{
			Header: ptp.Header{
				SdoIDAndMsgType:     ptp.NewSdoIDAndMsgType(ptp.MessageManagement, 0),
				Version:             ptp.Version,
				MessageLength:       162,
				DomainNumber:        0,
				MinorSdoID:          0,
				FlagField:           0,
				CorrectionField:     0,
				MessageTypeSpecific: 0,
				SourcePortIdentity: ptp.PortIdentity{
					PortNumber:    1,
					ClockIdentity: 5212879185253405146,
				},
				SequenceID:         0,
				ControlField:       4,
				LogMessageInterval: 0x7f,
			},
			TargetPortIdentity: ptp.PortIdentity{
				PortNumber:    3757,
				ClockIdentity: 0,
			},
			ActionField: ptp.RESPONSE,
		},
		TLV: &ptp.PortServiceStatsNPTLV{
			ManagementTLVHead: ptp.ManagementTLVHead{
				TLVHead: ptp.TLVHead{
					TLVType:     ptp.TLVManagement,
					LengthField: 92,
				},
				ManagementID: ptp.IDPortServiceStatsNP,
			},
			PortIdentity: ptp.PortIdentity{ // 4857dd.fffe.0e91da-1
				ClockIdentity: 5212879185253405146,
				PortNumber:    1,
			},
			PortServiceStats: ptp.PortServiceStats{
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

func prepareTestClient(t *testing.T, packets ...ptp.Packet) (*fakeConn, *ptp.MgmtClient) {
	outputs := []*bytes.Buffer{}
	for _, packet := range packets {
		buf := &bytes.Buffer{}
		b, err := ptp.Bytes(packet)
		require.NoError(t, err)
		err = binary.Write(buf, binary.BigEndian, b)
		require.NoError(t, err)
		outputs = append(outputs, buf)
	}
	conn := newConn(outputs)
	return conn, &ptp.MgmtClient{Sequence: 1, Connection: conn}
}

func TestCheckerRunEmpty(t *testing.T) {
	_, client := prepareTestClient(t)
	res, err := Run(client)
	require.EqualError(t, err, "getting CURRENT_DATA_SET management TLV: EOF")
	require.Nil(t, res)
}

func TestCheckerRunErrorMsg(t *testing.T) {
	_, client := prepareTestClient(t, managementError)
	res, err := Run(client)
	require.EqualError(t, err, "getting CURRENT_DATA_SET management TLV: got Management Error in response: NOT_SUPPORTED")
	require.Nil(t, res)

	_, client = prepareTestClient(t, currentDataSet, managementError)
	res, err = Run(client)
	require.EqualError(t, err, "getting DEFAULT_DATA_SET management TLV: got Management Error in response: NOT_SUPPORTED")
	require.Nil(t, res)

	_, client = prepareTestClient(t, currentDataSet, defaultDataSet, managementError)
	res, err = Run(client)
	require.EqualError(t, err, "getting PARENT_DATA_SET management TLV: got Management Error in response: NOT_SUPPORTED")
	require.Nil(t, res)
}

func TestCheckerRunWithoutNP(t *testing.T) {
	_, client := prepareTestClient(t, currentDataSet, defaultDataSet, parentDataSet, managementError)
	res, err := Run(client)
	require.NoError(t, err)

	want := &PTPCheckResult{
		OffsetFromMasterNS:  (currentDataSet.TLV.(*ptp.CurrentDataSetTLV)).OffsetFromMaster.Nanoseconds(),
		GrandmasterPresent:  true,
		MeanPathDelayNS:     (currentDataSet.TLV.(*ptp.CurrentDataSetTLV)).MeanPathDelay.Nanoseconds(),
		StepsRemoved:        int((currentDataSet.TLV.(*ptp.CurrentDataSetTLV)).StepsRemoved),
		ClockIdentity:       (defaultDataSet.TLV.(*ptp.DefaultDataSetTLV)).ClockIdentity.String(),
		GrandmasterIdentity: (parentDataSet.TLV.(*ptp.ParentDataSetTLV)).GrandmasterIdentity.String(),
		PortStatsTX:         map[string]uint64{},
		PortStatsRX:         map[string]uint64{},
	}
	require.Equal(t, want, res)
}

func TestCheckerRunWithPortStats(t *testing.T) {
	_, client := prepareTestClient(t, currentDataSet, defaultDataSet, parentDataSet, portStatsNP, managementError)
	res, err := Run(client)
	require.NoError(t, err)

	want := &PTPCheckResult{
		OffsetFromMasterNS:  (currentDataSet.TLV.(*ptp.CurrentDataSetTLV)).OffsetFromMaster.Nanoseconds(),
		GrandmasterPresent:  true,
		MeanPathDelayNS:     (currentDataSet.TLV.(*ptp.CurrentDataSetTLV)).MeanPathDelay.Nanoseconds(),
		StepsRemoved:        int((currentDataSet.TLV.(*ptp.CurrentDataSetTLV)).StepsRemoved),
		ClockIdentity:       (defaultDataSet.TLV.(*ptp.DefaultDataSetTLV)).ClockIdentity.String(),
		GrandmasterIdentity: (parentDataSet.TLV.(*ptp.ParentDataSetTLV)).GrandmasterIdentity.String(),
		PortStatsTX: map[string]uint64{
			"ANNOUNCE":              1962,
			"DELAY_REQ":             0,
			"DELAY_RESP":            0,
			"FOLLOW_UP":             3921,
			"MANAGEMENT":            0,
			"PDELAY_REQ":            0,
			"PDELAY_RES":            0,
			"PDELAY_RESP_FOLLOW_UP": 0,
			"SIGNALING":             0,
			"SYNC":                  3921,
		},
		PortStatsRX: map[string]uint64{
			"ANNOUNCE":              0,
			"DELAY_REQ":             0,
			"DELAY_RESP":            0,
			"FOLLOW_UP":             0,
			"MANAGEMENT":            0,
			"PDELAY_REQ":            0,
			"PDELAY_RES":            0,
			"PDELAY_RESP_FOLLOW_UP": 0,
			"SIGNALING":             0,
			"SYNC":                  0,
		},
	}
	require.Equal(t, want, res)
}

func TestCheckerRunFull(t *testing.T) {
	_, client := prepareTestClient(t, currentDataSet, defaultDataSet, parentDataSet, portStatsNP, timeStatusNP, portServiceStatsNP)
	res, err := Run(client)
	require.NoError(t, err)

	want := &PTPCheckResult{
		OffsetFromMasterNS:  (currentDataSet.TLV.(*ptp.CurrentDataSetTLV)).OffsetFromMaster.Nanoseconds(),
		GrandmasterPresent:  true,
		MeanPathDelayNS:     (currentDataSet.TLV.(*ptp.CurrentDataSetTLV)).MeanPathDelay.Nanoseconds(),
		StepsRemoved:        int((currentDataSet.TLV.(*ptp.CurrentDataSetTLV)).StepsRemoved),
		ClockIdentity:       (defaultDataSet.TLV.(*ptp.DefaultDataSetTLV)).ClockIdentity.String(),
		GrandmasterIdentity: (parentDataSet.TLV.(*ptp.ParentDataSetTLV)).GrandmasterIdentity.String(),
		IngressTimeNS:       (timeStatusNP.TLV.(*ptp.TimeStatusNPTLV)).IngressTimeNS,
		PortStatsTX: map[string]uint64{
			"ANNOUNCE":              1962,
			"DELAY_REQ":             0,
			"DELAY_RESP":            0,
			"FOLLOW_UP":             3921,
			"MANAGEMENT":            0,
			"PDELAY_REQ":            0,
			"PDELAY_RES":            0,
			"PDELAY_RESP_FOLLOW_UP": 0,
			"SIGNALING":             0,
			"SYNC":                  3921,
		},
		PortStatsRX: map[string]uint64{
			"ANNOUNCE":              0,
			"DELAY_REQ":             0,
			"DELAY_RESP":            0,
			"FOLLOW_UP":             0,
			"MANAGEMENT":            0,
			"PDELAY_REQ":            0,
			"PDELAY_RES":            0,
			"PDELAY_RESP_FOLLOW_UP": 0,
			"SIGNALING":             0,
			"SYNC":                  0,
		},
		PortServiceStats: &(portServiceStatsNP.TLV.(*ptp.PortServiceStatsNPTLV)).PortServiceStats,
	}
	require.Equal(t, want, res)
}

func TestPrepareLocalConn(t *testing.T) {
	dir, err := os.MkdirTemp("", "ptpcheck_test")
	require.NoError(t, err)
	defer os.RemoveAll(dir) // clean up
	targetSocketPath := filepath.Join(dir, "ptp4l")
	// create fake listener
	addr, _ := net.ResolveUnixAddr("unixgram", targetSocketPath)
	listener, err := net.ListenUnixgram("unixgram", addr)
	require.NoError(t, err)
	defer listener.Close()

	conn, cleanup, err := prepareConn(targetSocketPath)
	require.NoError(t, err)
	localFile := (conn.LocalAddr().(*net.UnixAddr)).Name
	require.NotEqual(t, "", localFile)
	stat, err := os.Stat(localFile)
	require.NoError(t, err)
	require.Equal(t, os.ModeSocket, stat.Mode().Type())

	// make sure we clean things up
	cleanup()
	_, err = os.Stat(localFile)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestPrepareLocalConnEmpty(t *testing.T) {
	dir, err := os.MkdirTemp("", "ptpcheck_test")
	require.NoError(t, err)
	defer os.RemoveAll(dir) // clean up
	targetSocketPath := filepath.Join(dir, "ptp4l")
	conn, cleanup, err := prepareConn(targetSocketPath)
	require.Error(t, err)
	require.Nil(t, conn)
	require.NotNil(t, cleanup)
}

func TestPrepareLocalConnError(t *testing.T) {
	conn, cleanup, err := prepareConn("")
	require.EqualError(t, err, "preparing ptp4l connection: target address is empty")
	require.Nil(t, conn)
	require.NotNil(t, cleanup)
}
