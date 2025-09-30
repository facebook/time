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
	"context"
	"os"
	"testing"
	"time"

	"github.com/facebook/time/fbclock"
	"github.com/facebook/time/fbclock/stats"
	"github.com/facebook/time/leapsectz"
	"github.com/facebook/time/ptp/linearizability"
	"github.com/stretchr/testify/require"

	ptp "github.com/facebook/time/ptp/protocol"
)

type testLogger struct {
	samples []*LogSample
}

func (l *testLogger) Log(s *LogSample) error {
	l.samples = append(l.samples, s)
	return nil
}

func newTestDaemon(cfg *Config, stats stats.Server) *Daemon {
	s := &Daemon{
		stats:       stats,
		cfg:         cfg,
		state:       newDaemonState(cfg.RingSize),
		l:           &testLogger{samples: []*LogSample{}},
		DataFetcher: &SockFetcher{},
	}
	return s
}

func TestDaemonStateLinearizabilityRing(t *testing.T) {
	s := newDaemonState(3)

	probes := []linearizability.TestResult{
		linearizability.PTPTestResult{
			Server:      "server01",
			TXTimestamp: time.Unix(0, 1647359186979431100),
			RXTimestamp: time.Unix(0, 1647359186979431635),
		},
		linearizability.PTPTestResult{
			Server:      "server02",
			TXTimestamp: time.Unix(0, 1647359186979431200),
			RXTimestamp: time.Unix(0, 1647359186979431735),
		},
		linearizability.PTPTestResult{
			Server:      "server01",
			TXTimestamp: time.Unix(0, 1647359186979431300),
			RXTimestamp: time.Unix(0, 1647359186979431835),
		},
	}

	for _, tr := range probes {
		s.pushLinearizabilityTestResult(tr)
	}
	got := s.takeLinearizabilityTestResult(3)
	require.ElementsMatch(t, probes, got)
}

func TestDaemonStateAggregateMax(t *testing.T) {
	s := newDaemonState(3)

	probes := []*DataPoint{
		{
			MasterOffsetNS:    123.0,
			PathDelayNS:       3,
			FreqAdjustmentPPB: 4,
		},
		{
			MasterOffsetNS:    -2000.0,
			PathDelayNS:       300,
			FreqAdjustmentPPB: 2,
		},
		{
			MasterOffsetNS:    1009.0,
			PathDelayNS:       200,
			FreqAdjustmentPPB: 5,
		},
	}

	for _, tr := range probes {
		s.pushDataPoint(tr)
	}
	got := s.aggregateDataPointsMax(3)
	want := &DataPoint{
		MasterOffsetNS:    2000.0,
		PathDelayNS:       300,
		FreqAdjustmentPPB: 5,
	}
	require.Equal(t, want, got)
}

func TestTargetsChange(t *testing.T) {
	testCases := []struct {
		name        string
		oldTargets  []string
		targets     []string
		wantAdded   []string
		wantRemoved []string
	}{
		{
			name: "no new targets",
			oldTargets: []string{
				"server1",
				"server2",
			},
			targets:   []string{},
			wantAdded: []string{},
			wantRemoved: []string{
				"server1",
				"server2",
			},
		},
		{
			name:       "no old targets",
			oldTargets: []string{},
			targets:    []string{"server1", "server2"},
			wantAdded: []string{
				"server1",
				"server2",
			},
			wantRemoved: []string{},
		},
		{
			name: "removed old tester",
			oldTargets: []string{
				"server1",
				"server3",
				"server2",
			},
			targets:     []string{"server1", "server2"},
			wantAdded:   []string{},
			wantRemoved: []string{"server3"},
		},
		{
			name: "added target",
			oldTargets: []string{
				"server1",
				"server2",
			},
			targets:     []string{"server1", "server2", "server3"},
			wantAdded:   []string{"server3"},
			wantRemoved: []string{},
		},
		{
			name: "equal",
			oldTargets: []string{
				"server2",
				"server1",
			},
			targets:     []string{"server1", "server2"},
			wantAdded:   []string{},
			wantRemoved: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotAdded, gotRemoved := targetsDiff(tc.oldTargets, tc.targets)
			require.ElementsMatch(t, tc.wantAdded, gotAdded, "added")
			require.ElementsMatch(t, tc.wantRemoved, gotRemoved, "removed")
		})
	}
}

