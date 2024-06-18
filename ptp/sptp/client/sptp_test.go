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

package client

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	gmstats "github.com/facebook/time/ptp/sptp/stats"
	"github.com/facebook/time/servo"

	"github.com/golang/mock/gomock"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

const defaultTestTimeout = time.Second

func TestProcessResultsNoResults(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClock := NewMockClock(ctrl)
	mockServo := NewMockServo(ctrl)
	mockStatsServer := NewMockStatsServer(ctrl)
	p := &SPTP{
		clock: mockClock,
		pi:    mockServo,
		stats: mockStatsServer,
	}
	results := map[string]*RunResult{}
	mockServo.EXPECT().MeanFreq()
	mockServo.EXPECT().SetLastFreq(float64(0))
	mockClock.EXPECT().AdjFreqPPB(gomock.Any())
	mockStatsServer.EXPECT().SetGmsTotal(0)
	mockStatsServer.EXPECT().SetGmsAvailable(0)
	p.processResults(results)

	require.Equal(t, "", p.bestGM)
}

func TestProcessResultsEmptyResult(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClock := NewMockClock(ctrl)
	mockServo := NewMockServo(ctrl)
	mockStatsServer := NewMockStatsServer(ctrl)

	cfg := DefaultConfig()
	cfg.Servers = map[string]int{
		"192.168.0.10": 1,
	}
	p := &SPTP{
		clock: mockClock,
		pi:    mockServo,
		stats: mockStatsServer,
		cfg:   cfg,
	}
	err := p.initClients()
	require.NoError(t, err)
	results := map[string]*RunResult{
		"192.168.0.10": {},
	}
	meanFreq := 10.2
	mockServo.EXPECT().MeanFreq().Return(meanFreq)
	mockServo.EXPECT().SetLastFreq(float64(10.2))
	mockClock.EXPECT().AdjFreqPPB(-1 * meanFreq)
	mockStatsServer.EXPECT().SetGmsTotal(1)
	mockStatsServer.EXPECT().SetGmsAvailable(0)
	mockStatsServer.EXPECT().SetGMStats(gomock.Any())
	p.processResults(results)
	require.Equal(t, "", p.bestGM)
}

func TestProcessResultsSingle(t *testing.T) {
	ts, err := time.Parse(time.RFC3339, "2021-05-21T13:32:05+01:00")
	require.Nil(t, err)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClock := NewMockClock(ctrl)
	mockClock.EXPECT().AdjFreqPPB(gomock.Any()).Return(nil)
	mockClock.EXPECT().Step(gomock.Any()).Return(nil)
	mockClock.EXPECT().SetSync()
	mockServo := NewMockServo(ctrl)
	mockServo.EXPECT().IsSpike(int64(-200002000)).Return(false)
	mockServo.EXPECT().Sample(int64(-200002000), gomock.Any()).Return(12.3, servo.StateJump)
	mockServo.EXPECT().IsSpike(int64(-100001000)).Return(false)
	mockServo.EXPECT().Sample(int64(-100001000), gomock.Any()).Return(14.2, servo.StateLocked)
	mockStatsServer := NewMockStatsServer(ctrl)
	mockStatsServer.EXPECT().SetGmsTotal(1)
	mockStatsServer.EXPECT().SetGmsAvailable(100)
	mockStatsServer.EXPECT().SetGMStats(gomock.Any())

	cfg := DefaultConfig()
	cfg.Servers = map[string]int{
		"192.168.0.10": 1,
	}
	p := &SPTP{
		clock: mockClock,
		pi:    mockServo,
		stats: mockStatsServer,
		cfg:   cfg,
	}
	results := map[string]*RunResult{
		"192.168.0.10": {
			Server: "192.168.0.10",
			Measurement: &MeasurementResult{
				Delay:     299995 * time.Microsecond,
				S2CDelay:  100,
				C2SDelay:  110,
				Offset:    -200002 * time.Microsecond,
				Timestamp: ts,
			},
		},
	}
	err = p.initClients()
	require.NoError(t, err)
	// we step here
	p.processResults(results)
	require.Equal(t, "192.168.0.10", p.bestGM)

	// we have to wait a second to avoid "samples are too fast" error
	time.Sleep(time.Second)
	results["192.168.0.10"].Measurement.Offset = -100001 * time.Microsecond
	// we adj here
	mockStatsServer.EXPECT().SetGmsTotal(1)
	mockStatsServer.EXPECT().SetGmsAvailable(100)
	mockStatsServer.EXPECT().SetTickDuration(gomock.Any())
	mockStatsServer.EXPECT().SetGMStats(gomock.Any())
	p.processResults(results)
	require.Equal(t, "192.168.0.10", p.bestGM)
}

