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
	"encoding/binary"
	"fmt"
	"net/netip"
	"sync"
	"testing"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	gmstats "github.com/facebook/time/ptp/sptp/stats"
	"github.com/facebook/time/servo"

	"github.com/stretchr/testify/require"
)

func TestRunResultToStatsError(t *testing.T) {
	r := &RunResult{
		Server: netip.MustParseAddr("192.168.0.10"),
		Error:  fmt.Errorf("ooops"),
	}
	want := &gmstats.Stat{
		GMAddress: "192.168.0.10",
		Priority3: 1,
		Error:     "ooops",
	}

	t.Run("not selected", func(t *testing.T) {
		got := runResultToGMStats(netip.MustParseAddr("192.168.0.10"), r, 1, false, 0)
		require.Equal(t, want, got)
	})

	// A grandmaster we could not reach must never be published as selected,
	// whatever the caller passes.
	t.Run("selected", func(t *testing.T) {
		got := runResultToGMStats(netip.MustParseAddr("192.168.0.10"), r, 1, true, int(servo.StateFilter))
		require.Equal(t, want, got)
	})
}

func TestRunResultToStatsNoMeasurement(t *testing.T) {
	r := &RunResult{Server: netip.MustParseAddr("192.168.0.10")}
	want := &gmstats.Stat{
		GMAddress: "192.168.0.10",
		Priority3: 3,
		Error:     "Measurement is missing on RunResult",
	}

	t.Run("not selected", func(t *testing.T) {
		got := runResultToGMStats(netip.MustParseAddr("192.168.0.10"), r, 3, false, int(servo.StateFilter))
		require.Equal(t, want, got)
	})

	want.Selected = true
	want.ServoState = int(servo.StateFilter)
	t.Run("selected", func(t *testing.T) {
		got := runResultToGMStats(netip.MustParseAddr("192.168.0.10"), r, 3, true, int(servo.StateFilter))
		require.Equal(t, want, got)
	})
}

func TestRunResultToStats(t *testing.T) {
	statsAnnouncePkt := ptp.Announce{
		Header: ptp.Header{
			SdoIDAndMsgType:    ptp.NewSdoIDAndMsgType(ptp.MessageAnnounce, 0),
			Version:            ptp.Version,
			SequenceID:         123,
			MessageLength:      uint16(binary.Size(ptp.Header{}) + binary.Size(ptp.AnnounceBody{})),
			FlagField:          ptp.FlagUnicast,
			LogMessageInterval: 0x7f,
		},
		AnnounceBody: ptp.AnnounceBody{
			OriginTimestamp:      ptp.NewTimestamp(time.Now()),
			GrandmasterPriority1: 1,
			GrandmasterPriority2: 2,
			GrandmasterIdentity:  2248787489,
			GrandmasterClockQuality: ptp.ClockQuality{
				ClockClass:              ptp.ClockClass6,
				ClockAccuracy:           ptp.ClockAccuracyMicrosecond250,
				OffsetScaledLogVariance: 4,
			},
		},
	}
	ts, err := time.Parse(time.RFC3339, "2021-05-21T13:32:05+01:00")
	require.Nil(t, err)
	r := &RunResult{
		Server: netip.MustParseAddr("192.168.0.10"),
		Measurement: &MeasurementResult{
			Delay:             299995 * time.Microsecond,
			S2CDelay:          10 * time.Microsecond,
			C2SDelay:          11 * time.Microsecond,
			Offset:            -100001 * time.Microsecond,
			CorrectionFieldRX: 6 * time.Microsecond,
			CorrectionFieldTX: 4 * time.Microsecond,
			Timestamp:         ts,
			Announce:          statsAnnouncePkt,
		},
	}

	want := &gmstats.Stat{
		GMAddress:         "192.168.0.10",
		ClockQuality:      statsAnnouncePkt.GrandmasterClockQuality,
		Error:             "",
		GMPresent:         1,
		IngressTime:       ts.UnixNano(),
		MeanPathDelay:     float64(299995 * time.Microsecond),
		Offset:            float64(-100001 * time.Microsecond),
		PortIdentity:      "000000.0086.09c621",
		Priority1:         1,
		Priority2:         2,
		Priority3:         3,
		Selected:          false,
		StepsRemoved:      1,
		CorrectionFieldRX: int64(6 * time.Microsecond),
		CorrectionFieldTX: int64(4 * time.Microsecond),
		S2CDelay:          10000,
		C2SDelay:          11000,
	}

	t.Run("not selected", func(t *testing.T) {
		got := runResultToGMStats(netip.MustParseAddr("192.168.0.10"), r, 3, false, 2)
		require.Equal(t, want, got)
	})
	want.Selected = true
	want.ServoState = 2
	t.Run("selected", func(t *testing.T) {
		got := runResultToGMStats(netip.MustParseAddr("192.168.0.10"), r, 3, true, 2)
		require.Equal(t, want, got)
	})
}