func TestMathPrepare(t *testing.T) {
	testCases := []struct {
		name    string
		in      *Math
		wantErr bool
	}{
		{
			name: "basic math",
			in: &Math{
				M:     "1 + 3",
				W:     "4 / 1",
				Drift: "1 - 0",
			},
			wantErr: false,
		},
		{
			name: "unknown function",
			in: &Math{
				M:     "1 + 3",
				W:     "missing(m)",
				Drift: "1 - 0",
			},
			wantErr: true,
		},
		{
			name: "known function",
			in: &Math{
				M:     "1 + 3",
				W:     "mean(m)",
				Drift: "1 - 0",
			},
			wantErr: false,
		},
		{
			name: "unknown variable",
			in: &Math{
				M:     "1 + 3",
				W:     "mean(missing)",
				Drift: "1 - 0",
			},
			wantErr: true,
		},
		{
			name: "no data",
			in: &Math{
				M:     "",
				W:     "",
				Drift: "",
			},
			wantErr: true,
		},
		{
			name: "all good",
			in: &Math{
				M:     "abs(mean(offset, 30)) + 1.0 * stddev(offset, 30) + 1.0 * stddev(delay, 10) + 1.0 * stddev(freq, 5)",
				W:     "mean(m, 30) + 4.0 * stddev(m, 30)",
				Drift: "mean(freqchangeabs, 29)",
			},
			wantErr: false,
		},
		{
			name: "all good with clock accuracy",
			in: &Math{
				M:     "mean(clockaccuracy, 30) + abs(mean(offset, 30)) + 1.0 * stddev(offset, 30)",
				W:     "mean(m, 30) + 4.0 * stddev(m, 30)",
				Drift: "mean(freqchangeabs, 29)",
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.Prepare()
			if tc.wantErr {
				require.Error(t, got)
			} else {
				require.NoError(t, got)
			}
		})
	}
}

func TestDataPointSanityCheck(t *testing.T) {
	testCases := []struct {
		name    string
		in      *DataPoint
		wantErr bool
	}{
		{
			name: "no ingress time",
			in: &DataPoint{
				IngressTimeNS:     0,
				MasterOffsetNS:    23.0,
				PathDelayNS:       213.0,
				FreqAdjustmentPPB: 212131,
				ClockAccuracyNS:   25.0,
			},
			wantErr: true,
		},
		{
			name: "zero offset",
			in: &DataPoint{
				IngressTimeNS:     1647359186979431900,
				MasterOffsetNS:    0,
				PathDelayNS:       213.0,
				FreqAdjustmentPPB: 212131,
				ClockAccuracyNS:   25.0,
			},
			wantErr: true,
		},
		{
			name: "zero delay",
			in: &DataPoint{
				IngressTimeNS:     1647359186979431900,
				MasterOffsetNS:    123,
				PathDelayNS:       0,
				FreqAdjustmentPPB: 212131,
				ClockAccuracyNS:   25.0,
			},
			wantErr: true,
		},
		{
			name: "zero freq",
			in: &DataPoint{
				IngressTimeNS:     1647359186979431900,
				MasterOffsetNS:    123,
				PathDelayNS:       213.0,
				FreqAdjustmentPPB: 0,
				ClockAccuracyNS:   25.0,
			},
			wantErr: true,
		},
		{
			name: "zero clock accuracy",
			in: &DataPoint{
				IngressTimeNS:     1647359186979431900,
				MasterOffsetNS:    123,
				PathDelayNS:       213.0,
				FreqAdjustmentPPB: 212131,
				ClockAccuracyNS:   0,
			},
			wantErr: true,
		},
		{
			name: "unknown clock accuracy",
			in: &DataPoint{
				IngressTimeNS:     1647359186979431900,
				MasterOffsetNS:    123,
				PathDelayNS:       213.0,
				FreqAdjustmentPPB: 212131,
				ClockAccuracyNS:   float64(ptp.ClockAccuracyUnknown.Duration()),
			},
			wantErr: true,
		},
		{
			name: "all good",
			in: &DataPoint{
				IngressTimeNS:     1647359186979431900,
				MasterOffsetNS:    123,
				PathDelayNS:       213.0,
				FreqAdjustmentPPB: 212131,
				ClockAccuracyNS:   25.0,
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.SanityCheck()
			if tc.wantErr {
				require.Error(t, got)
			} else {
				require.NoError(t, got)
			}
		})
	}
}

