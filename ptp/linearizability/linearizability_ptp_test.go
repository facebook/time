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

package linearizability

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/facebook/time/fbclock/stats"
	ptp "github.com/facebook/time/ptp/protocol"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func grantUnicastPkt(seq int, clockID ptp.ClockIdentity, duration time.Duration, what ptp.MessageType) *ptp.Signaling {
	l := binary.Size(ptp.Header{}) + binary.Size(ptp.PortIdentity{}) + binary.Size(ptp.GrantUnicastTransmissionTLV{})
	return &ptp.Signaling{
		Header: ptp.Header{
			SdoIDAndMsgType:    ptp.NewSdoIDAndMsgType(ptp.MessageSignaling, 0),
			Version:            ptp.Version,
			SequenceID:         uint16(seq),
			MessageLength:      uint16(l),
			FlagField:          ptp.FlagUnicast,
			LogMessageInterval: 0x7f,
		},
		TargetPortIdentity: ptp.PortIdentity{
			PortNumber:    1,
			ClockIdentity: clockID,
		},
		TLVs: []ptp.TLV{
			&ptp.GrantUnicastTransmissionTLV{
				TLVHead: ptp.TLVHead{
					TLVType:     ptp.TLVGrantUnicastTransmission,
					LengthField: uint16(binary.Size(ptp.GrantUnicastTransmissionTLV{}) - binary.Size(ptp.TLVHead{})),
				},
				MsgTypeAndReserved:    ptp.NewUnicastMsgTypeAndFlags(what, 0),
				LogInterMessagePeriod: 1,
				DurationField:         uint32(duration.Seconds()), // seconds
				Renewal:               1,
			},
		},
	}
}

func delayRespPkt(seq int, receiveTimestamp time.Time) *ptp.DelayResp {
	l := binary.Size(ptp.DelayResp{})
	return &ptp.DelayResp{
		Header: ptp.Header{
			SdoIDAndMsgType:    ptp.NewSdoIDAndMsgType(ptp.MessageDelayResp, 0),
			Version:            ptp.Version,
			SequenceID:         uint16(seq),
			MessageLength:      uint16(l),
			FlagField:          ptp.FlagUnicast,
			LogMessageInterval: 0x7f,
		},
		DelayRespBody: ptp.DelayRespBody{
			ReceiveTimestamp: ptp.NewTimestamp(receiveTimestamp),
		},
	}
}

func newTestTester() *PTPTester {
	cfg := &PTPTestConfig{
		Timeout:   500 * time.Millisecond,
		Server:    "whatever",
		Interface: "ethblah",
	}
	lt := &PTPTester{
		inChan: make(chan *inPacket, 10),
		cfg:    cfg,
		sendTS: make(map[uint16]time.Time),
	}
	lt.generalAddr = &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 320,
	}
	lt.eventAddr = &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 319,
	}
	lt.localEventPort = 12345
	return lt
}

