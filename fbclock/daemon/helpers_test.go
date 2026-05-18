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

package daemon

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/facebook/time/fbclock"
	"github.com/facebook/time/leapsectz"
	"github.com/stretchr/testify/require"
)

func TestMinRingSize(t *testing.T) {
	testCases := []struct {
		name     string
		ringSize int
		interval time.Duration
		want     int
	}{
		{
			name:     "ring size already covers 1 minute",
			ringSize: 60,
			interval: time.Second,
			want:     60,
		},
		{
			name:     "ring size larger than needed",
			ringSize: 120,
			interval: time.Second,
			want:     120,
		},
		{
			name:     "ring size too small for 1 minute coverage",
			ringSize: 10,
			interval: time.Second,
			want:     60,
		},
		{
			name:     "fast interval needs more samples",
			ringSize: 30,
			interval: 100 * time.Millisecond,
			want:     600,
		},
		{
			name:     "slow interval needs fewer samples",
			ringSize: 5,
			interval: 30 * time.Second,
			want:     5,
		},
		{
			name:     "exact boundary",
			ringSize: 30,
			interval: 2 * time.Second,
			want:     30,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := minRingSize(tc.ringSize, tc.interval)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestPrepareMathParameters(t *testing.T) {
	dataPoints := []*DataPoint{
		{MasterOffsetNS: 10.0, PathDelayNS: 100.0, FreqAdjustmentPPB: 1000.0, ClockAccuracyNS: 25.0},
		{MasterOffsetNS: 20.0, PathDelayNS: 200.0, FreqAdjustmentPPB: 1500.0, ClockAccuracyNS: 50.0},
		{MasterOffsetNS: 30.0, PathDelayNS: 300.0, FreqAdjustmentPPB: 1200.0, ClockAccuracyNS: 75.0},
	}

	got := prepareMathParameters(dataPoints)
	require.Equal(t, []float64{10.0, 20.0, 30.0}, got["offset"])
	require.Equal(t, []float64{100.0, 200.0, 300.0}, got["delay"])
	require.Equal(t, []float64{1000.0, 1500.0, 1200.0}, got["freq"])
	require.Equal(t, []float64{25.0, 50.0, 75.0}, got["clockaccuracy"])
	require.Equal(t, []float64{500.0, -300.0}, got["freqchange"])
	require.Equal(t, []float64{500.0, 300.0}, got["freqchangeabs"])
}

func TestPrepareMathParametersSinglePoint(t *testing.T) {
	dataPoints := []*DataPoint{
		{MasterOffsetNS: 42.0, PathDelayNS: 123.0, FreqAdjustmentPPB: 999.0, ClockAccuracyNS: 10.0},
	}

	got := prepareMathParameters(dataPoints)
	require.Equal(t, []float64{42.0}, got["offset"])
	require.Equal(t, []float64{123.0}, got["delay"])
	require.Equal(t, []float64{999.0}, got["freq"])
	require.Equal(t, []float64{10.0}, got["clockaccuracy"])
	require.Empty(t, got["freqchange"])
	require.Empty(t, got["freqchangeabs"])
}

func TestMapOfInterface(t *testing.T) {
	input := map[string][]float64{
		"a": {1.0, 2.0},
		"b": {3.0},
	}
	got := mapOfInterface(input)
	require.Len(t, got, 2)
	require.Equal(t, []float64{1.0, 2.0}, got["a"])
	require.Equal(t, []float64{3.0}, got["b"])
}

func TestIsSupportedVar(t *testing.T) {
	require.True(t, isSupportedVar("offset"))
	require.True(t, isSupportedVar("delay"))
	require.True(t, isSupportedVar("freq"))
	require.True(t, isSupportedVar("m"))
	require.True(t, isSupportedVar("clockaccuracy"))
	require.True(t, isSupportedVar("freqchange"))
	require.True(t, isSupportedVar("freqchangeabs"))
	require.False(t, isSupportedVar("missing"))
	require.False(t, isSupportedVar(""))
}

func TestStddev(t *testing.T) {
	// welford uses sample stddev (N-1 denominator)
	input := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	got := stddev(input)
	require.InDelta(t, 2.138, got, 0.01)

	input = []float64{5, 5, 5, 5}
	got = stddev(input)
	require.Equal(t, 0.0, got)
}

func TestCalcCoeffPPBZeroPrevSysclock(t *testing.T) {
	prev := &fbclock.DataV2{SysclockTimeNS: 0}
	cur := &fbclock.DataV2{SysclockTimeNS: 100, PHCTimeNS: 100}
	c, err := calcCoeffPPB(prev, cur)
	require.Equal(t, int64(0), c)
	require.NoError(t, err)
}

func TestCalcCoeffPPBIdenticalClocks(t *testing.T) {
	prev := &fbclock.DataV2{SysclockTimeNS: 1000000000, PHCTimeNS: 2000000000}
	cur := &fbclock.DataV2{SysclockTimeNS: 1010000000, PHCTimeNS: 2010000000}
	c, err := calcCoeffPPB(prev, cur)
	require.Equal(t, int64(0), c)
	require.NoError(t, err)
}

func TestCalcCoeffPPBFasterPHC(t *testing.T) {
	prev := &fbclock.DataV2{SysclockTimeNS: 1000000000, PHCTimeNS: 2000000000}
	cur := &fbclock.DataV2{SysclockTimeNS: 2000000000, PHCTimeNS: 3000001000}
	c, err := calcCoeffPPB(prev, cur)
	// float precision: result is 999 or 1000 depending on rounding
	require.InDelta(t, int64(1000), c, 1)
	require.NoError(t, err)
}

func TestCalcCoeffPPBSlowerPHC(t *testing.T) {
	prev := &fbclock.DataV2{SysclockTimeNS: 1749167822494826022, PHCTimeNS: 1749167859494830869}
	cur := &fbclock.DataV2{SysclockTimeNS: 1749167822504951677, PHCTimeNS: 1749167859504956519}
	c, err := calcCoeffPPB(prev, cur)
	require.Equal(t, int64(-493), c)
	require.NoError(t, err)
}

func TestDummyLoggerLog(t *testing.T) {
	b := &bytes.Buffer{}
	l := NewDummyLogger(b, 1)
	s := &LogSample{
		MeasurementNS: 42.5,
		WindowNS:      100.3,
	}
	err := l.Log(s)
	require.NoError(t, err)
	require.Contains(t, b.String(), "m = 42ns")
	require.Contains(t, b.String(), "w = 100ns")
}

func TestDummyLoggerLogSampling(t *testing.T) {
	b := &bytes.Buffer{}
	l := NewDummyLogger(b, 0)
	s := &LogSample{MeasurementNS: 1.0, WindowNS: 2.0}
	err := l.Log(s)
	require.NoError(t, err)
	require.Empty(t, b.String())
}

func TestReadConfig(t *testing.T) {
	yamlContent := `ptpclientaddress: "/var/run/ptp4l"
ringsize: 30
interval: 1s
iface: eth0
linearizabilitytestinterval: 10s
sptp: true
bootdelay: 5s
enabledatav2: true
math:
  m: "mean(clockaccuracy, 30) + abs(mean(offset, 30))"
  w: "mean(m, 30) + 4.0 * stddev(m, 30)"
  drift: "1.5 * mean(freqchangeabs, 29)"
`
	tmpFile, err := os.CreateTemp("", "config_test_*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(yamlContent)
	require.NoError(t, err)
	tmpFile.Close()

	cfg, err := ReadConfig(tmpFile.Name())
	require.NoError(t, err)
	require.Equal(t, "/var/run/ptp4l", cfg.PTPClientAddress)
	require.Equal(t, 30, cfg.RingSize)
	require.Equal(t, time.Second, cfg.Interval)
	require.Equal(t, "eth0", cfg.Iface)
	require.Equal(t, 10*time.Second, cfg.LinearizabilityTestInterval)
	require.True(t, cfg.SPTP)
	require.Equal(t, 5*time.Second, cfg.BootDelay)
	require.True(t, cfg.EnableDataV2)
	require.Equal(t, "mean(clockaccuracy, 30) + abs(mean(offset, 30))", cfg.Math.M)
	require.Equal(t, "mean(m, 30) + 4.0 * stddev(m, 30)", cfg.Math.W)
	require.Equal(t, "1.5 * mean(freqchangeabs, 29)", cfg.Math.Drift)
}

func TestReadConfigMissing(t *testing.T) {
	_, err := ReadConfig("/nonexistent/path/config.yaml")
	require.Error(t, err)
}

func TestReadConfigInvalid(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "config_test_*.yaml")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString("not: valid: yaml: content: [[[")
	require.NoError(t, err)
	tmpFile.Close()

	_, err = ReadConfig(tmpFile.Name())
	require.Error(t, err)
}

func TestEvalAndValidateEdgeCases(t *testing.T) {
	testCases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "empty ptpclientaddress",
			cfg:     Config{PTPClientAddress: "", RingSize: 30, Interval: time.Second},
			wantErr: true,
		},
		{
			name:    "zero ringsize",
			cfg:     Config{PTPClientAddress: "/var/run/ptp4l", RingSize: 0, Interval: time.Second},
			wantErr: true,
		},
		{
			name:    "negative ringsize",
			cfg:     Config{PTPClientAddress: "/var/run/ptp4l", RingSize: -1, Interval: time.Second},
			wantErr: true,
		},
		{
			name:    "zero interval",
			cfg:     Config{PTPClientAddress: "/var/run/ptp4l", RingSize: 30, Interval: 0},
			wantErr: true,
		},
		{
			name:    "interval too large",
			cfg:     Config{PTPClientAddress: "/var/run/ptp4l", RingSize: 30, Interval: 2 * time.Minute},
			wantErr: true,
		},
		{
			name:    "negative linearizability interval",
			cfg:     Config{PTPClientAddress: "/var/run/ptp4l", RingSize: 30, Interval: time.Second, LinearizabilityTestInterval: -time.Second},
			wantErr: true,
		},
		{
			name:    "negative max gm offset",
			cfg:     Config{PTPClientAddress: "/var/run/ptp4l", RingSize: 30, Interval: time.Second, LinearizabilityTestMaxGMOffset: -time.Second},
			wantErr: true,
		},
		{
			name: "valid config",
			cfg: Config{
				PTPClientAddress: "/var/run/ptp4l",
				RingSize:         30,
				Interval:         time.Second,
				Math: Math{
					M:     "mean(offset, 30)",
					W:     "mean(m, 30)",
					Drift: "mean(freqchangeabs, 29)",
				},
			},
			wantErr: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.EvalAndValidate()
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConvolveNotEnoughValues(t *testing.T) {
	input := []float64{1.0, 2.0}
	coeffs := []float64{0.5, 0.5, 0.5}
	_, err := convolve(input, coeffs)
	require.Error(t, err)
}

func TestConvolveExactSize(t *testing.T) {
	input := []float64{1.0, 2.0, 3.0}
	coeffs := []float64{1.0, 0.0, 0.0}
	got, err := convolve(input, coeffs)
	require.NoError(t, err)
	require.Len(t, got, 3)
}

func TestLeapSecondSmearingEmpty(t *testing.T) {
	got := leapSecondSmearing(nil)
	require.Equal(t, &clockSmearing{}, got)
}

func TestLeapSecondSmearingSingle(t *testing.T) {
	leaps := []leapsectz.LeapSecond{
		{Tleap: 1483228826, Nleap: 26},
	}
	got := leapSecondSmearing(leaps)
	require.Equal(t, &clockSmearing{}, got)
}