func TestDaemonCalculateSHMData(t *testing.T) {
	cfg := &Config{
		RingSize: 30,
		Math: Math{
			M:     "mean(clockaccuracy, 30) + abs(mean(offset, 30)) + 1.0 * stddev(offset, 30)",
			W:     "mean(m, 30) + 4.0 * stddev(m, 30)",
			Drift: "1.5 * mean(freqchangeabs, 29)",
		},
	}
	leaps := []leapsectz.LeapSecond{
		{Tleap: 1435708825, Nleap: 26},
		{Tleap: 1483228826, Nleap: 27},
	}

	err := cfg.Math.Prepare()
	require.NoError(t, err)
	stats := stats.NewStats()
	s := newTestDaemon(cfg, stats)
	startTime := time.Duration(1647359186979431900)
	var d *DataPoint
	adj := 212131.0
	for i := range 58 {
		if i%2 == 0 {
			adj += float64(i)
		} else {
			adj -= float64(i)
		}
		d = &DataPoint{
			IngressTimeNS:     int64(startTime + time.Duration(i)*time.Second),
			MasterOffsetNS:    23.0,
			PathDelayNS:       213.0,
			FreqAdjustmentPPB: adj,
			ClockAccuracyNS:   100.0,
		}
		shmData, err := s.calculateSHMData(d, leaps)
		if i < 29 {
			require.Nil(t, shmData)
			require.Error(t, err, "not enough data should give us error when calculating shm state")
		} else {
			require.NotNil(t, shmData)
			require.NoError(t, err)
		}
	}
	d = &DataPoint{
		IngressTimeNS:     int64(startTime + 61*time.Second),
		MasterOffsetNS:    23.0,
		PathDelayNS:       213.0,
		FreqAdjustmentPPB: 212131,
		ClockAccuracyNS:   100.0,
	}

	want := &fbclock.Data{
		IngressTimeNS:        d.IngressTimeNS,
		ErrorBoundNS:         123.0,
		HoldoverMultiplierNS: 64.5,
		SmearingStartS:       1483228836,
		SmearingEndS:         1483291336,
		UTCOffsetPreS:        36,
		UTCOffsetPostS:       37,
	}
	shmData, err := s.calculateSHMData(d, leaps)
	require.NoError(t, err)
	require.Equal(t, want, shmData)

	// ptp4l got restarted, not yet syncing, all values are zeroes
	d = &DataPoint{
		IngressTimeNS:     0,
		MasterOffsetNS:    0,
		PathDelayNS:       0,
		FreqAdjustmentPPB: 0,
	}
	shmData, err = s.calculateSHMData(d, leaps)
	require.Error(t, err, "we expect calculateSHMData to produce no new shm state when new input is invalid")
	require.Nil(t, shmData)
	// ptp4l started syncing, but haven't started updating the clock
	d = &DataPoint{
		IngressTimeNS:     int64(startTime + 65*time.Second),
		MasterOffsetNS:    0,
		PathDelayNS:       213,
		FreqAdjustmentPPB: 0,
	}
	shmData, err = s.calculateSHMData(d, leaps)
	require.Nil(t, shmData)
	require.Error(t, err, "we expect calculateSHMData to produce no new shm state when new input is incomplete")

	// ptp4l is back to normal operations
	d = &DataPoint{
		IngressTimeNS:     int64(startTime + 66*time.Second),
		MasterOffsetNS:    234,
		PathDelayNS:       213,
		FreqAdjustmentPPB: 32333,
		ClockAccuracyNS:   100,
	}
	shmData, err = s.calculateSHMData(d, leaps)
	want = &fbclock.Data{
		IngressTimeNS:        d.IngressTimeNS,
		ErrorBoundNS:         157,
		HoldoverMultiplierNS: 9362.84482758621,
		SmearingStartS:       1483228836,
		SmearingEndS:         1483291336,
		UTCOffsetPreS:        36,
		UTCOffsetPostS:       37,
	}
	require.NoError(t, err)
	require.Equal(t, want, shmData)

	// no leap second data
	leaps = []leapsectz.LeapSecond{}
	d = &DataPoint{
		IngressTimeNS:     int64(startTime + 66*time.Second),
		MasterOffsetNS:    234,
		PathDelayNS:       213,
		FreqAdjustmentPPB: 32333,
		ClockAccuracyNS:   100,
	}
	shmData, err = s.calculateSHMData(d, leaps)
	want = &fbclock.Data{
		IngressTimeNS:        d.IngressTimeNS,
		ErrorBoundNS:         185,
		HoldoverMultiplierNS: 9361.241379310344,
		SmearingStartS:       0,
		SmearingEndS:         0,
		UTCOffsetPreS:        0,
		UTCOffsetPostS:       0,
	}
	require.NoError(t, err)
	require.Equal(t, want, shmData)
}

