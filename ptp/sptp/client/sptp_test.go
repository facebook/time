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
	"testing"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	gmstats "github.com/facebook/time/ptp/sptp/stats"
	"github.com/facebook/time/servo"

	"github.com/golang/mock/gomock"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestProcessResultsNoResults(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPHC := NewMockPHCIface(ctrl)
	mockStatsServer := NewMockStatsServer(ctrl)
	p := &SPTP{
		phc:   mockPHC,
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
	mockPHC := NewMockPHCIface(ctrl)
	mockServo := NewMockServo(ctrl)
	mockStatsServer := NewMockStatsServer(ctrl)
	p := &SPTP{
		phc:   mockPHC,
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
	mockPHC := NewMockPHCIface(ctrl)
	mockPHC.EXPECT().AdjFreqPPB(gomock.Any()).Return(nil)
	mockPHC.EXPECT().Step(gomock.Any()).Return(nil)
	mockServo := NewMockServo(ctrl)
	mockServo.EXPECT().Sample(int64(-200002000), gomock.Any()).Return(12.3, servo.StateJump)
	mockServo.EXPECT().Sample(int64(-100001000), gomock.Any()).Return(14.2, servo.StateLocked)
	mockStatsServer := NewMockStatsServer(ctrl)
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.gms.total", int64(1))
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.gms.available_pct", int64(100))
	mockStatsServer.EXPECT().SetGMStats(gomock.Any())
	p := &SPTP{
		phc:   mockPHC,
		pi:    mockServo,
		stats: mockStatsServer,
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
	mockStatsServer.EXPECT().SetGMStats(gomock.Any())
	p.processResults(results)
	require.Equal(t, "iamthebest", p.bestGM)
}

func TestProcessResultsMulti(t *testing.T) {
	ts, err := time.Parse(time.RFC3339, "2021-05-21T13:32:05+01:00")
	require.Nil(t, err)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPHC := NewMockPHCIface(ctrl)
	mockPHC.EXPECT().AdjFreqPPB(gomock.Any()).Return(nil)
	mockPHC.EXPECT().Step(gomock.Any()).Return(nil)
	mockServo := NewMockServo(ctrl)
	mockServo.EXPECT().Sample(int64(-200002000), gomock.Any()).Return(12.3, servo.StateJump)
	mockServo.EXPECT().Sample(int64(-104002000), gomock.Any()).Return(14.2, servo.StateLocked)
	mockStatsServer := NewMockStatsServer(ctrl)
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.gms.total", int64(2))
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.gms.available_pct", int64(50))
	mockStatsServer.EXPECT().SetGMStats(gomock.Any())
	mockStatsServer.EXPECT().SetGMStats(gomock.Any())

	p := &SPTP{
		phc:   mockPHC,
		pi:    mockServo,
		stats: mockStatsServer,
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
	mockPHC := NewMockPHCIface(ctrl)
	mockPHC.EXPECT().AdjFreqPPB((float64(0)))
	mockServo := NewMockServo(ctrl)
	mockServo.EXPECT().SyncInterval(float64(1))
	mockServo.EXPECT().MeanFreq()
	mockStatsServer := NewMockStatsServer(ctrl)
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.gms.total", int64(2)).Times(2)
	mockStatsServer.EXPECT().SetCounter("ptp.sptp.gms.available_pct", int64(0)).Times(2)
	mockStatsServer.EXPECT().UpdateCounterBy("ptp.sptp.portstats.tx.delay_req", int64(1)).Times(4)
	mockStatsServer.EXPECT().SetGMStats(&gmstats.Stat{GMAddress: "192.168.0.10", Error: context.DeadlineExceeded.Error(), Priority3: 1}).Times(2)
	mockStatsServer.EXPECT().SetGMStats(&gmstats.Stat{GMAddress: "192.168.0.11", Error: context.DeadlineExceeded.Error(), Priority3: 2}).Times(2)

	p := &SPTP{
		phc:   mockPHC,
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
	err = p.runInternal(ctx, time.Second)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
