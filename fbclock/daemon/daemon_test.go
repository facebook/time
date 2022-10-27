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
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/facebook/time/fbclock"
	"github.com/facebook/time/ptp/linearizability"

	ptp "github.com/facebook/time/ptp/protocol"
)

type testLogger struct {
	samples []*LogSample
}

func (l *testLogger) Log(s *LogSample) error {
	l.samples = append(l.samples, s)
	return nil
}

func newTestDaemon(cfg *Config, stats StatsServer) *Daemon {
	s := &Daemon{
		stats: stats,
		cfg:   cfg,
		state: newDaemonState(cfg.RingSize),
		l:     &testLogger{samples: []*LogSample{}},
	}
	return s
}

func TestDaemonStateLinearizabilityRing(t *testing.T) {
	s := newDaemonState(3)

	probes := []*linearizability.TestResult{
		{
			Server:      "server01",
			TXTimestamp: time.Unix(0, 1647359186979431100),
			RXTimestamp: time.Unix(0, 1647359186979431635),
		},
		{
			Server:      "server02",
			TXTimestamp: time.Unix(0, 1647359186979431200),
			RXTimestamp: time.Unix(0, 1647359186979431735),
		},
		{
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

	probes := []*dataPoint{
		{
			masterOffsetNS:    123.0,
			pathDelayNS:       3,
			freqAdjustmentPPB: 4,
		},
		{
			masterOffsetNS:    -2000.0,
			pathDelayNS:       300,
			freqAdjustmentPPB: 2,
		},
		{
			masterOffsetNS:    1009.0,
			pathDelayNS:       200,
			freqAdjustmentPPB: 5,
		},
	}

	for _, tr := range probes {
		s.pushDataPoint(tr)
	}
	got := s.aggregateDataPointsMax(3)
	want := &dataPoint{
		masterOffsetNS:    2000.0,
		pathDelayNS:       300,
		freqAdjustmentPPB: 5,
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
			assert.ElementsMatch(t, tc.wantAdded, gotAdded, "added")
			assert.ElementsMatch(t, tc.wantRemoved, gotRemoved, "removed")
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
				assert.Error(t, got)
			} else {
				assert.NoError(t, got)
			}
		})
	}
}