func TestLinearizabilityTestRunTest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	lt := newTestTester()
	lt.listenerRunning = true
	txTime := time.Unix(0, 1653574589806120900)
	rxTime := time.Unix(0, 1653574589806121900)
	genConn := NewMockUDPConn(ctrl)
	genConn.EXPECT().WriteTo(gomock.Any(), gomock.Any()).DoAndReturn(func(b []byte, _ net.Addr) (int, error) {
		r := bytes.NewReader(b)
		h := &ptp.Header{}
		err := binary.Read(r, binary.BigEndian, h)
		require.Nil(t, err, "reading header")
		require.Equal(t, ptp.MessageSignaling, h.SdoIDAndMsgType.MsgType(), "only expect signaling msgs over genConn")
		signaling := &ptp.Signaling{}
		err = ptp.FromBytes(b, signaling)
		require.Nil(t, err, "reading signaling msg")
		require.Equal(t, 1, len(signaling.TLVs), "expect only 1 TLV in signaling msg")
		tlv := signaling.TLVs[0]
		// we only expect SIGNALING messages where client asks for unicast grants.
		// for each such request we grant it.
		switch v := tlv.(type) {
		case *ptp.RequestUnicastTransmissionTLV:
			msgType := v.MsgTypeAndReserved.MsgType()
			switch msgType {
			case ptp.MessageDelayResp:
				grantDelayResp := grantUnicastPkt(2, lt.clockID, time.Second, ptp.MessageDelayResp)
				grantDelayRespBytes, err := ptp.Bytes(grantDelayResp)
				require.Nil(t, err)
				lt.inChan <- &inPacket{
					data: grantDelayRespBytes,
					ts:   time.Now(),
				}
				return 20, nil
			default:
				assert.Fail(t, fmt.Sprintf("got unexpected grant for %s", msgType))
			}
		case *ptp.CancelUnicastTransmissionTLV:
			return 0, nil
		default:
			assert.Fail(t, fmt.Sprintf("got unsupported TLV type %s(%d)", tlv.Type(), tlv.Type()))
		}
		return 10, nil
	})
	lt.gConn = genConn
	eventConn := NewMockUDPConnWithTS(ctrl)
	// handle whatever client is sending over eventConn
	eventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any()).DoAndReturn(func(b []byte, _ net.Addr) (int, time.Time, error) {
		delayReq := &ptp.SyncDelayReq{}
		err := ptp.FromBytes(b, delayReq)
		require.Nil(t, err, "reading delayReq msg")

		// don't respond to first delay request, as if we don't have subscription
		if delayReq.SequenceID == 0 {
			return 10, txTime, nil
		}

		delayResp := delayRespPkt(2, rxTime)
		delayRespBytes, err := ptp.Bytes(delayResp)
		require.Nil(t, err)
		lt.inChan <- &inPacket{
			data: delayRespBytes,
			ts:   time.Now(),
		}
		return 10, txTime, nil
	}).Times(2)
	lt.eConn = eventConn

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// this will run 'runSingleTest' without subscription request,
	// then retry with subscription request after no response is received
	result := lt.RunTest(ctx)
	want := PTPTestResult{
		Server:      lt.cfg.Server,
		Error:       nil,
		TXTimestamp: txTime,
		RXTimestamp: rxTime,
	}
	require.Equal(t, want, result)
}

func TestLinearizabilityTestRunTestTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	lt := newTestTester()
	lt.listenerRunning = true
	txTime := time.Unix(0, 1653574589806120900)
	// rxTime := time.Unix(0, 1653574589806121900)
	genConn := NewMockUDPConn(ctrl)
	genConn.EXPECT().WriteTo(gomock.Any(), gomock.Any())
	lt.gConn = genConn
	eventConn := NewMockUDPConnWithTS(ctrl)
	// handle whatever client is sending over eventConn
	eventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any()).DoAndReturn(func(b []byte, _ net.Addr) (int, time.Time, error) {
		delayReq := &ptp.SyncDelayReq{}
		err := ptp.FromBytes(b, delayReq)
		require.Nil(t, err, "reading delayReq msg")
		return 10, txTime, nil
	})
	lt.eConn = eventConn

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// this will run 'runSingleTest' without subscription request,
	// then retry with subscription request after no response is received
	result := lt.RunTest(ctx)
	require.ErrorIs(t, result.Err(), context.DeadlineExceeded)
	want := PTPTestResult{
		Server:      lt.cfg.Server,
		Error:       context.DeadlineExceeded,
		TXTimestamp: time.Time{},
		RXTimestamp: time.Time{},
	}
	require.Equal(t, want, result)
}

