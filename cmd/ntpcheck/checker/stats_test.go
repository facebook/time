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

package checker

import (
	"testing"

	"github.com/facebook/time/ntp/control"
	"github.com/stretchr/testify/require"
)

func TestNTPStatsNoSysVars(t *testing.T) {
	// Check that no sysvars triggers exit code
	r := &NTPCheckResult{
		SysVars: nil,
		Peers: map[uint16]*Peer{
			0: {
				Selection: control.SelCandidate,
			},
			1: {
				Selection: control.SelBackup,
			},
		},
	}
	_, err := NewNTPStats(r)
	require.EqualError(t, err, "no system variables to output stats")
}

func TestNTPStatsNoPeers(t *testing.T) {
	s := SystemVariables{}
	r := &NTPCheckResult{
		SysVars: &s,
		Peers:   map[uint16]*Peer{},
	}
	_, err := NewNTPStats(r)
	require.EqualError(t, err, "nothing to calculate stats from: no peers present")
}

func TestNTPStatsNoGoodPeer(t *testing.T) {
	// Check that no "good" pier triggers exit code
	s := SystemVariables{}
	r := &NTPCheckResult{
		SysVars: &s,
		Peers: map[uint16]*Peer{
			0: {},
		},
	}
	_, err := NewNTPStats(r)
	require.EqualError(t, err, "nothing to calculate stats from: no good peers present")
}

func TestNTPStatsNoSysPeer(t *testing.T) {
	s := SystemVariables{
		Offset:    0.003,
		RootDelay: 3.14,
	}
	r := &NTPCheckResult{
		SysVars: &s,
		Peers: map[uint16]*Peer{
			0: {
				Selection: control.SelCandidate,
				Offset:    0.01,
				Delay:     2.01,
				Stratum:   3,
				HPoll:     10,
				PPoll:     9,
				Jitter:    3.1,
			},
			1: {
				Selection: control.SelBackup,
				Offset:    0.040,
				Delay:     3.21,
				Stratum:   4,
				HPoll:     10,
				PPoll:     4,
				Jitter:    4,
			},
		},
	}
	stats, err := NewNTPStats(r)
	require.NoError(t, err)
	want := &NTPStats{
		PeerDelay:             2.61,
		PeerOffset:            0.025,
		PeerPoll:              1 << 4,
		PeerStratum:           3,
		PeerJitter:            3.55,
		PeerCount:             2,
		Offset:                s.Offset,
		RootDelay:             s.RootDelay,
		OffsetComparedToPeers: 0.015,
	}
	require.Equal(t, want, stats)
}

func TestNTPStatsNoSysPeerMeasuresPeerSetSkew(t *testing.T) {
	peers := map[uint16]*Peer{}
	id := uint16(0)
	for _, offset := range []float64{-400, 100, 100} {
		peers[id] = &Peer{
			Selection: control.SelCandidate,
			Offset:    offset,
			Stratum:   3,
			HPoll:     10,
			PPoll:     4,
		}
		id++
	}
	stats, err := NewNTPStats(&NTPCheckResult{SysVars: &SystemVariables{}, Peers: peers})
	require.NoError(t, err)
	require.InDelta(t, -200.0/3, stats.PeerOffset, 1e-9)
	require.InDelta(t, 500.0/3, stats.OffsetComparedToPeers, 1e-9)
}

func TestNTPStatsNoSysPeerIgnoresNoSelectControls(t *testing.T) {
	peers := map[uint16]*Peer{
		0: {Selection: control.SelCandidate, Offset: 10, Stratum: 3, HPoll: 10, PPoll: 4},
		1: {Selection: control.SelCandidate, Offset: 20, Stratum: 3, HPoll: 10, PPoll: 4},
		2: {Selection: control.SelReject, NoSelect: true, Offset: 1000, Stratum: 1, HPoll: 10, PPoll: 4},
		3: {Selection: control.SelReject, NoSelect: true, Offset: 1000, Stratum: 1, HPoll: 10, PPoll: 4},
	}
	stats, err := NewNTPStats(&NTPCheckResult{SysVars: &SystemVariables{}, Peers: peers})
	require.NoError(t, err)
	require.InDelta(t, 15.0, stats.PeerOffset, 1e-9)
	// FindAcceptableNonSysPeers only falls back to the noselect controls when there are
	// fewer than two good peers, so 985 (the distance to the controls) is never reported here.
	require.InDelta(t, 5.0, stats.OffsetComparedToPeers, 1e-9)
}

func TestNTPStatsSysPeerWithoutComparisonPeers(t *testing.T) {
	peers := map[uint16]*Peer{
		0: {Selection: control.SelSYSPeer, Offset: 0.045, Stratum: 1, HPoll: 10, PPoll: 4},
	}
	stats, err := NewNTPStats(&NTPCheckResult{SysVars: &SystemVariables{}, Peers: peers})
	require.NoError(t, err)
	require.Zero(t, stats.OffsetComparedToPeers)
}

