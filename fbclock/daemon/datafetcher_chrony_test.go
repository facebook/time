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
	"net"
	"os"
	"testing"
	"time"

	"github.com/facebook/time/fbclock"
	"github.com/facebook/time/fbclock/stats"
	"github.com/facebook/time/leapsectz"
	"github.com/facebook/time/ntp/chrony"
	ntp "github.com/facebook/time/ntp/protocol"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// trackingReply is a real reply captured from chronyd, the same fixture the
// chrony package decodes in its own tests.
var trackingReply = []uint8{
	0x06, 0x02, 0x00, 0x00, 0x00, 0x21, 0x00, 0x05, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xe6, 0x25,
	0xc6, 0x6e, 0x24, 0x01, 0xdb, 0x00, 0x31, 0x10, 0x21, 0x32,
	0xfa, 0xce, 0x00, 0x00, 0x00, 0x8e, 0x00, 0x00, 0x00, 0x02,
	0x00, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x61, 0x38, 0xe1, 0x81, 0x36, 0x94, 0x8d, 0xd5, 0xdf, 0x19,
	0x2d, 0xb7, 0xdf, 0x42, 0x83, 0xf5, 0xe2, 0xeb, 0xca, 0x12,
	0x05, 0x39, 0xe1, 0x11, 0xeb, 0x7b, 0x3e, 0x5d, 0xf4, 0xb0,
	0x75, 0x12, 0xea, 0xe7, 0x5b, 0x0c, 0xf0, 0x88, 0x1d, 0x4e,
	0x16, 0x82, 0x1f, 0x69,
}

// fakeChronyd answers the first request on a loopback UDP port; nil leaves it unanswered.
func fakeChronyd(t *testing.T, reply []uint8) (address string) {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	go func() {
		buf := make([]uint8, 1024)
		_, client, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if reply != nil {
			_, _ = conn.WriteToUDP(reply, client)
		}
	}()
	return conn.LocalAddr().String()
}

func TestFetchTracking(t *testing.T) {
	cfg := &Config{Interval: 2 * time.Second}

	tracking, err := (&ChronyFetcher{address: fakeChronyd(t, trackingReply)}).FetchTracking(cfg)
	require.NoError(t, err)
	require.Equal(t, uint16(3), tracking.Stratum)
	require.Equal(t, time.Unix(0, 1631117697915705301), tracking.RefTime)
	require.Equal(t, 0.0010384710039943457, tracking.RootDispersion)
}

