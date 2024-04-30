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
	"testing"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	gmstats "github.com/facebook/time/ptp/sptp/stats"

	"github.com/stretchr/testify/require"
)

func TestRunResultToStatsError(t *testing.T) {
	r := &RunResult{
		Server: "192.168.0.10",
		Error:  fmt.Errorf("ooops"),
	}
	got := runResultToStats("192.168.0.10", r, 1, false)
	want := &gmstats.Stat{
		GMAddress: "192.168.0.10",
		Priority3: 1,
		Error:     "ooops",
	}
	require.Equal(t, want, got)
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
		Server: "192.168.0.10",
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
		got := runResultToStats("192.168.0.10", r, 3, false)
		require.Equal(t, want, got)
	})
	want.Selected = true
	t.Run("selected", func(t *testing.T) {
		got := runResultToStats("192.168.0.10", r, 3, true)
		require.Equal(t, want, got)
	})
}

func TestSetGMStats(t *testing.T) {
	gm := &gmstats.Stat{
		GMAddress: "192.168.0.10",
		Error:     "mymy",
	}
	s := NewStats()
	s.SetGMStats(gm)
	want := gmstats.Stats{
		gm,
	}
	require.Equal(t, want, s.GetGMStats())
}

func TestInc(t *testing.T) {
	s := NewStats()
	s.rxAnnounce = 42
	s.rxSync = 43
	s.rxDelayReq = 44
	s.txDelayReq = 45
	s.unsupported = 46
	s.IncRXAnnounce()
	s.IncRXSync()
	s.IncRXDelayReq()
	s.IncTXDelayReq()
	s.IncUnsupported()
	require.Equal(t, int64(43), s.rxAnnounce)
	require.Equal(t, int64(44), s.rxSync)
	require.Equal(t, int64(45), s.rxDelayReq)
	require.Equal(t, int64(46), s.txDelayReq)
	require.Equal(t, int64(47), s.unsupported)
}

func TestSysStats(t *testing.T) {
	stats := NewStats()
	time.Sleep(time.Second)
	err := stats.CollectSysStats()
	require.NoError(t, err)
	// Sys counters are set and above 0
	require.Less(t, int64(0), stats.goRoutines)
	require.Less(t, int64(0), stats.rss)
	require.Equal(t, int64(1), stats.uptimeSec)
}

func TestGetCounters(t *testing.T) {
	stats := NewStats()
	m := stats.GetCounters()
	require.Contains(t, m, "ptp.sptp.gms.total")
	require.Contains(t, m, "ptp.sptp.gms.available_pct")
	require.Contains(t, m, "ptp.sptp.tick_duration_ns")
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
