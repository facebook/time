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
			0: &Peer{
				Selection: control.SelCandidate,
			},
			1: &Peer{
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
	require.EqualError(t, err, "no peers detected to output stats")
}

func TestNTPStatsNoGoodPeer(t *testing.T) {
	// Check that no "good" pier triggers exit code
	s := SystemVariables{}
	r := &NTPCheckResult{
		SysVars: &s,
		Peers: map[uint16]*Peer{
			0: &Peer{},
		},
	}
	_, err := NewNTPStats(r)
	require.EqualError(t, err, "nothing to calculate stats from: no good peers present")
}

func TestNTPStatsNoSysPeer(t *testing.T) {
	s := SystemVariables{}
	r := &NTPCheckResult{
		SysVars: &s,
		Peers: map[uint16]*Peer{
			0: &Peer{
				Selection: control.SelCandidate,
				Offset:    0.01,
				Delay:     2.01,
				Stratum:   3,
				HPoll:     10,
				PPoll:     9,
				Jitter:    3.1,
			},
			1: &Peer{
				Selection: control.SelBackup,
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
		PeerDelay:   2.61,
		PeerOffset:  0.0275,
		PeerPoll:    1 << 4,
		PeerStratum: 3,
		PeerJitter:  3.55,
		PeerCount:   2,
	}
	require.Equal(t, want, stats)
}

func TestNTPStatsWithSysPeer(t *testing.T) {
	s := SystemVariables{}
	r := &NTPCheckResult{
		SysVars: &s,
		Peers: map[uint16]*Peer{
			0: &Peer{
				Selection: control.SelCandidate,
				Offset:    0.01,
				Delay:     2.01,
				Stratum:   3,
				HPoll:     10,
				PPoll:     9,
				Jitter:    3.1,
			},
			1: &Peer{
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
		PeerDelay:   3.21,
		PeerOffset:  0.045,
		PeerPoll:    1 << 4,
		PeerStratum: 4,
		PeerJitter:  4,
		PeerCount:   2,
	}
	require.Equal(t, want, stats)
}