func TestProcessResultsFastSamples(t *testing.T) {
	ts, err := time.Parse(time.RFC3339, "2021-05-21T13:32:05+01:00")
	require.Nil(t, err)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClock := NewMockClock(ctrl)
	mockClock.EXPECT().AdjFreqPPB(gomock.Any()).Return(nil)
	mockClock.EXPECT().Step(gomock.Any()).Return(nil)
	mockServo := NewMockServo(ctrl)
	mockServo.EXPECT().IsSpike(int64(-200002000)).Return(false)
	mockServo.EXPECT().IsSpike(int64(-200002000)).Return(false)
	mockServo.EXPECT().Sample(int64(-200002000), gomock.Any()).Return(12.3, servo.StateJump)
	mockServo.EXPECT().MeanFreq().Return(12.3)
	mockServo.EXPECT().SetLastFreq(float64(12.3))
	mockStatsServer := NewMockStatsServer(ctrl)
	mockStatsServer.EXPECT().SetGmsTotal(1)
	mockStatsServer.EXPECT().SetGmsAvailable(100)
	mockStatsServer.EXPECT().SetGMStats(gomock.Any())

	cfg := DefaultConfig()
	cfg.Servers = map[string]int{
		"192.168.0.10": 1,
	}
	p := &SPTP{
		clock: mockClock,
		pi:    mockServo,
		stats: mockStatsServer,
		cfg:   cfg,
	}
	results := map[string]*RunResult{
		"192.168.0.10": {
			Server: "192.168.0.10",
			Measurement: &MeasurementResult{
				Delay:     299995 * time.Microsecond,
				S2CDelay:  100,
				C2SDelay:  110,
				Offset:    -200002 * time.Microsecond,
				Timestamp: ts,
			},
		},
	}
	err = p.initClients()
	require.NoError(t, err)
	// we step here
	p.processResults(results)
	require.Equal(t, "192.168.0.10", p.bestGM)

	// we stick to holdover mode and mean freq
	mockStatsServer.EXPECT().SetGmsTotal(1)
	mockStatsServer.EXPECT().SetGmsAvailable(100)
	mockStatsServer.EXPECT().SetTickDuration(gomock.Any())
	mockStatsServer.EXPECT().SetGMStats(gomock.Any())
	p.processResults(results)
	require.Equal(t, "192.168.0.10", p.bestGM)
}

