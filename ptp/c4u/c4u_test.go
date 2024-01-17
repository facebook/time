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

package c4u

import (
	"os"
	"testing"
	"time"

	"github.com/facebook/time/ptp/c4u/clock"
	"github.com/facebook/time/ptp/c4u/stats"
	"github.com/facebook/time/ptp/c4u/utcoffset"
	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/ptp4u/server"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	// We don't really care about UTCOffset here - just to be the same result as in c4u.Run()
	utcoffset, _ := utcoffset.Run()

	expected := &server.DynamicConfig{
		ClockClass:     ptp.ClockClass6,
		ClockAccuracy:  ptp.ClockAccuracyNanosecond25,
		DrainInterval:  30 * time.Second,
		MaxSubDuration: 1 * time.Hour,
		MetricInterval: 1 * time.Minute,
		MinSubInterval: 1 * time.Second,
		UTCOffset:      utcoffset,
	}

	cfg, err := os.CreateTemp("", "c4u")
	require.NoError(t, err)
	defer os.Remove(cfg.Name())

	c := &Config{
		Path:         cfg.Name(),
		Sample:       3,
		Apply:        true,
		AccuracyExpr: "1",
		ClassExpr:    "6",
	}

	st := stats.NewJSONStats()
	rb := clock.NewRingBuffer(2)
	dp := &clock.DataPoint{
		PHCOffset:            time.Microsecond,
		OscillatorOffset:     time.Microsecond,
		OscillatorClockClass: clock.ClockClassHoldover,
	}
	rb.Write(dp)
	err = Run(c, rb, st)
	require.NoError(t, err)

	dc, err := server.ReadDynamicConfig(c.Path)
	require.NoError(t, err)
	require.Equal(t, expected, dc)
}

func TestRunNilDatapoint(t *testing.T) {
	// We don't really care about UTCOffset here - just to be the same result as in c4u.Run()
	utcoffset, _ := utcoffset.Run()

	expected := &server.DynamicConfig{
		ClockClass:     ptp.ClockClass52,
		ClockAccuracy:  254,
		DrainInterval:  30 * time.Second,
		MaxSubDuration: 1 * time.Hour,
		MetricInterval: 1 * time.Minute,
		MinSubInterval: 1 * time.Second,
		UTCOffset:      utcoffset,
	}

	cfg, err := os.CreateTemp("", "c4u")
	require.NoError(t, err)
	defer os.Remove(cfg.Name())

	c := &Config{
		Path:         cfg.Name(),
		Sample:       3,
		Apply:        true,
		AccuracyExpr: "1",
		ClassExpr:    "p99(oscillatorclass)",
	}

	st := stats.NewJSONStats()
	rb := clock.NewRingBuffer(2)
	dp := &clock.DataPoint{
		PHCOffset:            time.Microsecond,
		OscillatorOffset:     time.Microsecond,
		OscillatorClockClass: clock.ClockClassHoldover,
	}
	rb.Write(dp)
	err = Run(c, rb, st)
	require.NoError(t, err)

	dc, err := server.ReadDynamicConfig(c.Path)
	require.NoError(t, err)
	// must make sure nil entry results in ClockClass = 52
	require.Equal(t, expected, dc)
}

func TestEvaluateClockQuality(t *testing.T) {
	c := &Config{
		LockBaseLine:        ptp.ClockAccuracyMicrosecond1,
		HoldoverBaseLine:    ptp.ClockAccuracyMicrosecond2point5,
		CalibratingBaseLine: ptp.ClockAccuracyMicrosecond25,
	}

	expected := &ptp.ClockQuality{ClockClass: clock.ClockClassUncalibrated, ClockAccuracy: ptp.ClockAccuracyUnknown}
	q := evaluateClockQuality(c, nil)
	require.Equal(t, expected, q)

	// Lock
	expected = &ptp.ClockQuality{ClockClass: clock.ClockClassLock, ClockAccuracy: ptp.ClockAccuracyMicrosecond1}
	q = evaluateClockQuality(c, &ptp.ClockQuality{ClockClass: clock.ClockClassLock, ClockAccuracy: ptp.ClockAccuracyNanosecond100})
	require.Equal(t, expected, q)

	expected = &ptp.ClockQuality{ClockClass: clock.ClockClassLock, ClockAccuracy: ptp.ClockAccuracyMicrosecond2point5}
	q = evaluateClockQuality(c, &ptp.ClockQuality{ClockClass: clock.ClockClassLock, ClockAccuracy: ptp.ClockAccuracyMicrosecond2point5})
	require.Equal(t, expected, q)

	// Holdover
	expected = &ptp.ClockQuality{ClockClass: clock.ClockClassHoldover, ClockAccuracy: ptp.ClockAccuracyMicrosecond2point5}
	q = evaluateClockQuality(c, &ptp.ClockQuality{ClockClass: clock.ClockClassHoldover, ClockAccuracy: ptp.ClockAccuracyNanosecond250})
	require.Equal(t, expected, q)

	expected = &ptp.ClockQuality{ClockClass: clock.ClockClassHoldover, ClockAccuracy: ptp.ClockAccuracyMicrosecond25}
	q = evaluateClockQuality(c, &ptp.ClockQuality{ClockClass: clock.ClockClassHoldover, ClockAccuracy: ptp.ClockAccuracyMicrosecond25})
	require.Equal(t, expected, q)

	// Calibrating
	expected = &ptp.ClockQuality{ClockClass: clock.ClockClassCalibrating, ClockAccuracy: ptp.ClockAccuracyMicrosecond25}
	q = evaluateClockQuality(c, &ptp.ClockQuality{ClockClass: clock.ClockClassCalibrating, ClockAccuracy: ptp.ClockAccuracyMicrosecond1})
	require.Equal(t, expected, q)

	expected = &ptp.ClockQuality{ClockClass: clock.ClockClassCalibrating, ClockAccuracy: ptp.ClockAccuracyMicrosecond100}
	q = evaluateClockQuality(c, &ptp.ClockQuality{ClockClass: clock.ClockClassCalibrating, ClockAccuracy: ptp.ClockAccuracyMicrosecond100})
	require.Equal(t, expected, q)

	// Uncalibrated
	expected = &ptp.ClockQuality{ClockClass: clock.ClockClassUncalibrated, ClockAccuracy: ptp.ClockAccuracyUnknown}
	q = evaluateClockQuality(c, &ptp.ClockQuality{ClockClass: clock.ClockClassUncalibrated, ClockAccuracy: ptp.ClockAccuracyNanosecond25})
	require.Equal(t, expected, q)
}