func TestDaemonDoWork(t *testing.T) {
	cfg := &Config{
		Interval: time.Second,
		RingSize: 30,
		Math: Math{
			M:     "mean(clockaccuracy, 30) + abs(mean(offset, 30)) + 1.0 * stddev(offset, 30)",
			W:     "mean(m, 30) + 4.0 * stddev(m, 30)",
			Drift: "1.5 * mean(freqchangeabs, 29)",
		},
	}
	err := cfg.Math.Prepare()
	require.NoError(t, err)
	stats := stats.NewStats()
	s := newTestDaemon(cfg, stats)
	startTime := time.Duration(1647359186979431900)
	phcTime := startTime // we modify this during the test
	// override function to get PHC time
	s.getPHCTime = func() (time.Time, error) { return time.Unix(0, int64(phcTime)), nil }
	// shared mem
	tmpFile, err := os.CreateTemp("", "daemon_test")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	shm, err := fbclock.OpenFBClockShmCustom(tmpFile.Name())
	require.NoError(t, err)
	defer shm.Close()

	// populate the data
	var d *DataPoint

	// bad data (ptp4l is just starting)
	for i := range 10 {
		d = &DataPoint{
			IngressTimeNS:     0,
			MasterOffsetNS:    0,
			PathDelayNS:       0,
			FreqAdjustmentPPB: 0,
		}
		err = s.doWork(shm, d)
		require.Error(t, err, "not enough data should give us error when calculating shm state")
		c := stats.Get()
		// not enough data for those
		require.Equal(t, int64(0), c["ingress_time_ns"])
		require.Equal(t, int64(0), c["master_offset_ns"])
		require.Equal(t, int64(0), c["path_delay_ns"])
		require.Equal(t, int64(0), c["freq_adj_ppb"])
		require.Equal(t, int64(0), c["dift_ppb"])
		require.Equal(t, int64(0), c["master_offset_ns.60.abs_max"])
		require.Equal(t, int64(0), c["path_delay_ns.60.abs_max"])
		require.Equal(t, int64(0), c["freq_adj_ppb.60.abs_max"])
		require.Equal(t, int64(i+1), c["data_sanity_check_error"])
	}
	// good data
	adj := 212131.0
	for i := range 58 {
		if i%2 == 0 {
			adj += float64(i)
		} else {
			adj -= float64(i)
		}
		tme := startTime + time.Duration(i)*time.Second
		d = &DataPoint{
			IngressTimeNS:     int64(tme),
			MasterOffsetNS:    23.0,
			PathDelayNS:       213.0,
			FreqAdjustmentPPB: adj,
			ClockAccuracyNS:   25.0,
		}
		phcTime = tme + time.Microsecond
		err = s.doWork(shm, d)
		require.NoError(t, err, "not enough data should give us error when calculating shm state, which we log and continue")
		// check exported stats
		c := stats.Get()
		require.Equal(t, int64(tme), c["ingress_time_ns"])
		require.Equal(t, int64(d.MasterOffsetNS), c["master_offset_ns"])
		require.Equal(t, int64(d.PathDelayNS), c["path_delay_ns"])
		require.Equal(t, int64(d.FreqAdjustmentPPB), c["freq_adj_ppb"])
		require.Equal(t, int64(0), c["data_sanity_check_error"])
		// we can calculate M after 30 seconds
		require.Equal(t, int64(48), c["m_ns"])
		require.Equal(t, int64(48), c["m_ns"])

		if i < 29 {
			require.Equal(t, int64(0), c["w_ns"])
			require.Equal(t, int64(0), c["master_offset_ns.60.abs_max"])
			require.Equal(t, int64(0), c["path_delay_ns.60.abs_max"])
			require.Equal(t, int64(0), c["freq_adj_ppb.60.abs_max"])
		} else {
			require.Equal(t, int64(48), c["w_ns"])
			require.Equal(t, int64(23), c["master_offset_ns.60.abs_max"])
			require.Equal(t, int64(213), c["path_delay_ns.60.abs_max"])
			require.LessOrEqual(t, int64(212145), c["freq_adj_ppb.60.abs_max"])
		}

		// not enough data for those
		require.Equal(t, int64(0), c["dift_ppb"])
	}

	// another data point, now that we have enough in the ring buffer to write to shm
	d = &DataPoint{
		IngressTimeNS:     int64(startTime + 61*time.Second),
		MasterOffsetNS:    23.0,
		PathDelayNS:       213.0,
		FreqAdjustmentPPB: 212131,
		ClockAccuracyNS:   25.0,
	}
	phcTime = startTime + 62*time.Second

	err = s.doWork(shm, d)
	require.NoError(t, err)
	// check that we have proper stats reported
	c := stats.Get()
	require.Equal(t, int64(startTime+61*time.Second), c["ingress_time_ns"], "ingress_time_ns after good data")
	require.Equal(t, int64(d.MasterOffsetNS), c["master_offset_ns"], "master_offset_ns after good data")
	require.Equal(t, int64(d.PathDelayNS), c["path_delay_ns"], "path_delay_ns after good data")
	require.Equal(t, int64(d.FreqAdjustmentPPB), c["freq_adj_ppb"], "freq_adj_ppb after good data")
	require.Equal(t, int64(48), c["m_ns"], "m_ns after good data")
	require.Equal(t, int64(48), c["w_ns"], "w_ns after good data")
	require.Equal(t, int64(64), c["drift_ppb"], "drift_ppb after good data")
	require.Equal(t, int64(23), c["master_offset_ns.60.abs_max"], "master_offset_ns.60.abs_max after good data")
	require.Equal(t, int64(213), c["path_delay_ns.60.abs_max"], "path_delay_ns.60.abs_max after good data")
	require.Equal(t, int64(212159), c["freq_adj_ppb.60.abs_max"], "freq_adj_ppb.60.abs_max after good data")
	require.Equal(t, int64(0), c["data_sanity_check_error"])

	// check that we wrote data correctly
	want := &fbclock.Data{
		IngressTimeNS:        d.IngressTimeNS,
		ErrorBoundNS:         48.0,
		HoldoverMultiplierNS: 64.5,
	}
	shmpData, err := fbclock.MmapShmpData(shm.File.Fd())
	require.NoError(t, err)
	got, err := fbclock.ReadFBClockData(shmpData)
	require.NoError(t, err)
	require.Equal(t, want.IngressTimeNS, got.IngressTimeNS)
	require.Equal(t, want.ErrorBoundNS, got.ErrorBoundNS)
	require.InDelta(t, want.HoldoverMultiplierNS, got.HoldoverMultiplierNS, 0.001)

	// ptp4l has a hiccup, but that should be okay
	d = &DataPoint{
		IngressTimeNS:     0,
		MasterOffsetNS:    0,
		PathDelayNS:       0,
		FreqAdjustmentPPB: 0,
	}
	phcTime = startTime + 63*time.Second

	err = s.doWork(shm, d)
	require.Error(t, err, "data point fails sanity check")
	// check that we have proper stats reported
	c = stats.Get()
	require.Equal(t, int64(0), c["ingress_time_ns"], "ingress_time_ns after bad data")
	require.Equal(t, int64(d.MasterOffsetNS), c["master_offset_ns"], "master_offset_ns after bad data")
	require.Equal(t, int64(d.PathDelayNS), c["path_delay_ns"], "path_delay_ns after bad data")
	require.Equal(t, int64(d.FreqAdjustmentPPB), c["freq_adj_ppb"], "freq_adj_ppb after bad data")
	require.Equal(t, int64(48), c["m_ns"], "m_ns after bad data")
	require.Equal(t, int64(48), c["w_ns"], "w_ns after bad data")
	require.Equal(t, int64(64), c["drift_ppb"], "drift_ppb after bad data")
	require.Equal(t, int64(23), c["master_offset_ns.60.abs_max"], "master_offset_ns.60.abs_max after bad data")
	require.Equal(t, int64(213), c["path_delay_ns.60.abs_max"], "path_delay_ns.60.abs_max after bad data")
	require.Equal(t, int64(212159), c["freq_adj_ppb.60.abs_max"], "freq_adj_ppb.60.abs_max after bad data")
	require.Equal(t, int64(1), c["data_sanity_check_error"])

	// check that we wrote data correctly (should be the same as before, as new data point was discarded)
	got, err = fbclock.ReadFBClockData(shmpData)
	require.NoError(t, err)
	require.Equal(t, want.IngressTimeNS, got.IngressTimeNS)
	require.Equal(t, want.ErrorBoundNS, got.ErrorBoundNS)
	require.InDelta(t, want.HoldoverMultiplierNS, got.HoldoverMultiplierNS, 0.001)
}