func TestFetchTrackingTimeout(t *testing.T) {
	// UDP gives no connection-refused, so the read deadline ends the poll
	cfg := &Config{Interval: 20 * time.Millisecond}

	_, err := (&ChronyFetcher{address: fakeChronyd(t, nil)}).FetchTracking(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to get tracking from chronyd")
}

// testUTCOffsetS is what current tzdata yields (latest leap 2017-01-01), and
// leap2017 is that leap instant: Tleap-Nleap+1 of the record below.
const (
	testUTCOffsetS = 37
	leap2017       = 1483228800
)

var (
	// testLeaps is verbatim from /usr/share/zoneinfo/right/UTC
	testLeaps = []leapsectz.LeapSecond{
		{Tleap: 1435708825, Nleap: 26},
		{Tleap: 1483228826, Nleap: 27},
	}
	// testSysTime is the CLOCK_REALTIME read the daemon stamps at fetch time, a
	// second after the fixtures' RefTime so assertions can tell them apart.
	testSysTime = time.Unix(1755000001, 0)
	// testAsOf is what it publishes: that reading shifted into TAI.
	testAsOf = testSysTime.Add(testUTCOffsetS * time.Second)
)

func newChronyTestDaemon(t *testing.T, cfg *Config, st stats.Server) *Daemon {
	t.Helper()
	return &Daemon{
		stats:       st,
		cfg:         cfg,
		state:       newDaemonState(1),
		l:           &testLogger{samples: []*LogSample{}},
		DataFetcher: &ChronyFetcher{},
		getSysTime:  func() (time.Time, error) { return testSysTime, nil },
	}
}

func TestCalculateSHMDataChronyErrorBound(t *testing.T) {
	cfg := &Config{Chrony: true, Interval: time.Second, MaxDriftRate: 50}
	s := newChronyTestDaemon(t, cfg, stats.NewStats())
	tracking := &chrony.Tracking{
		RefTime:           time.Unix(1755000000, 0),
		CurrentCorrection: -1 * time.Microsecond.Seconds(),
		RootDispersion:    2 * time.Microsecond.Seconds(),
		RootDelay:         4 * time.Microsecond.Seconds(),
		SkewPPM:           0.005,
	}

	d, err := s.calculateSHMDataChrony(tracking, testAsOf, nil)
	require.NoError(t, err)
	// |-1us| + 2us + 4us/2 = 5us; the correction contributes magnitude, not sign
	require.Equal(t, uint64(5000), d.ErrorBoundNS)
	// the anchor read, not RefTime, whose age is already in RootDispersion
	require.Equal(t, testAsOf.UnixNano(), d.IngressTimeNS)
}

func TestCalculateSHMDataChronyRoundsBoundUp(t *testing.T) {
	cfg := &Config{Chrony: true, Interval: time.Second, MaxDriftRate: 50}
	s := newChronyTestDaemon(t, cfg, stats.NewStats())
	tracking := &chrony.Tracking{
		RefTime:        time.Unix(1755000000, 0),
		RootDispersion: 1.5 * time.Nanosecond.Seconds(),
	}

	d, err := s.calculateSHMDataChrony(tracking, testAsOf, nil)
	require.NoError(t, err)
	require.Equal(t, uint64(2), d.ErrorBoundNS)
}

func TestCalculateSHMDataChronyHoldoverMultiplier(t *testing.T) {
	testCases := []struct {
		name         string
		skewPPM      float64
		maxDriftRate float64
		want         float64
	}{
		{name: "chrony skew above the floor is used", skewPPM: 80, maxDriftRate: 50, want: 80000},
		{name: "optimistic skew is floored at max drift rate", skewPPM: 0.005, maxDriftRate: 50, want: 50000},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{Chrony: true, Interval: time.Second, MaxDriftRate: tc.maxDriftRate}
			s := newChronyTestDaemon(t, cfg, stats.NewStats())
			tracking := &chrony.Tracking{
				RefTime:        time.Unix(1755000000, 0),
				RootDispersion: time.Microsecond.Seconds(),
				SkewPPM:        tc.skewPPM,
			}

			d, err := s.calculateSHMDataChrony(tracking, testAsOf, nil)
			require.NoError(t, err)
			require.Equal(t, tc.want, d.HoldoverMultiplierNS)
		})
	}
}

func TestCalculateSHMDataChronyRejectsBadTracking(t *testing.T) {
	testCases := []struct {
		name     string
		tracking *chrony.Tracking
	}{
		{
			name: "chronyd has no usable source",
			tracking: &chrony.Tracking{
				RefTime:        time.Unix(1755000000, 0),
				RootDispersion: time.Microsecond.Seconds(),
				LeapStatus:     ntp.LeapAlarm,
			},
		},
		{
			name: "chronyd never synchronised, so reference time is unset",
			tracking: &chrony.Tracking{
				RootDispersion: time.Microsecond.Seconds(),
			},
		},
		{
			name: "error bound rounds down to zero",
			tracking: &chrony.Tracking{
				RefTime: time.Unix(1755000000, 0),
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{Chrony: true, Interval: time.Second, MaxDriftRate: 50}
			s := newChronyTestDaemon(t, cfg, stats.NewStats())

			_, err := s.calculateSHMDataChrony(tc.tracking, testAsOf, nil)
			require.ErrorIs(t, err, errCorrectness)
		})
	}
}