func TestLinearizabilityTestRunSingleTest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	lt := newTestTester()
	lt.listenerRunning = true
	txTime := time.Unix(0, 1653574589806120900)
	rxTime := time.Unix(0, 1653574589806121900)
	genConn := NewMockUDPConn(ctrl)
	genConn.EXPECT().WriteTo(gomock.Any(), gomock.Any()).DoAndReturn(func(b []byte, _ net.Addr) (int, error) {
		r := bytes.NewReader(b)
		h := &ptp.Header{}
		err := binary.Read(r, binary.BigEndian, h)
		require.Nil(t, err, "reading header")
		require.Equal(t, ptp.MessageSignaling, h.SdoIDAndMsgType.MsgType(), "only expect signaling msgs over genConn")
		signaling := &ptp.Signaling{}
		err = ptp.FromBytes(b, signaling)
		require.Nil(t, err, "reading signaling msg")
		require.Equal(t, 1, len(signaling.TLVs), "expect only 1 TLV in signaling msg")
		tlv := signaling.TLVs[0]
		// we only expect SIGNALING messages where client asks for unicast grants.
		// for each such request we grant it.
		switch v := tlv.(type) {
		case *ptp.RequestUnicastTransmissionTLV:
			msgType := v.MsgTypeAndReserved.MsgType()
			switch msgType {
			case ptp.MessageDelayResp:
				grantDelayResp := grantUnicastPkt(2, lt.clockID, time.Second, ptp.MessageDelayResp)
				grantDelayRespBytes, err := ptp.Bytes(grantDelayResp)
				require.Nil(t, err)
				lt.inChan <- &inPacket{
					data: grantDelayRespBytes,
					ts:   time.Now(),
				}
				return 20, nil
			default:
				assert.Fail(t, fmt.Sprintf("got unexpected grant for %s", msgType))
			}
		case *ptp.CancelUnicastTransmissionTLV:
			return 0, nil
		default:
			assert.Fail(t, fmt.Sprintf("got unsupported TLV type %s(%d)", tlv.Type(), tlv.Type()))
		}
		return 10, nil
	})
	lt.gConn = genConn
	eventConn := NewMockUDPConnWithTS(ctrl)
	// handle whatever client is sending over eventConn
	eventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any()).DoAndReturn(func(b []byte, _ net.Addr) (int, time.Time, error) {
		delayReq := &ptp.SyncDelayReq{}
		err := ptp.FromBytes(b, delayReq)
		require.Nil(t, err, "reading delayReq msg")

		delayResp := delayRespPkt(1, rxTime)
		delayRespBytes, err := ptp.Bytes(delayResp)
		require.Nil(t, err)
		lt.inChan <- &inPacket{
			data: delayRespBytes,
			ts:   time.Now(),
		}
		return 10, txTime, nil
	})
	lt.eConn = eventConn

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := lt.runSingleTest(ctx, time.Second)
	require.NoError(t, err)
	want := &PTPTestResult{
		Server:      lt.cfg.Server,
		Error:       nil,
		TXTimestamp: txTime,
		RXTimestamp: rxTime,
	}
	require.Equal(t, want, lt.result)
}

func TestLinearizabilityTestRunSingleTestSPTP(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	lt := newTestTester()
	lt.listenerRunning = true
	lt.proto = SPTP
	txTime := time.Unix(0, 1653574589806120900)
	rxTime := time.Unix(0, 1653574589806121900)
	genConn := NewMockUDPConn(ctrl)
	lt.gConn = genConn
	eventConn := NewMockUDPConnWithTS(ctrl)
	// handle whatever client is sending over eventConn
	eventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any()).DoAndReturn(func(b []byte, _ net.Addr) (int, time.Time, error) {
		delayReq := &ptp.SyncDelayReq{}
		err := ptp.FromBytes(b, delayReq)
		require.Nil(t, err, "reading delayReq msg")

		delayResp := delayRespPkt(0, rxTime)
		delayRespBytes, err := ptp.Bytes(delayResp)
		require.Nil(t, err)
		lt.inChan <- &inPacket{
			data: delayRespBytes,
			ts:   time.Now(),
		}
		return 10, txTime, nil
	})
	lt.eConn = eventConn

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := lt.runSingleTest(ctx, 0)
	require.NoError(t, err)
	want := &PTPTestResult{
		Server:      lt.cfg.Server,
		Error:       nil,
		TXTimestamp: txTime,
		RXTimestamp: rxTime,
	}
	require.Equal(t, want, lt.result)
}

func TestLinearizabilityTestRunSingleTestError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	lt := newTestTester()
	lt.listenerRunning = true
	genConn := NewMockUDPConn(ctrl)
	genConn.EXPECT().WriteTo(gomock.Any(), gomock.Any()).DoAndReturn(func(b []byte, _ net.Addr) (int, error) {
		r := bytes.NewReader(b)
		h := &ptp.Header{}
		err := binary.Read(r, binary.BigEndian, h)
		require.Nil(t, err, "reading header")
		require.Equal(t, ptp.MessageSignaling, h.SdoIDAndMsgType.MsgType(), "only expect signaling msgs over genConn")
		signaling := &ptp.Signaling{}
		err = ptp.FromBytes(b, signaling)
		require.Nil(t, err, "reading signaling msg")
		require.Equal(t, 1, len(signaling.TLVs), "expect only 1 TLV in signaling msg")
		tlv := signaling.TLVs[0]
		// we only expect SIGNALING messages where client asks for unicast grants.
		switch v := tlv.(type) {
		case *ptp.RequestUnicastTransmissionTLV:
			msgType := v.MsgTypeAndReserved.MsgType()
			switch msgType {
			case ptp.MessageDelayResp:
				// deny grant by setting duration to 0
				grantDelayResp := grantUnicastPkt(2, lt.clockID, 0, ptp.MessageDelayResp)
				grantDelayRespBytes, err := ptp.Bytes(grantDelayResp)
				require.Nil(t, err)
				lt.inChan <- &inPacket{
					data: grantDelayRespBytes,
					ts:   time.Now(),
				}
				return 20, nil
			default:
				assert.Fail(t, fmt.Sprintf("got unexpected grant for %s", msgType))
			}
		case *ptp.CancelUnicastTransmissionTLV:
			return 0, nil
		default:
			assert.Fail(t, fmt.Sprintf("got unsupported TLV type %s(%d)", tlv.Type(), tlv.Type()))
		}
		return 10, nil
	})
	lt.gConn = genConn
	eventConn := NewMockUDPConnWithTS(ctrl)
	lt.eConn = eventConn

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := lt.runSingleTest(ctx, time.Second)
	require.ErrorIs(t, err, ErrGrantDenied)
}

