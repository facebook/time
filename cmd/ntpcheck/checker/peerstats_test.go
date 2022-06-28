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

	"github.com/stretchr/testify/require"

	"github.com/facebook/time/ntp/control"
)

func TestNTPPeerStatsNoSysVars(t *testing.T) {
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
	_, err := NewNTPPeerStats(r)
	require.EqualError(t, err, "no system variables to output stats")
}

func TestNTPPeerStatsNoPeers(t *testing.T) {
	s := SystemVariables{}
	r := &NTPCheckResult{
		SysVars: &s,
		Peers:   map[uint16]*Peer{},
	}
	peerStats, err := NewNTPPeerStats(r)
	require.NoError(t, err)
	want := map[string]any{}
	require.Equal(t, want, peerStats)
}

func TestNTPPeerStatsWithSysPeer(t *testing.T) {
	s := SystemVariables{}
	r := &NTPCheckResult{
		SysVars: &s,
		Peers: map[uint16]*Peer{
			0: &Peer{
				SRCAdr:    "192.168.0.2",
				Selection: control.SelCandidate,
				Offset:    0.01,
				Delay:     2.01,
				Stratum:   3,
				HPoll:     10,
				PPoll:     9,
				Jitter:    3.1,
			},
			1: &Peer{
				SRCAdr:    "192.168.0.3",
				Selection: control.SelSYSPeer,
				Offset:    0.045,
				Delay:     3.21,
				Stratum:   4,
				HPoll:     10,
				PPoll:     4,
				Jitter:    4,
			},
			// no ips, skip
			2: &Peer{
				Selection: control.SelReject,
			},
		},
	}
	peerStats, err := NewNTPPeerStats(r)
	require.NoError(t, err)
	want := map[string]any{
		"ntp.peers.192_168_0_2.delay":   2.01,
		"ntp.peers.192_168_0_2.jitter":  3.1,
		"ntp.peers.192_168_0_2.offset":  0.01,
		"ntp.peers.192_168_0_2.poll":    512,
		"ntp.peers.192_168_0_2.stratum": 3,
		"ntp.peers.192_168_0_3.delay":   3.21,
		"ntp.peers.192_168_0_3.jitter":  4.0,
		"ntp.peers.192_168_0_3.offset":  0.045,
		"ntp.peers.192_168_0_3.poll":    16,
		"ntp.peers.192_168_0_3.stratum": 4,
	}
	require.Equal(t, want, peerStats)
}