func TestDoWorkChrony(t *testing.T) {
	cfg := &Config{Chrony: true, Interval: time.Second, MaxDriftRate: 50}
	st := stats.NewStats()
	s := newChronyTestDaemon(t, cfg, st)
	// seed so the assertion below distinguishes "untouched" from "unregistered"
	for _, k := range []string{"m_ns", "w_ns", "drift_ppb"} {
		st.SetCounter(k, 1)
	}

	tracking := &chrony.Tracking{
		RefTime:           time.Unix(1755000000, 0),
		CurrentCorrection: -1 * time.Microsecond.Seconds(),
		RootDispersion:    2 * time.Microsecond.Seconds(),
		RootDelay:         4 * time.Microsecond.Seconds(),
		LastOffset:        3 * time.Microsecond.Seconds(),
		SkewPPM:           80,
		FreqPPM:           -1.5,
	}
	require.NoError(t, s.doWorkChrony(tracking))

	c := st.Get()
	require.Equal(t, int64(5000), c["error_bound_ns"])
	require.Equal(t, int64(80000), c["holdover_multiplier_ns"])
	require.Equal(t, int64(-1000), c["current_correction_ns"])
	require.Equal(t, int64(2000), c["root_dispersion_ns"])
	require.Equal(t, int64(4000), c["root_delay_ns"])
	require.Equal(t, int64(3000), c["master_offset_ns"])
	require.Equal(t, int64(80000), c["skew_ppb"])
	require.Equal(t, int64(-1500), c["freq_ppb"])
	// RefTime stays observable under its own name
	require.Equal(t, int64(1755000000000000000), c["chrony_ref_time_ns"])
	// nothing derived from the ring buffer is published in chrony mode
	for _, k := range []string{"m_ns", "w_ns", "drift_ppb"} {
		require.Equal(t, int64(1), c[k], k)
	}

	// the counter must agree with what we publish to shm
	stored := s.state.getLastStoredData()
	require.Equal(t, uint64(5000), stored.ErrorBoundNS)
	require.Equal(t, testAsOf.UnixNano(), stored.IngressTimeNS)
	require.Equal(t, stored.IngressTimeNS, c["ingress_time_ns"])
}

// TestChronyServesSHMv2 covers the serving path a client actually reads: a real
// UDP fetch from a fake chronyd, through doWorkChrony, out to the v2 segment.
func TestChronyServesSHMv2(t *testing.T) {
	cfg := &Config{
		Chrony: true, Interval: time.Second,
		MaxDriftRate: 50, EnableDataV2: true,
	}
	require.NoError(t, cfg.EvalAndValidate())
	s := newChronyTestDaemon(t, cfg, stats.NewStats())
	s.DataFetcher = &ChronyFetcher{address: fakeChronyd(t, trackingReply)}

	tracking, err := s.FetchTracking(cfg)
	require.NoError(t, err)
	require.NoError(t, s.doWorkChrony(tracking))

	tmpFile, err := os.CreateTemp("", "fbclock_v2")
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })
	// left open on purpose: populateDataV2 never returns, so closing underneath
	// it would only produce write errors for the rest of the run
	shm, err := fbclock.OpenFBClockShmCustomVer(tmpFile.Name(), 2)
	require.NoError(t, err)
	go s.populateDataV2(shm)

	shmp, err := fbclock.MmapShmpDataV2(shm.File.Fd())
	require.NoError(t, err)
	var got *fbclock.DataV2
	require.Eventually(t, func() bool {
		got, err = fbclock.ReadFBClockDataV2(shmp)
		return err == nil && got.IngressTimeNS != 0
	}, 5*time.Second, 10*time.Millisecond)

	stored := s.state.getLastStoredData()
	require.Equal(t, stored.ErrorBoundNS, got.ErrorBoundNS)
	require.Equal(t, stored.IngressTimeNS, got.IngressTimeNS)
	require.Equal(t, stored.HoldoverMultiplierNS, got.HoldoverMultiplierNS)
	// the anchor a client reads is TAI, the system clock plus the tzdata offset
	require.Equal(t, testAsOf.UnixNano(), got.PHCTimeNS)
	require.Equal(t, testSysTime.UnixNano(), got.SysclockTimeNS)
	require.Equal(t, uint32(unix.CLOCK_REALTIME), got.ClockID)
	// one clock named twice leaves nothing to extrapolate
	require.Zero(t, got.CoefPPB)
	// what the client widens by: phc_time_ns - ingress_time_ns, never negative
	require.GreaterOrEqual(t, got.PHCTimeNS, got.IngressTimeNS)
}

func TestReadClocksChrony(t *testing.T) {
	cfg := &Config{Chrony: true, Interval: time.Second, MaxDriftRate: 50, EnableDataV2: true}
	s := newChronyTestDaemon(t, cfg, stats.NewStats())
	// a PHC read here would panic: chrony mode never populates getPHCAndSysTime
	require.Nil(t, s.getPHCAndSysTime)
	// no anchor at all until doWorkChrony has published the leap records
	_, _, _, _, err := s.readClocks()
	require.ErrorIs(t, err, errNoLeapData)
	s.state.leaps.Store(&testLeaps)

	ref, base, clockID, delay, err := s.readClocks()
	require.NoError(t, err)
	// TAI anchor over a REALTIME base; they differ by a constant so CoefPPB is 0
	require.Equal(t, testAsOf, ref)
	require.Equal(t, testSysTime, base)
	require.Equal(t, uint32(unix.CLOCK_REALTIME), clockID)
	require.Zero(t, delay)
}

