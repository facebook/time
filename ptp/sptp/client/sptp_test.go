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

func TestProcessResultsNoResults(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClock := NewMockClock(ctrl)
	mockStatsServer := NewMockStatsServer(ctrl)
	p := &SPTP{
		clock: mockClock,
		stats: mockStatsServer,
	}
	results := map[string]*RunResult{}
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.gms.total", int64(0))
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.gms.available_pct", int64(0))
	p.processResults(results)

	require.Equal(t, "", p.bestGM)
}

func TestProcessResultsEmptyResult(t *testing.T) {
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
	results := map[string]*RunResult{
		"iamthebest": {},
	}
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.gms.total", int64(1))
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.gms.available_pct", int64(0))
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
	mockServo := NewMockServo(ctrl)
	mockServo.EXPECT().Sample(int64(-200002000), gomock.Any()).Return(12.3, servo.StateJump)
	mockServo.EXPECT().Sample(int64(-100001000), gomock.Any()).Return(14.2, servo.StateLocked)
	mockStatsServer := NewMockStatsServer(ctrl)
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.gms.total", int64(1))
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.gms.available_pct", int64(100))
	mockStatsServer.EXPECT().SetGMStats(gomock.Any())
	p := &SPTP{
		clock: mockClock,
		pi:    mockServo,
		stats: mockStatsServer,
		cfg:   DefaultConfig(),
	}
	results := map[string]*RunResult{
		"iamthebest": {
			Server: "iamthebest",
			Measurement: &MeasurementResult{
				Delay:              299995 * time.Microsecond,
				ServerToClientDiff: 100,
				ClientToServerDiff: 110,
				Offset:             -200002 * time.Microsecond,
				Timestamp:          ts,
			},
		},
	}
	// we step here
	p.processResults(results)
	require.Equal(t, "iamthebest", p.bestGM)

	results["iamthebest"].Measurement.Offset = -100001 * time.Microsecond
	// we adj here
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.gms.total", int64(1))
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.gms.available_pct", int64(100))
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.tick_duration_ns", gomock.Any())
	mockStatsServer.EXPECT().SetGMStats(gomock.Any())
	p.processResults(results)
	require.Equal(t, "iamthebest", p.bestGM)
}

func TestProcessResultsMulti(t *testing.T) {
	ts, err := time.Parse(time.RFC3339, "2021-05-21T13:32:05+01:00")
	require.Nil(t, err)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClock := NewMockClock(ctrl)
	mockClock.EXPECT().AdjFreqPPB(gomock.Any()).Return(nil)
	mockClock.EXPECT().Step(gomock.Any()).Return(nil)
	mockServo := NewMockServo(ctrl)
	mockServo.EXPECT().Sample(int64(-200002000), gomock.Any()).Return(12.3, servo.StateJump)
	mockServo.EXPECT().Sample(int64(-104002000), gomock.Any()).Return(14.2, servo.StateLocked)
	mockStatsServer := NewMockStatsServer(ctrl)
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.gms.total", int64(2))
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.gms.available_pct", int64(50))
	mockStatsServer.EXPECT().SetGMStats(gomock.Any())
	mockStatsServer.EXPECT().SetGMStats(gomock.Any())

	p := &SPTP{
		clock: mockClock,
		pi:    mockServo,
		stats: mockStatsServer,
		cfg:   DefaultConfig(),
	}
	announce0 := announcePkt(0)
	announce0.GrandmasterIdentity = ptp.ClockIdentity(0x001)
	announce0.GrandmasterPriority2 = 2
	announce1 := announcePkt(1)
	announce1.GrandmasterIdentity = ptp.ClockIdentity(0x042)
	announce1.GrandmasterPriority2 = 1
	results := map[string]*RunResult{
		"iamthebest": {
			Server: "iamthebest",
			Measurement: &MeasurementResult{
				Delay:              299995 * time.Microsecond,
				ServerToClientDiff: 100,
				ClientToServerDiff: 110,
				Offset:             -200002 * time.Microsecond,
				Timestamp:          ts,
				Announce:           *announce0,
			},
		},
		"soontobebest": {
			Server: "soontobebest",
			Error:  fmt.Errorf("context deadline exceeded"),
		},
	}
	// we step here
	p.processResults(results)
	require.Equal(t, "iamthebest", p.bestGM)

	results["iamthebest"].Measurement.Offset = -100001 * time.Microsecond
	results["soontobebest"].Error = nil
	results["soontobebest"].Measurement = &MeasurementResult{
		Delay:              299995 * time.Microsecond,
		ServerToClientDiff: 90,
		ClientToServerDiff: 120,
		Offset:             -104002 * time.Microsecond,
		Timestamp:          ts,
		Announce:           *announce1,
	}
	// we adj here, while also switching to new best GM
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.gms.total", int64(2))
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.gms.available_pct", int64(100))
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.tick_duration_ns", gomock.Any())
	mockStatsServer.EXPECT().SetGMStats(gomock.Any())
	mockStatsServer.EXPECT().SetGMStats(gomock.Any())
	p.processResults(results)
	require.Equal(t, "soontobebest", p.bestGM)
}