func TestNTPStatsWithSysPeer(t *testing.T) {
	s := SystemVariables{
		Offset:    0.003,
		RootDelay: 3.14,
	}
	r := &NTPCheckResult{
		SysVars: &s,
		Peers: map[uint16]*Peer{
			0: {
				Selection: control.SelCandidate,
				Offset:    0.01,
				Delay:     2.01,
				Stratum:   3,
				HPoll:     10,
				PPoll:     9,
				Jitter:    3.1,
			},
			1: {
				Selection: control.SelSYSPeer,
				Offset:    0.045,
				Delay:     3.21,
				Stratum:   4,
				HPoll:     10,
				PPoll:     4,
				Jitter:    4,
			},
		},
	}
	stats, err := NewNTPStats(r)
	require.NoError(t, err)
	want := &NTPStats{
		PeerDelay:             3.21,
		PeerOffset:            0.045,
		PeerPoll:              1 << 4,
		PeerStratum:           4,
		PeerJitter:            4,
		PeerCount:             2,
		Offset:                s.Offset,
		RootDelay:             s.RootDelay,
		OffsetComparedToPeers: r.Peers[1].Offset - r.Peers[0].Offset,
	}
	require.Equal(t, want, stats)
}

func TestNTPStatsWithSysPeerAndNoSelect(t *testing.T) {
	s := SystemVariables{
		Offset:    0.003,
		RootDelay: 3.14,
	}
	r := &NTPCheckResult{
		SysVars: &s,
		Peers: map[uint16]*Peer{
			0: {
				Selection: control.SelReject,
				Offset:    0.03,
				Delay:     2.01,
				Stratum:   3,
				HPoll:     10,
				PPoll:     9,
				Jitter:    3.1,
				NoSelect:  true,
			},
			1: {
				Selection: control.SelSYSPeer,
				Offset:    0.045,
				Delay:     3.21,
				Stratum:   1,
				HPoll:     10,
				PPoll:     4,
				Jitter:    4,
			},
		},
	}
	stats, err := NewNTPStats(r)
	require.NoError(t, err)
	want := &NTPStats{
		PeerDelay:             3.21,
		PeerOffset:            0.045,
		PeerPoll:              1 << 4,
		PeerStratum:           1,
		PeerJitter:            4,
		PeerCount:             2,
		Offset:                s.Offset,
		RootDelay:             s.RootDelay,
		OffsetComparedToPeers: r.Peers[1].Offset - r.Peers[0].Offset,
	}
	require.Equal(t, want, stats)
}

func TestNTPStatsOffsetComparedToPeers(t *testing.T) {
	tests := []struct {
		name        string
		sysOffset   float64
		peerOffsets []float64
		want        float64
	}{
		{
			name:        "same sign, peers near sys.peer, unchanged by sign handling",
			sysOffset:   0.045,
			peerOffsets: []float64{0.030, 0.040, 0.050},
			want:        0.005,
		},
		{
			name:        "sys.peer pinned to its refclock while peers see a stepped clock, unchanged by sign handling",
			sysOffset:   0,
			peerOffsets: []float64{-42000, -41000, -40000},
			want:        41000,
		},
		{
			name:        "even peer count: medianOffset's existing tie-break picks the upper middle offset",
			sysOffset:   -60,
			peerOffsets: []float64{-61, 59},
			want:        119,
		},
		{
			name:        "peers drift the opposite way from sys.peer",
			sysOffset:   -0.2,
			peerOffsets: []float64{0.3, 0.4, 0.5},
			want:        0.6,
		},
		{
			name:        "sys.peer and peers disagree symmetrically",
			sysOffset:   600,
			peerOffsets: []float64{-610, -600, -590},
			want:        1200,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peers := map[uint16]*Peer{
				0: {Selection: control.SelSYSPeer, Offset: tt.sysOffset, Stratum: 1, HPoll: 10, PPoll: 4},
			}
			id := uint16(1)
			for _, offset := range tt.peerOffsets {
				peers[id] = &Peer{
					Selection: control.SelReject,
					NoSelect:  true,
					Offset:    offset,
					Stratum:   1,
					HPoll:     10,
					PPoll:     4,
				}
				id++
			}
			stats, err := NewNTPStats(&NTPCheckResult{SysVars: &SystemVariables{}, Peers: peers})
			require.NoError(t, err)
			require.InDelta(t, tt.want, stats.OffsetComparedToPeers, 1e-9)
		})
	}
}

func TestMedianOffset(t *testing.T) {
	peers := []*Peer{
		0: {Offset: 1},
		1: {Offset: 2},
		2: {Offset: 3},
		3: {Offset: 4},
		4: {Offset: 5},
		5: {Offset: 6},
		6: {Offset: 100500},
	}
	medianOffset := medianOffset(peers)
	require.Equal(t, float64(4), medianOffset)
}