func TestDataPointSanityCheck(t *testing.T) {
	testCases := []struct {
		name    string
		in      *dataPoint
		wantErr bool
	}{
		{
			name: "no ingress time",
			in: &dataPoint{
				ingressTimeNS:     0,
				masterOffsetNS:    23.0,
				pathDelayNS:       213.0,
				freqAdjustmentPPB: 212131,
				clockAccuracyNS:   25.0,
			},
			wantErr: true,
		},
		{
			name: "zero offset",
			in: &dataPoint{
				ingressTimeNS:     1647359186979431900,
				masterOffsetNS:    0,
				pathDelayNS:       213.0,
				freqAdjustmentPPB: 212131,
				clockAccuracyNS:   25.0,
			},
			wantErr: true,
		},
		{
			name: "zero delay",
			in: &dataPoint{
				ingressTimeNS:     1647359186979431900,
				masterOffsetNS:    123,
				pathDelayNS:       0,
				freqAdjustmentPPB: 212131,
				clockAccuracyNS:   25.0,
			},
			wantErr: true,
		},
		{
			name: "zero freq",
			in: &dataPoint{
				ingressTimeNS:     1647359186979431900,
				masterOffsetNS:    123,
				pathDelayNS:       213.0,
				freqAdjustmentPPB: 0,
				clockAccuracyNS:   25.0,
			},
			wantErr: true,
		},
		{
			name: "zero clock accuracy",
			in: &dataPoint{
				ingressTimeNS:     1647359186979431900,
				masterOffsetNS:    123,
				pathDelayNS:       213.0,
				freqAdjustmentPPB: 212131,
				clockAccuracyNS:   0,
			},
			wantErr: true,
		},
		{
			name: "unknown clock accuracy",
			in: &dataPoint{
				ingressTimeNS:     1647359186979431900,
				masterOffsetNS:    123,
				pathDelayNS:       213.0,
				freqAdjustmentPPB: 212131,
				clockAccuracyNS:   float64(ptp.ClockAccuracyUnknown.Duration()),
			},
			wantErr: true,
		},
		{
			name: "all good",
			in: &dataPoint{
				ingressTimeNS:     1647359186979431900,
				masterOffsetNS:    123,
				pathDelayNS:       213.0,
				freqAdjustmentPPB: 212131,
				clockAccuracyNS:   25.0,
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.SanityCheck()
			if tc.wantErr {
				assert.Error(t, got)
			} else {
				assert.NoError(t, got)
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
			Drift: "mean(freqchangeabs, 29)",
		},
	}
	err := cfg.Math.Prepare()
	require.NoError(t, err)
	stats := NewStats()
	s := newTestDaemon(cfg, stats)
	startTime := time.Duration(1647359186979431900)
	var d *dataPoint
	adj := 212131.0
	for i := 0; i < 58; i++ {
		if i%2 == 0 {
			adj += float64(i)
		} else {
			adj -= float64(i)
		}
		d = &dataPoint{
			ingressTimeNS:     int64(startTime + time.Duration(i)*time.Second),
			masterOffsetNS:    23.0,
			pathDelayNS:       213.0,
			freqAdjustmentPPB: adj,
			clockAccuracyNS:   100.0,
		}
		shmData, err := s.calculateSHMData(d)
		require.Nil(t, shmData)
		require.Error(t, err, "not enough data should give us error when calculating shm state")
	}
	d = &dataPoint{
		ingressTimeNS:     int64(startTime + 61*time.Second),
		masterOffsetNS:    23.0,
		pathDelayNS:       213.0,
		freqAdjustmentPPB: 212131,
		clockAccuracyNS:   100.0,
	}

	want := &fbclock.Data{
		IngressTimeNS:        d.ingressTimeNS,
		ErrorBoundNS:         123.0,
		HoldoverMultiplierNS: 64.5,
	}
	shmData, err := s.calculateSHMData(d)
	require.NoError(t, err)
	require.Equal(t, want, shmData)

	// ptp4l got restarted, not yet syncing, all values are zeroes
	d = &dataPoint{
		ingressTimeNS:     0,
		masterOffsetNS:    0,
		pathDelayNS:       0,
		freqAdjustmentPPB: 0,
	}
	shmData, err = s.calculateSHMData(d)
	require.Error(t, err, "we expect calculateSHMData to produce no new shm state when new input is invalid")
	require.Nil(t, shmData)
	// ptp4l started syncing, but haven't started updating the clock
	d = &dataPoint{
		ingressTimeNS:     int64(startTime + 65*time.Second),
		masterOffsetNS:    0,
		pathDelayNS:       213,
		freqAdjustmentPPB: 0,
	}
	shmData, err = s.calculateSHMData(d)
	require.Nil(t, shmData)
	require.Error(t, err, "we expect calculateSHMData to produce no new shm state when new input is incomplete")

	// ptp4l is back to normal operations
	d = &dataPoint{
		ingressTimeNS:     int64(startTime + 66*time.Second),
		masterOffsetNS:    234,
		pathDelayNS:       213,
		freqAdjustmentPPB: 32333,
		clockAccuracyNS:   100,
	}
	shmData, err = s.calculateSHMData(d)
	want = &fbclock.Data{
		IngressTimeNS:        d.ingressTimeNS,
		ErrorBoundNS:         157,
		HoldoverMultiplierNS: 9362.84482758621,
	}
	require.NoError(t, err)
	assert.Equal(t, want, shmData)
}

func TestDaemonDoWork(t *testing.T) {
	cfg := &Config{
		Interval: time.Second,
		RingSize: 30,
		Math: Math{
			M:     "mean(clockaccuracy, 30) + abs(mean(offset, 30)) + 1.0 * stddev(offset, 30)",
			W:     "mean(m, 30) + 4.0 * stddev(m, 30)",
			Drift: "mean(freqchangeabs, 29)",
		},
	}
	err := cfg.Math.Prepare()
	require.NoError(t, err)
	stats := NewStats()
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
	var d *dataPoint

	// bad data (ptp4l is just starting)
	for i := 0; i < 10; i++ {
		d = &dataPoint{
			ingressTimeNS:     0,
			masterOffsetNS:    0,
			pathDelayNS:       0,
			freqAdjustmentPPB: 0,
		}
		err = s.doWork(shm, d)
		require.Error(t, err, "not enough data should give us error when calculating shm state")
		// not enough data for those
		require.Equal(t, int64(0), stats.counters["ingress_time_ns"])
		require.Equal(t, int64(0), stats.counters["time_since_ingress_ns"])
		require.Equal(t, int64(0), stats.counters["master_offset_ns"])
		require.Equal(t, int64(0), stats.counters["path_delay_ns"])
		require.Equal(t, int64(0), stats.counters["freq_adj_ppb"])
		require.Equal(t, int64(0), stats.counters["master_offset_ns.60.abs_max"])
		require.Equal(t, int64(0), stats.counters["path_delay_ns.60.abs_max"])
		require.Equal(t, int64(0), stats.counters["freq_adj_ppb.60.abs_max"])
		require.Equal(t, int64(i+1), stats.counters["data_sanity_check_error"])
	}
	// good data
	adj := 212131.0
	for i := 0; i < 58; i++ {
		if i%2 == 0 {
			adj += float64(i)
		} else {
			adj -= float64(i)
		}
		tme := startTime + time.Duration(i)*time.Second
		d = &dataPoint{
			ingressTimeNS:     int64(tme),
			masterOffsetNS:    23.0,
			pathDelayNS:       213.0,
			freqAdjustmentPPB: adj,
			clockAccuracyNS:   25.0,
		}
		phcTime = tme + time.Microsecond
		err = s.doWork(shm, d)
		require.NoError(t, err, "not enough data should give us error when calculating shm state, which we log and continue")
		// check exported stats
		require.Equal(t, int64(tme), stats.counters["ingress_time_ns"])
		require.Equal(t, int64(time.Microsecond), stats.counters["time_since_ingress_ns"])
		require.Equal(t, int64(d.masterOffsetNS), stats.counters["master_offset_ns"])
		require.Equal(t, int64(d.pathDelayNS), stats.counters["path_delay_ns"])
		require.Equal(t, int64(d.freqAdjustmentPPB), stats.counters["freq_adj_ppb"])
		require.Equal(t, int64(0), stats.counters["data_sanity_check_error"])
		// we can calculate M after 30 seconds
		if i < 29 {
			require.Equal(t, int64(0), stats.counters["m_ns"])
		} else {
			require.Equal(t, int64(48), stats.counters["m_ns"])
		}
		require.Equal(t, int64(0), stats.counters["w_ns"])
		// not enough data for those
		require.Equal(t, int64(0), stats.counters["master_offset_ns.60.abs_max"])
		require.Equal(t, int64(0), stats.counters["path_delay_ns.60.abs_max"])
		require.Equal(t, int64(0), stats.counters["freq_adj_ppb.60.abs_max"])
	}

	// another data point, now that we have enough in the ring buffer to write to shm
	d = &dataPoint{
		ingressTimeNS:     int64(startTime + 61*time.Second),
		masterOffsetNS:    23.0,
		pathDelayNS:       213.0,
		freqAdjustmentPPB: 212131,
		clockAccuracyNS:   25.0,
	}
	phcTime = startTime + 62*time.Second

	err = s.doWork(shm, d)
	require.NoError(t, err)
	// check that we have proper stats reported
	require.Equal(t, int64(startTime+61*time.Second), stats.counters["ingress_time_ns"], "ingress_time_ns after good data")
	require.Equal(t, int64(time.Second), stats.counters["time_since_ingress_ns"], "time_since_ingress_ns after good data")
	require.Equal(t, int64(d.masterOffsetNS), stats.counters["master_offset_ns"], "master_offset_ns after good data")
	require.Equal(t, int64(d.pathDelayNS), stats.counters["path_delay_ns"], "path_delay_ns after good data")
	require.Equal(t, int64(d.freqAdjustmentPPB), stats.counters["freq_adj_ppb"], "freq_adj_ppb after good data")
	require.Equal(t, int64(48), stats.counters["m_ns"], "m_ns after good data")
	require.Equal(t, int64(48), stats.counters["w_ns"], "w_ns after good data")
	require.Equal(t, int64(23), stats.counters["master_offset_ns.60.abs_max"], "master_offset_ns.60.abs_max after good data")
	require.Equal(t, int64(213), stats.counters["path_delay_ns.60.abs_max"], "path_delay_ns.60.abs_max after good data")
	require.Equal(t, int64(212159), stats.counters["freq_adj_ppb.60.abs_max"], "freq_adj_ppb.60.abs_max after good data")
	require.Equal(t, int64(0), stats.counters["data_sanity_check_error"])

	// check that we wrote data correctly
	want := &fbclock.Data{
		IngressTimeNS:        d.ingressTimeNS,
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
	d = &dataPoint{
		ingressTimeNS:     0,
		masterOffsetNS:    0,
		pathDelayNS:       0,
		freqAdjustmentPPB: 0,
	}
	phcTime = startTime + 63*time.Second

	err = s.doWork(shm, d)
	require.Error(t, err, "data point fails sanity check")
	// check that we have proper stats reported
	require.Equal(t, int64(0), stats.counters["ingress_time_ns"], "ingress_time_ns after bad data")
	require.Equal(t, int64(2*time.Second), stats.counters["time_since_ingress_ns"], "time_since_ingress_ns after bad data")
	require.Equal(t, int64(d.masterOffsetNS), stats.counters["master_offset_ns"], "master_offset_ns after bad data")
	require.Equal(t, int64(d.pathDelayNS), stats.counters["path_delay_ns"], "path_delay_ns after bad data")
	require.Equal(t, int64(d.freqAdjustmentPPB), stats.counters["freq_adj_ppb"], "freq_adj_ppb after bad data")
	require.Equal(t, int64(48), stats.counters["m_ns"], "m_ns after bad data")
	require.Equal(t, int64(48), stats.counters["w_ns"], "w_ns after bad data")
	require.Equal(t, int64(23), stats.counters["master_offset_ns.60.abs_max"], "master_offset_ns.60.abs_max after bad data")
	require.Equal(t, int64(213), stats.counters["path_delay_ns.60.abs_max"], "path_delay_ns.60.abs_max after bad data")
	require.Equal(t, int64(212159), stats.counters["freq_adj_ppb.60.abs_max"], "freq_adj_ppb.60.abs_max after bad data")
	require.Equal(t, int64(1), stats.counters["data_sanity_check_error"])

	// check that we wrote data correctly (should be the same as before, as new data point was discarded)
	got, err = fbclock.ReadFBClockData(shmpData)
	require.NoError(t, err)
	require.Equal(t, want.IngressTimeNS, got.IngressTimeNS)
	require.Equal(t, want.ErrorBoundNS, got.ErrorBoundNS)
	require.InDelta(t, want.HoldoverMultiplierNS, got.HoldoverMultiplierNS, 0.001)
}
