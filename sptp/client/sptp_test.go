package client

import (
	"fmt"
	"testing"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
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
	p := &SPTP{
		phc: mockPHC,
	}
	results := map[string]*RunResult{}
	p.processResults(results)
	require.Equal(t, "", p.bestGM)
}

func TestProcessResultsEmptyResult(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockPHC := NewMockPHCIface(ctrl)
	mockServo := NewMockServo(ctrl)
	p := &SPTP{
		phc:   mockPHC,
		pi:    mockServo,
		stats: NewStats(),
	}
	results := map[string]*RunResult{
		"iamthebest": {},
	}
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
	mockServo.EXPECT().Sample(int64(-200002000), gomock.Any()).Return(12.3, servo.ServoJump)
	mockServo.EXPECT().Sample(int64(-100001000), gomock.Any()).Return(14.2, servo.ServoLocked)
	p := &SPTP{
		phc:   mockPHC,
		pi:    mockServo,
		stats: NewStats(),
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
	mockServo.EXPECT().Sample(int64(-200002000), gomock.Any()).Return(12.3, servo.ServoJump)
	mockServo.EXPECT().Sample(int64(-104002000), gomock.Any()).Return(14.2, servo.ServoLocked)
	p := &SPTP{
		phc:   mockPHC,
		pi:    mockServo,
		stats: NewStats(),
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
	p.processResults(results)
	require.Equal(t, "soontobebest", p.bestGM)
}
