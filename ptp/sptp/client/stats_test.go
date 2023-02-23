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

func TestStatsReset(t *testing.T) {
	stats := NewStats()

	stats.SetCounter("some.counter", 123)
	got := stats.Get()
	want := map[string]int64{
		"some.counter": 123,
	}
	require.Equal(t, want, got)
	stats.Reset()
	got = stats.Get()
	want = map[string]int64{
		"some.counter": 0,
	}
	require.Equal(t, want, got)
}

func TestRunResultToStatsError(t *testing.T) {
	r := &RunResult{
		Server: "192.168.0.10",
		Error:  fmt.Errorf("ooops"),
	}
	got := runResultToStats(r, 1, false)
	want := &gmstats.Stats{
		Error: "ooops",
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
			Delay:              299995 * time.Microsecond,
			ServerToClientDiff: 10 * time.Microsecond,
			ClientToServerDiff: 11 * time.Microsecond,
			Offset:             -100001 * time.Microsecond,
			CorrectionFieldRX:  6 * time.Microsecond,
			CorrectionFieldTX:  4 * time.Microsecond,
			Timestamp:          ts,
			Announce:           statsAnnouncePkt,
		},
	}

	want := &gmstats.Stats{
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
	}

	t.Run("not selected", func(t *testing.T) {
		got := runResultToStats(r, 3, false)
		require.Equal(t, want, got)
	})
	want.Selected = true
	t.Run("selected", func(t *testing.T) {
		got := runResultToStats(r, 3, true)
		require.Equal(t, want, got)
	})
}

func TestSetGMStats(t *testing.T) {
	gms := &gmstats.Stats{
		Error: "mymy",
	}
	s := NewStats()
	s.SetGMStats("192.168.0.10", gms)
	want := map[string]*gmstats.Stats{
		"192.168.0.10": gms,
	}
	require.Equal(t, want, s.gmStats)
}
