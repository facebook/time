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
	"path/filepath"
	"testing"
	"time"

	"github.com/facebook/time/leapsectz/leaptest"
	"github.com/facebook/time/ptp/c4u/clock"
	"github.com/facebook/time/ptp/c4u/stats"
	"github.com/facebook/time/ptp/c4u/utcoffset"
	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/ptp4u/server"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	leaptest.Use(t, 27)

	expected := &server.DynamicConfig{
		ClockClass:     ptp.ClockClass6,
		ClockAccuracy:  ptp.ClockAccuracyNanosecond25,
		DrainInterval:  30 * time.Second,
		MaxSubDuration: 1 * time.Hour,
		MetricInterval: 1 * time.Minute,
		MinSubInterval: 1 * time.Second,
		UTCOffset:      37 * time.Second,
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
	leaptest.Use(t, 27)

	expected := &server.DynamicConfig{
		ClockClass:     ptp.ClockClass52,
		ClockAccuracy:  254,
		DrainInterval:  30 * time.Second,
		MaxSubDuration: 1 * time.Hour,
		MetricInterval: 1 * time.Minute,
		MinSubInterval: 1 * time.Second,
		UTCOffset:      37 * time.Second,
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

func runConfig(t *testing.T, path string) (*Config, *clock.RingBuffer) {
	t.Helper()
	c := &Config{
		Path:         path,
		Sample:       3,
		Apply:        true,
		AccuracyExpr: "1",
		ClassExpr:    "6",
	}
	rb := clock.NewRingBuffer(2)
	rb.Write(&clock.DataPoint{
		PHCOffset:            time.Microsecond,
		OscillatorOffset:     time.Microsecond,
		OscillatorClockClass: clock.ClockClassHoldover,
	})
	return c, rb
}

func writeDynamicConfig(t *testing.T, path string, utcOffset time.Duration) {
	t.Helper()
	onDisk := &server.DynamicConfig{
		ClockClass:     ptp.ClockClass52,
		ClockAccuracy:  254,
		DrainInterval:  30 * time.Second,
		MaxSubDuration: 1 * time.Hour,
		MetricInterval: 1 * time.Minute,
		MinSubInterval: 1 * time.Second,
		UTCOffset:      utcOffset,
	}
	require.NoError(t, onDisk.Write(path))
}

func TestRunKeepsUsableUTCOffsetWhenLeapDataIsUnusable(t *testing.T) {
	tests := []struct {
		name     string
		unusable func(testing.TB)
	}{
		{name: "no past leap second", unusable: leaptest.UseFutureOnly},
		{name: "unreadable leap table", unusable: leaptest.UseUnreadable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.unusable(t)
			_, err := utcoffset.Run()
			require.Error(t, err, "the leap data must be unusable or this test proves nothing")

			path := filepath.Join(t.TempDir(), "ptp4u.yaml")
			writeDynamicConfig(t, path, 37*time.Second)

			c, rb := runConfig(t, path)
			require.NoError(t, Run(c, rb, stats.NewJSONStats()))

			dc, err := server.ReadDynamicConfig(path)
			require.NoError(t, err, "ptp4u must still accept the config c4u wrote")
			require.Equal(t, 37*time.Second, dc.UTCOffset, "the last usable offset survives")
			require.Equal(t, ptp.ClockClass6, dc.ClockClass, "clock quality still gets through")
			require.Equal(t, ptp.ClockAccuracyNanosecond25, dc.ClockAccuracy)
		})
	}
}

func TestRunLeavesAnOutOfRangeConfigAloneWhenLeapDataIsUnusable(t *testing.T) {
	leaptest.UseFutureOnly(t)

	path := filepath.Join(t.TempDir(), "ptp4u.yaml")
	writeDynamicConfig(t, path, 10*time.Second)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	_, err = server.ReadDynamicConfig(path)
	require.Error(t, err, "the on-disk offset is out of range, so c4u falls back to defaultConfig")

	c, rb := runConfig(t, path)
	require.NoError(t, Run(c, rb, stats.NewJSONStats()))

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after), "c4u must not replace 10s with defaultConfig's 0s")
}

func TestRunWritesNoConfigWhenThereIsNoUsableUTCOffset(t *testing.T) {
	leaptest.UseFutureOnly(t)

	path := filepath.Join(t.TempDir(), "ptp4u.yaml")
	c, rb := runConfig(t, path)
	require.NoError(t, Run(c, rb, stats.NewJSONStats()))

	_, err := os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist, "c4u must not create a config ptp4u would refuse to load")
}

func TestRunResumesWritingOnceLeapDataIsUsableAgain(t *testing.T) {
	leaptest.UseFutureOnly(t)

	path := filepath.Join(t.TempDir(), "ptp4u.yaml")
	c, rb := runConfig(t, path)
	require.NoError(t, Run(c, rb, stats.NewJSONStats()))
	_, err := os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)

	leaptest.Use(t, 27)
	require.NoError(t, Run(c, rb, stats.NewJSONStats()))

	dc, err := server.ReadDynamicConfig(path)
	require.NoError(t, err, "refusing to write must not be a one-way door")
	require.Equal(t, 37*time.Second, dc.UTCOffset)
	require.Equal(t, ptp.ClockClass6, dc.ClockClass)
}

func TestRunRefreshesAStaleUsableUTCOffset(t *testing.T) {
	leaptest.Use(t, 27)

	path := filepath.Join(t.TempDir(), "ptp4u.yaml")
	writeDynamicConfig(t, path, 36*time.Second)

	c, rb := runConfig(t, path)
	require.NoError(t, Run(c, rb, stats.NewJSONStats()))

	dc, err := server.ReadDynamicConfig(path)
	require.NoError(t, err)
	require.Equal(t, 37*time.Second, dc.UTCOffset)
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