func TestProcessResultsMulti(t *testing.T) {
	ts, err := time.Parse(time.RFC3339, "2021-05-21T13:32:05+01:00")
	require.Nil(t, err)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClock := NewMockClock(ctrl)
	mockClock.EXPECT().AdjFreqPPB(gomock.Any()).Return(nil)
	mockClock.EXPECT().Step(gomock.Any()).Return(nil)
	mockClock.EXPECT().SetSync()
	mockServo := NewMockServo(ctrl)
	mockServo.EXPECT().IsSpike(int64(-200002000)).Return(false)
	mockServo.EXPECT().Sample(int64(-200002000), gomock.Any()).Return(12.3, servo.StateJump)
	mockServo.EXPECT().IsSpike(int64(-104002000)).Return(false)
	mockServo.EXPECT().Sample(int64(-104002000), gomock.Any()).Return(14.2, servo.StateLocked)
	mockStatsServer := NewMockStatsServer(ctrl)
	mockStatsServer.EXPECT().SetGmsTotal(2)
	mockStatsServer.EXPECT().SetGmsAvailable(50)
	mockStatsServer.EXPECT().SetGMStats(gomock.Any())
	mockStatsServer.EXPECT().SetGMStats(gomock.Any())

	cfg := DefaultConfig()
	cfg.Servers = map[string]int{
		"192.168.0.10": 1,
		"192.168.0.11": 1,
	}
	p := &SPTP{
		clock: mockClock,
		pi:    mockServo,
		stats: mockStatsServer,
		cfg:   cfg,
	}
	err = p.initClients()
	require.NoError(t, err)
	announce0 := announcePkt(0)
	announce0.GrandmasterIdentity = ptp.ClockIdentity(0x001)
	announce0.GrandmasterPriority2 = 2
	announce1 := announcePkt(1)
	announce1.GrandmasterIdentity = ptp.ClockIdentity(0x042)
	announce1.GrandmasterPriority2 = 1
	results := map[string]*RunResult{
		"192.168.0.10": {
			Server: "192.168.0.10",
			Measurement: &MeasurementResult{
				Delay:     299995 * time.Microsecond,
				S2CDelay:  100,
				C2SDelay:  110,
				Offset:    -200002 * time.Microsecond,
				Timestamp: ts,
				Announce:  *announce0,
			},
		},
		"192.168.0.11": {
			Server: "192.168.0.11",
			Error:  fmt.Errorf("context deadline exceeded"),
		},
	}
	// we step here
	p.processResults(results)
	require.Equal(t, "192.168.0.10", p.bestGM)

	// we have to wait a second to avoid "samples are too fast" error
	time.Sleep(time.Second)

	results["192.168.0.10"].Measurement.Offset = -100001 * time.Microsecond
	results["192.168.0.11"].Error = nil
	results["192.168.0.11"].Measurement = &MeasurementResult{
		Delay:     299995 * time.Microsecond,
		S2CDelay:  90,
		C2SDelay:  120,
		Offset:    -104002 * time.Microsecond,
		Timestamp: ts,
		Announce:  *announce1,
	}
	// we adj here, while also switching to new best GM
	mockStatsServer.EXPECT().SetGmsTotal(2)
	mockStatsServer.EXPECT().SetGmsAvailable(100)
	mockStatsServer.EXPECT().SetTickDuration(gomock.Any())
	mockStatsServer.EXPECT().SetGMStats(gomock.Any())
	mockStatsServer.EXPECT().SetGMStats(gomock.Any())
	p.processResults(results)
	require.Equal(t, "192.168.0.11", p.bestGM)
}