func TestSetGMStats(t *testing.T) {
	gm := &gmstats.Stat{
		GMAddress: "192.168.0.10",
		Error:     "mymy",
	}
	s, err := NewStats()
	require.NoError(t, err)
	s.SetGMStats(gm)
	want := gmstats.Stats{
		gm,
	}
	require.Equal(t, want, s.GetGMStats())
}

func TestInc(t *testing.T) {
	s, err := NewStats()
	require.NoError(t, err)
	s.rxAnnounce.Store(42)
	s.rxSync.Store(43)
	s.rxDelayReq.Store(44)
	s.txDelayReq.Store(45)
	s.unsupported.Store(46)
	s.filtered.Store(47)
	s.IncRXAnnounce()
	s.IncRXSync()
	s.IncRXDelayReq()
	s.IncTXDelayReq()
	s.IncUnsupported()
	s.IncFiltered()
	require.Equal(t, int64(43), s.rxAnnounce.Load())
	require.Equal(t, int64(44), s.rxSync.Load())
	require.Equal(t, int64(45), s.rxDelayReq.Load())
	require.Equal(t, int64(46), s.txDelayReq.Load())
	require.Equal(t, int64(47), s.unsupported.Load())
	require.Equal(t, int64(48), s.filtered.Load())
}

func TestSysStats(t *testing.T) {
	stats, err := NewStats()
	require.NoError(t, err)
	time.Sleep(time.Second)
	stats.CollectSysStats()
	// Sys counters are set and above 0
	require.Less(t, int64(0), stats.goRoutines)
	require.Less(t, int64(0), stats.rss)
	require.LessOrEqual(t, int64(1), stats.uptimeSec)
}

func TestGetCounters(t *testing.T) {
	stats, err := NewStats()
	require.NoError(t, err)
	m := stats.GetCounters()
	require.Contains(t, m, "ptp.sptp.gms.total")
	require.Contains(t, m, "ptp.sptp.gms.available_pct")
	require.Contains(t, m, "ptp.sptp.filtered")
	require.Contains(t, m, "ptp.sptp.portstats.rx.sync")
	require.Contains(t, m, "ptp.sptp.portstats.rx.announce")
	require.Contains(t, m, "ptp.sptp.portstats.rx.delay_req")
	require.Contains(t, m, "ptp.sptp.portstats.tx.delay_req")
	require.Contains(t, m, "ptp.sptp.portstats.rx.unsupported")
	require.Contains(t, m, "ptp.sptp.runtime.gc.pause_ns.sum.60")
	require.Contains(t, m, "ptp.sptp.runtime.mem.gc.pause_total_ns")
	require.Contains(t, m, "ptp.sptp.runtime.cpu.goroutines")
	require.Contains(t, m, "ptp.sptp.process.rss")
	require.Contains(t, m, "ptp.sptp.process.cpu_pct.avg.60")
	require.Contains(t, m, "ptp.sptp.process.uptime")
}

func TestGetCountersDuringCounterUpdates(t *testing.T) {
	s, err := NewStats()
	require.NoError(t, err)

	const updates = 2000
	const sysCollections = 5
	writersDone := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		defer close(writersDone)
		for i := range updates {
			s.SetGmsTotal(i)
			s.SetGmsAvailable(i)
			s.SetServoState(i)
			s.IncFiltered()
			s.IncRXSync()
			s.IncRXAnnounce()
			s.IncRXDelayReq()
			s.IncTXDelayReq()
			s.IncUnsupported()
			s.IncPortChangeCount(1)
		}
	})
	wg.Go(func() {
		for range sysCollections {
			s.CollectSysStats()
		}
	})
	// Read for as long as the writers run, so the overlap does not depend on scheduling.
	wg.Go(func() {
		for {
			s.GetCounters()
			select {
			case <-writersDone:
				return
			default:
			}
		}
	})
	wg.Wait()

	got := s.GetCounters()
	require.Equal(t, int64(updates-1), got["ptp.sptp.gms.total"])
	require.Equal(t, int64(updates-1), got["ptp.sptp.gms.available_pct"])
	require.Equal(t, int64(updates-1), got["ptp.sptp.servo.state"])
	require.Equal(t, int64(updates), got["ptp.sptp.filtered"])
	require.Equal(t, int64(updates), got["ptp.sptp.portstats.rx.sync"])
	require.Equal(t, int64(updates), got["ptp.sptp.portstats.rx.announce"])
	require.Equal(t, int64(updates), got["ptp.sptp.portstats.rx.delay_req"])
	require.Equal(t, int64(updates), got["ptp.sptp.portstats.tx.delay_req"])
	require.Equal(t, int64(updates), got["ptp.sptp.portstats.rx.unsupported"])
	require.Equal(t, int64(updates), got["ptp.sptp.port_change_count"])
}