func TestRunInternalAllDead(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEventConn := NewMockUDPConnWithTS(ctrl)
	mockEventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any()).Times(4)
	mockClock := NewMockClock(ctrl)
	mockClock.EXPECT().AdjFreqPPB((float64(0)))
	mockServo := NewMockServo(ctrl)
	mockServo.EXPECT().SyncInterval(float64(1))
	mockServo.EXPECT().MeanFreq()
	mockStatsServer := NewMockStatsServer(ctrl)
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.gms.total", int64(2)).Times(2)
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.gms.available_pct", int64(0)).Times(2)
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.tick_duration_ns", gomock.Any())
	mockStatsServer.EXPECT().UpdateCounterBy("ptp.sptp.portstats.tx.delay_req", int64(1)).Times(4)
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

func TestRunListenerNoAddr(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEventConn := NewMockUDPConnWithTS(ctrl)
	mockEventConn.EXPECT().ReadPacketWithRXTimestamp().AnyTimes()
	mockGenConn := NewMockUDPConn(ctrl)
	mockGenConn.EXPECT().ReadFromUDP(gomock.Any()).AnyTimes()
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err = p.RunListener(ctx)
	require.EqualError(t, err, "received packet on port 320 with nil source address")
}

func TestRunListenerError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEventConn := NewMockUDPConnWithTS(ctrl)
	mockEventConn.EXPECT().ReadPacketWithRXTimestamp().Return([]byte{}, &unix.SockaddrInet6{}, time.Time{}, fmt.Errorf("some error")).AnyTimes()
	mockGenConn := NewMockUDPConn(ctrl)
	mockGenConn.EXPECT().ReadFromUDP(gomock.Any()).Return(2, nil, fmt.Errorf("some error")).AnyTimes()
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
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
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
	mockEventConn.EXPECT().ReadPacketWithRXTimestamp().DoAndReturn(func() ([]byte, unix.Sockaddr, time.Time, error) {
		// limit how many we send, so we don't overwhelm the client. packets from uknown IPs will be discarded
		if sentEvent > 10 {
			return nil, &unix.SockaddrInet4{}, time.Time{}, nil
		}
		addr := "192.168.0.11"
		if sentEvent%2 == 0 {
			addr = "192.168.0.10"
		}
		var addrBytes [4]byte
		copy(addrBytes[:], net.ParseIP(addr).To4())
		sentEvent++
		return []byte{1, 2, 3, 4}, &unix.SockaddrInet4{Addr: addrBytes, Port: 319}, time.Now(), nil
	}).AnyTimes()

	mockGenConn := NewMockUDPConn(ctrl)
	mockGenConn.EXPECT().ReadFromUDP(gomock.Any()).DoAndReturn(func(b []byte) (int, *net.UDPAddr, error) {
		// limit how many we send, so we don't overwhelm the client. packets from uknown IPs will be discarded
		if sentGen > 10 {
			return 0, &net.UDPAddr{}, nil
		}
		sentGen++
		addr := "192.168.0.11"
		if sentGen%2 == 0 {
			addr = "192.168.0.10"
		}
		udpAddr := &net.UDPAddr{
			IP:   net.ParseIP(addr),
			Port: 320,
		}
		b[0] = 9
		b[1] = 3
		b[2] = 7
		b[3] = 4

		return 4, udpAddr, nil
	}).AnyTimes()

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
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err = p.RunListener(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	receivedEvent10 := 0
	receivedGen10 := 0
	receivedEvent11 := 0
	receivedGen11 := 0
	nctx, ncancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer ncancel()
LOOP:
	for {
		select {
		case <-nctx.Done():
			break LOOP
		case p := <-p.clients["192.168.0.10"].inChan:
			if p.data[0] == 1 {
				receivedEvent10++
			} else if p.data[0] == 9 {
				receivedGen10++
			}
		case p := <-p.clients["192.168.0.11"].inChan:
			if p.data[0] == 1 {
				receivedEvent11++
			} else if p.data[0] == 9 {
				receivedGen11++
			}
		}
	}
	require.Equal(t, sentGen/2, receivedGen10, "expect to receive N general packets to client 192.168.0.10")
	require.Equal(t, sentEvent/2+1, receivedEvent10, "expect to receive N event packets to client 192.168.0.10")
	require.Equal(t, sentGen/2+1, receivedGen11, "expect to receive N general packets to client 192.168.0.11")
	require.Equal(t, sentEvent/2, receivedEvent11, "expect to receive N event packets to client 192.168.0.11")
}