func TestRunInternalAllDead(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEventConn := NewMockUDPConnWithTS(ctrl)
	mockEventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any()).Times(4)
	mockClock := NewMockClock(ctrl)
	mockClock.EXPECT().AdjFreqPPB((float64(0))).Times(3)
	mockServo := NewMockServo(ctrl)
	mockServo.EXPECT().SyncInterval(float64(1))
	mockServo.EXPECT().MeanFreq().Times(3)
	mockServo.EXPECT().SetLastFreq(float64(0)).Times(2)
	mockStatsServer := NewMockStatsServer(ctrl)
	mockStatsServer.EXPECT().SetGmsTotal(2).Times(2)
	mockStatsServer.EXPECT().SetGmsAvailable(0).Times(2)
	mockStatsServer.EXPECT().SetTickDuration(gomock.Any())
	mockStatsServer.EXPECT().IncTXDelayReq().Times(4)
	mockStatsServer.EXPECT().SetGMStats(&gmstats.Stat{GMAddress: "192.168.0.10", Error: context.DeadlineExceeded.Error(), Priority3: 1}).Times(2)
	mockStatsServer.EXPECT().SetGMStats(&gmstats.Stat{GMAddress: "192.168.0.11", Error: context.DeadlineExceeded.Error(), Priority3: 2}).Times(2)

	p := &SPTP{
		clock: mockClock,
		pi:    mockServo,
		stats: mockStatsServer,
		cfg: &Config{
			Interval: time.Second,
			Servers: map[string]int{
				"192.168.0.10": 1,
				"192.168.0.11": 2,
			},
			Measurement: MeasurementConfig{
				PathDelayFilterLength:         59,
				PathDelayFilter:               "median",
				PathDelayDiscardFilterEnabled: true,
				PathDelayDiscardBelow:         2 * time.Microsecond,
			},
		},
		eventConn: mockEventConn,
	}
	err := p.initClients()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	err = p.runInternal(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRunFiltered(t *testing.T) {
	ts, err := time.Parse(time.RFC3339, "2021-05-21T13:32:05+01:00")
	require.Nil(t, err)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClock := NewMockClock(ctrl)
	mockServo := NewMockServo(ctrl)
	mockServo.EXPECT().IsSpike(int64(-200002000)).Return(false)
	mockServo.EXPECT().Sample(int64(-200002000), gomock.Any()).Return(12.3, servo.StateFilter)
	mockStatsServer := NewMockStatsServer(ctrl)
	mockStatsServer.EXPECT().SetGmsTotal(1)
	mockStatsServer.EXPECT().SetGmsAvailable(100)
	mockStatsServer.EXPECT().SetGMStats(gomock.Any())

	cfg := DefaultConfig()
	cfg.Servers = map[string]int{
		"192.168.0.10": 1,
	}
	p := &SPTP{
		clock: mockClock,
		pi:    mockServo,
		stats: mockStatsServer,
		cfg:   cfg,
	}
	results := map[string]*RunResult{
		"192.168.0.10": {
			Server: "192.168.0.10",
			Measurement: &MeasurementResult{
				Delay:     299995 * time.Microsecond,
				S2CDelay:  100,
				C2SDelay:  110,
				Offset:    -200002 * time.Microsecond,
				Timestamp: ts,
			},
		},
	}
	err = p.initClients()
	require.NoError(t, err)
	// we step here
	p.processResults(results)
	require.Equal(t, "192.168.0.10", p.bestGM)
	require.Nil(t, nil, results["192.168.0.10"])
}

func TestRunListenerNoAddr(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEventConn := NewMockUDPConnWithTS(ctrl)
	mockEventConn.EXPECT().ReadPacketWithRXTimestampBuf(gomock.Any(), gomock.Any()).AnyTimes()
	mockGenConn := NewMockUDPConnNoTS(ctrl)
	mockGenConn.EXPECT().ReadPacketBuf(gomock.Any()).AnyTimes()
	mockClock := NewMockClock(ctrl)
	mockServo := NewMockServo(ctrl)
	mockStatsServer := NewMockStatsServer(ctrl)

	p := &SPTP{
		clock: mockClock,
		pi:    mockServo,
		stats: mockStatsServer,
		cfg: &Config{
			Interval: time.Second,
			Servers: map[string]int{
				"192.168.0.10": 1,
				"192.168.0.11": 2,
			},
		},
		eventConn: mockEventConn,
		genConn:   mockGenConn,
	}
	err := p.initClients()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer cancel()
	err = p.RunListener(ctx)
	require.EqualError(t, err, "received packet on port 320 with nil source address")
}

func TestRunListenerError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEventConn := NewMockUDPConnWithTS(ctrl)
	mockEventConn.EXPECT().ReadPacketWithRXTimestampBuf(gomock.Any(), gomock.Any()).Return(0, &unix.SockaddrInet6{}, time.Time{}, fmt.Errorf("some error")).AnyTimes()
	mockGenConn := NewMockUDPConnNoTS(ctrl)
	mockGenConn.EXPECT().ReadPacketBuf(gomock.Any()).Return(2, "", fmt.Errorf("some error")).AnyTimes()
	mockClock := NewMockClock(ctrl)
	mockServo := NewMockServo(ctrl)
	mockStatsServer := NewMockStatsServer(ctrl)

	p := &SPTP{
		clock: mockClock,
		pi:    mockServo,
		stats: mockStatsServer,
		cfg: &Config{
			Interval: time.Second,
			Servers: map[string]int{
				"192.168.0.10": 1,
				"192.168.0.11": 2,
			},
		},
		eventConn: mockEventConn,
		genConn:   mockGenConn,
	}
	err := p.initClients()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer cancel()
	err = p.RunListener(ctx)
	require.EqualError(t, err, "some error")
}