func TestLinearizabilityTestRunSingleTestNoSub(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	lt := newTestTester()
	lt.listenerRunning = true
	txTime := time.Unix(0, 1653574589806120900)
	rxTime := time.Unix(0, 1653574589806121900)
	genConn := NewMockUDPConn(ctrl)
	lt.gConn = genConn
	eventConn := NewMockUDPConnWithTS(ctrl)
	// handle whatever client is sending over eventConn
	eventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any()).DoAndReturn(func(b []byte, _ net.Addr) (int, time.Time, error) {
		delayReq := &ptp.SyncDelayReq{}
		err := ptp.FromBytes(b, delayReq)
		require.Nil(t, err, "reading delayReq msg")

		delayResp := delayRespPkt(0, rxTime)
		delayRespBytes, err := ptp.Bytes(delayResp)
		require.Nil(t, err)
		lt.inChan <- &inPacket{
			data: delayRespBytes,
			ts:   time.Now(),
		}
		return 10, txTime, nil
	})
	lt.eConn = eventConn

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := lt.runSingleTest(ctx, 0)
	require.NoError(t, err)

	want := &PTPTestResult{
		Server:      lt.cfg.Server,
		Error:       nil,
		TXTimestamp: txTime,
		RXTimestamp: rxTime,
	}
	require.Equal(t, lt.result, want)
}

func TestLinearizabilityTestRunSingleTestTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	lt := newTestTester()
	lt.listenerRunning = true

	genConn := NewMockUDPConn(ctrl)
	genConn.EXPECT().WriteTo(gomock.Any(), gomock.Any())
	lt.gConn = genConn

	eventConn := NewMockUDPConnWithTS(ctrl)
	lt.eConn = eventConn

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := lt.runSingleTest(ctx, time.Second)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestLinearizabilityTestRunSingleTestTimeoutNoSub(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	lt := newTestTester()
	lt.listenerRunning = true

	genConn := NewMockUDPConn(ctrl)
	lt.gConn = genConn

	eventConn := NewMockUDPConnWithTS(ctrl)
	eventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any())
	lt.eConn = eventConn

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := lt.runSingleTest(ctx, 0)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestTestResultGood(t *testing.T) {
	testCases := []struct {
		name    string
		in      PTPTestResult
		want    bool
		wantErr bool
	}{
		{
			name: "error",
			in: PTPTestResult{
				Server:      "time01",
				RXTimestamp: time.Time{},
				TXTimestamp: time.Time{},
				Error:       fmt.Errorf("test error"),
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "fail",
			in: PTPTestResult{
				Server:      "time01",
				RXTimestamp: time.Unix(0, 1647359186979431900),
				TXTimestamp: time.Unix(0, 1647359186979432900),
				Error:       nil,
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "pass",
			in: PTPTestResult{
				Server:      "time01",
				RXTimestamp: time.Unix(0, 1647359186979432900),
				TXTimestamp: time.Unix(0, 1647359186979431900),
				Error:       nil,
			},
			want:    true,
			wantErr: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.in.Good()
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got, "Good() for %+v must return %v", tc.in, tc.want)
			}
		})
	}
}

func TestTestResultExplain(t *testing.T) {
	testCases := []struct {
		name string
		in   PTPTestResult
		want string
	}{
		{
			name: "error",
			in: PTPTestResult{
				Server:      "time01",
				RXTimestamp: time.Time{},
				TXTimestamp: time.Time{},
				Error:       fmt.Errorf("test error"),
			},
			want: "linearizability test against \"time01\" couldn't be completed because of error: test error",
		},
		{
			name: "fail",
			in: PTPTestResult{
				Server:      "time01",
				RXTimestamp: time.Unix(0, 1647359186979431900),
				TXTimestamp: time.Unix(0, 1647359186979432900),
				Error:       nil,
			},
			want: fmt.Sprintf("linearizability test against \"time01\" failed because delta (-1µs) between RX and TX timestamps is not positive. TX=%v, RX=%v", time.Unix(0, 1647359186979432900), time.Unix(0, 1647359186979431900)),
		},
		{
			name: "pass",
			in: PTPTestResult{
				Server:      "time01",
				RXTimestamp: time.Unix(0, 1647359186979432900),
				TXTimestamp: time.Unix(0, 1647359186979431900),
				Error:       nil,
			},
			want: "linearizability test against \"time01\" passed",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.Explain()
			require.Equal(t, tc.want, got, "Explain() for %+v must return %v", tc.in, tc.want)
		})
	}
}