func TestLeapSecondSmearing(t *testing.T) {
	// assume that these were the 2 most recent leap seconds in tzinfo data
	leaps := []leapsectz.LeapSecond{
		{Tleap: 1435708837, Nleap: 25}, // Wednesday, 1 July 2015 00:00:25 | 25 leap seconds
		{Tleap: 1483228826, Nleap: 26}, // Sunday, 01 Jan 2017 00:00:26 UTC | 26 leap seconds
	}
	got := leapSecondSmearing(leaps)
	want := &clockSmearing{
		smearingStartS: 1483228836, // Sun, 01 Jan 2017 00:00:36 TAI (or Sat, 31 Dec 2016 12:00:00 UTC)
		smearingEndS:   1483291336, // Sun, 01 Jan 2017 17:22:53 TAI (or Sun, 01 Jan 2017 17:22:16 UTC)
		utcOffsetPreS:  35,
		utcOffsetPostS: 36,
	}
	require.Equal(t, want, got)

	// less than 2 leap second events in tzdata
	leaps = []leapsectz.LeapSecond{
		{Tleap: 1483228826, Nleap: 26}, // Sunday, 01 Jan 2017 00:00:26 UTC | 26 leap seconds
	}
	got = leapSecondSmearing(leaps)
	want = &clockSmearing{}
	require.Equal(t, want, got)
}