func TestRunListenerGood(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEventConn := NewMockUDPConnWithTS(ctrl)
	sentEvent := 0
	sentGen := 0
	syncBytes, _ := ptp.Bytes(&ptp.SyncDelayReq{})
	announceBytes, _ := ptp.Bytes(&ptp.Announce{})

	mockEventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any()).AnyTimes()
	mockEventConn.EXPECT().ReadPacketWithRXTimestampBuf(gomock.Any(), gomock.Any()).DoAndReturn(func(b, oob []byte) (int, unix.Sockaddr, time.Time, error) {
		// limit how many we send, so we don't overwhelm the client. packets from uknown IPs will be discarded
		if sentEvent > 10 {
			return 0, &unix.SockaddrInet4{}, time.Time{}, nil
		}
		addr := "192.168.0.11"
		if sentEvent%2 == 0 {
			addr = "192.168.0.10"
		}
		var addrBytes [4]byte
		copy(addrBytes[:], net.ParseIP(addr).To4())
		sentEvent++
		clear(b)
		b = syncBytes
		b[0] = 1
		b[1] = 2
		b[2] = 3
		b[3] = 4
		return len(syncBytes), &unix.SockaddrInet4{Addr: addrBytes, Port: 319}, time.Now(), nil
	}).AnyTimes()

	mockGenConn := NewMockUDPConnNoTS(ctrl)
	mockGenConn.EXPECT().ReadPacketBuf(gomock.Any()).DoAndReturn(func(b []byte) (int, string, error) {
		// limit how many we send, so we don't overwhelm the client. packets from uknown IPs will be discarded
		if sentGen > 10 {
			return 0, "whatever", nil
		}
		sentGen++
		addr := "192.168.0.11"
		if sentGen%2 == 0 {
			addr = "192.168.0.10"
		}
		clear(b)
		b = announceBytes
		b[0] = 9
		b[1] = 3
		b[2] = 7
		b[3] = 4
		return len(announceBytes), addr, nil
	}).AnyTimes()

	mockClock := NewMockClock(ctrl)
	mockServo := NewMockServo(ctrl)
	mockStatsServer := NewMockStatsServer(ctrl)
	mockStatsServer.EXPECT().IncRXAnnounce().Times(11)
	mockStatsServer.EXPECT().IncRXSync().Times(11)

	p := &SPTP{
		clock: mockClock,
		pi:    mockServo,
		stats: mockStatsServer,
		cfg: &Config{
			Interval: time.Second,
			Servers: map[string]int{
				"192.168.0.10": 1,
				"192.168.0.11": 2,
			},
		},
		eventConn: mockEventConn,
		genConn:   mockGenConn,
	}
	err := p.initClients()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer cancel()
	err = p.RunListener(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	received10 := 0
	received11 := 0
	nctx, ncancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer ncancel()
LOOP:
	for {
		select {
		case <-nctx.Done():
			break LOOP
		case <-p.clients["192.168.0.10"].inChan:
			received10++
		case <-p.clients["192.168.0.11"].inChan:
			received11++
		}
	}
	require.Equal(t, 11, sentGen)
	require.Equal(t, 11, sentEvent)
	require.Equal(t, sentGen/2+sentEvent/2+1, received10, "expect to receive N packets to client 192.168.0.10")
	require.Equal(t, sentGen/2+1+sentEvent/2, received11, "expect to receive N packets to client 192.168.0.11")
}

func TestPTPing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEventConn := NewMockUDPConnWithTS(ctrl)
	mockEventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any()).Times(2)
	mockClock := NewMockClock(ctrl)
	mockServo := NewMockServo(ctrl)
	mockStatsServer := NewMockStatsServer(ctrl)
	p := &SPTP{
		clock:     mockClock,
		pi:        mockServo,
		stats:     mockStatsServer,
		cfg:       &Config{},
		eventConn: mockEventConn,
	}

	response := []byte{}
	ip := net.ParseIP("1.2.3.4")

	err := p.ptping(ip, 1234, response, time.Time{})
	require.Equal(t, "failed to read delay request not enough data to decode SyncDelayReq", err.Error())

	b := &ptp.SyncDelayReq{}
	response, err = b.MarshalBinary()
	require.Nil(t, err)
	err = p.ptping(ip, 1234, response, time.Time{})
	require.Nil(t, err)
}

func TestShiftPriorities(t *testing.T) {
	p := &SPTP{
		priorities: map[string]int{
			"O": 1,
			"L": 2,
			"E": 3,
			"G": 4,
		},
	}

	p.reprioritize("L")
	require.Equal(t, 1, p.priorities["L"])
	require.Equal(t, 2, p.priorities["E"])
	require.Equal(t, 3, p.priorities["G"])
	require.Equal(t, 4, p.priorities["O"])

	p.reprioritize("O")
	require.Equal(t, 1, p.priorities["O"])
	require.Equal(t, 2, p.priorities["L"])
	require.Equal(t, 3, p.priorities["E"])
	require.Equal(t, 4, p.priorities["G"])

	// Shift by 0 (no change)
	p.reprioritize("O")
	require.Equal(t, 1, p.priorities["O"])
	require.Equal(t, 2, p.priorities["L"])
	require.Equal(t, 3, p.priorities["E"])
	require.Equal(t, 4, p.priorities["G"])
}