func TestProcessMonitoringResults(t *testing.T) {
	s := stats.NewStats()
	results := map[string]TestResult{
		"server01.nha1": PTPTestResult{ // tests pass - TX before RX
			Server:      "192.168.0.10",
			Error:       nil,
			TXTimestamp: time.Unix(0, 1653574589806127700),
			RXTimestamp: time.Unix(0, 1653574589806127800),
		},
		"server02.nha1": PTPTestResult{ // tests failed - TX after RX
			Server:      "192.168.0.11",
			Error:       nil,
			TXTimestamp: time.Unix(0, 1653574589806127730),
			RXTimestamp: time.Unix(0, 1653574589806127600),
		},
		"server03.nha1": PTPTestResult{ // drained server
			Server:      "192.168.0.12",
			Error:       ErrGrantDenied,
			TXTimestamp: time.Time{},
			RXTimestamp: time.Time{},
		},
		"server04.nha1": PTPTestResult{ // tests pass - TX before RX
			Server:      "192.168.0.13",
			Error:       nil,
			TXTimestamp: time.Unix(0, 1653574589806127900),
			RXTimestamp: time.Unix(0, 1653574589806127930),
		},
		"server05.nha1": PTPTestResult{ // failing server
			Server:      "192.168.0.14",
			Error:       fmt.Errorf("ooops"),
			TXTimestamp: time.Time{},
			RXTimestamp: time.Time{},
		},
	}

	ProcessMonitoringResults("ptp.linearizability.", results, s)

	c := s.Get()
	require.Equal(t, int64(3), c["ptp.linearizability.failed_tests"])
	require.Equal(t, int64(2), c["ptp.linearizability.passed_tests"])
	require.Equal(t, int64(5), c["ptp.linearizability.total_tests"])
}

func TestProcessTimestamp(t *testing.T) {
	server := "test_server"
	cfg := &PTPTestConfig{
		Server: server,
	}
	// Create a PTPTester instance
	tester := &PTPTester{
		cfg:    cfg,
		sendTS: make(map[uint16]time.Time),
	}
	// Define test cases
	now := time.Now()
	oneSecAgo := now.Add(-time.Second)
	tests := []struct {
		name           string
		sequenceID     uint16
		rxTimestamp    time.Time
		setupSendTS    func()
		expectedError  error
		expectedResult PTPTestResult
		expectedState  state
	}{
		{
			name:        "Valid sequence ID",
			sequenceID:  1,
			rxTimestamp: now,
			setupSendTS: func() {
				tester.sendTS = map[uint16]time.Time{1: oneSecAgo}
			},
			expectedError: nil,
			expectedState: stateDone,
			expectedResult: PTPTestResult{
				Server:      "test_server",
				RXTimestamp: now,
				TXTimestamp: oneSecAgo,
			},
		},
		{
			name:        "Invalid sequence ID",
			sequenceID:  2,
			rxTimestamp: now,
			setupSendTS: func() {
				tester.sendTS = map[uint16]time.Time{1: oneSecAgo}
			},
			expectedError: fmt.Errorf("unexpected sequence 2, expected one of [1]"),
			expectedState: tester.state, // state should remain unchanged
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup the sendTS map
			tt.setupSendTS()
			tester.state = stateInit
			// Call processTimestamp
			err := tester.processTimestamp(tt.sequenceID, tt.rxTimestamp)
			// Check the error
			if tt.expectedError != nil {
				require.Error(t, err)
				require.EqualError(t, err, tt.expectedError.Error())
			} else {
				require.NoError(t, err)
			}
			// Check the state
			require.Equal(t, tt.expectedState, tester.state)
			// Check the result if no error
			if err == nil {
				// assert state and result
				require.Equal(t, tt.expectedState, tester.state)
				require.Equal(t, tt.expectedResult, *tester.result)
			}
		})
	}
}