func TestRunLinearizabilityTestsNoGMs(t *testing.T) {
	cfg := &Config{
		LinearizabilityTestInterval: time.Second,
	}
	stats := stats.NewStats()
	s := newTestDaemon(cfg, stats)
	go s.runLinearizabilityTests(context.Background())
	time.Sleep(100 * time.Millisecond)
	c := stats.Get()
	t.Log(c)
	require.Equal(t, int64(len(defaultTargets)), c["linearizability.total_tests"], "linearizability.total_tests must be set")
	require.Equal(t, int64(len(defaultTargets)), c["linearizability.failed_tests"], "linearizability.failed_tests must be set")
	require.Equal(t, int64(0), c["linearizability.passed_tests"])
}

func TestNoTestResults(t *testing.T) {
	targets := []string{"o", "l", "e", "g"}
	want := map[string]linearizability.TestResult{
		"o": linearizability.SPTPHTTPTestResult{Error: errNoTestResults},
		"l": linearizability.SPTPHTTPTestResult{Error: errNoTestResults},
		"e": linearizability.SPTPHTTPTestResult{Error: errNoTestResults},
		"g": linearizability.SPTPHTTPTestResult{Error: errNoTestResults},
	}

	got := noTestResults(targets)
	require.Equal(t, want, got)
}

func TestNoPHC(t *testing.T) {
	cfg := &Config{}
	stats := stats.NewStats()
	s := newTestDaemon(cfg, stats)
	s.getPHCTime = func() (time.Time, error) { return time.Time{}, errNoPHC }

	err := s.doWork(&fbclock.Shm{}, &DataPoint{})
	require.ErrorIs(t, err, errNoPHC)
}

func TestCoeffV2(t *testing.T) {
	prevDataV2 := fbclock.DataV2{}
	curDataV2 := fbclock.DataV2{}
	c, err := calcCoeffPPB(&prevDataV2, &curDataV2)
	require.Equal(t, int64(0), c)
	require.NoError(t, err)
	prevDataV2.SysclockTimeNS = 1749167822494826022
	prevDataV2.PHCTimeNS = 1749167859494830869
	curDataV2.SysclockTimeNS = 1749167822504951677
	curDataV2.PHCTimeNS = 1749167859504956519
	c, err = calcCoeffPPB(&prevDataV2, &curDataV2)
	require.Equal(t, int64(-493), c)
	require.NoError(t, err)
}