func TestReadClocksChronyLeapBoundary(t *testing.T) {
	cfg := &Config{Chrony: true, Interval: time.Second, MaxDriftRate: 50, EnableDataV2: true}
	s := newChronyTestDaemon(t, cfg, stats.NewStats())
	s.state.leaps.Store(&testLeaps)

	// the offset must move with the sampled instant, not with the chrony poll
	for _, tc := range []struct {
		name   string
		now    time.Time
		offset time.Duration
	}{
		{name: "before", now: time.Unix(leap2017-1, 0), offset: 36 * time.Second},
		{name: "at", now: time.Unix(leap2017, 0), offset: 37 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s.getSysTime = func() (time.Time, error) { return tc.now, nil }
			ref, base, _, _, err := s.readClocks()
			require.NoError(t, err)
			require.Equal(t, tc.offset, ref.Sub(base))
		})
	}
}

func TestReadClocksPTPUnaffected(t *testing.T) {
	cfg := &Config{Interval: time.Second, EnableDataV2: true}
	s := newChronyTestDaemon(t, cfg, stats.NewStats())
	phcTime, sysTime := time.Unix(1755000002, 0), time.Unix(1755000003, 0)
	s.getPHCAndSysTime = func() (time.Time, time.Time, uint32, time.Duration, error) {
		return phcTime, sysTime, unix.CLOCK_MONOTONIC_RAW, time.Microsecond, nil
	}

	ref, base, clockID, delay, err := s.readClocks()
	require.NoError(t, err)
	require.Equal(t, phcTime, ref)
	require.Equal(t, sysTime, base)
	require.Equal(t, uint32(unix.CLOCK_MONOTONIC_RAW), clockID)
	require.Equal(t, time.Microsecond, delay)
}

func TestDoWorkChronyUnsyncedRefTime(t *testing.T) {
	cfg := &Config{Chrony: true, Interval: time.Second, MaxDriftRate: 50}
	st := stats.NewStats()
	s := newChronyTestDaemon(t, cfg, st)
	st.SetCounter("chrony_ref_time_ns", 1)

	// before its first sync chronyd sends the zero Time, whose UnixNano is a
	// large negative sentinel rather than 0
	tracking := &chrony.Tracking{RootDispersion: time.Microsecond.Seconds()}
	require.ErrorIs(t, s.doWorkChrony(tracking), errCorrectness)
	require.Equal(t, int64(1), st.Get()["chrony_ref_time_ns"])
}

func TestDoWorkChronySysClockError(t *testing.T) {
	cfg := &Config{Chrony: true, Interval: time.Second, MaxDriftRate: 50}
	s := newChronyTestDaemon(t, cfg, stats.NewStats())
	s.getSysTime = func() (time.Time, error) { return time.Time{}, os.ErrNotExist }

	tracking := &chrony.Tracking{
		RefTime:        time.Unix(1755000000, 0),
		RootDispersion: time.Microsecond.Seconds(),
	}
	// without an anchor reading there is nothing to date the bound against
	require.Error(t, s.doWorkChrony(tracking))
	require.Nil(t, s.state.getLastStoredData())
}

func TestEvalAndValidateChrony(t *testing.T) {
	testCases := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{name: "chrony mode needs no ptp client, ring size or expressions"},
		{name: "zero max drift rate leaves no holdover floor", mutate: func(c *Config) { c.MaxDriftRate = 0 }, wantErr: true},
		{name: "refuses to serve the v1 segment", mutate: func(c *Config) { c.EnableDataV2 = false }, wantErr: true},
		{name: "negative max drift rate", mutate: func(c *Config) { c.MaxDriftRate = -1 }, wantErr: true},
		{name: "interval is still validated", mutate: func(c *Config) { c.Interval = 2 * time.Minute }, wantErr: true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{
				Chrony: true, Interval: time.Second,
				MaxDriftRate: 50, EnableDataV2: true,
			}
			if tc.mutate != nil {
				tc.mutate(&cfg)
			}
			err := cfg.EvalAndValidate()
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
