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
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/facebookincubator/ntp/protocol/chrony"
	"github.com/facebookincubator/ntp/protocol/control"
)

type fakeChronyClient struct {
	readCount int
	outputs   []chrony.ResponsePacket
}

func (c *fakeChronyClient) Communicate(packet chrony.RequestPacket) (chrony.ResponsePacket, error) {
	pos := c.readCount
	if c.readCount < len(c.outputs) {
		c.readCount++
		return c.outputs[pos], nil
	}
	return nil, fmt.Errorf("EOF")
}

// some shared vars
var refTime = time.Unix(1587738257, 0)
var replyTracking = &chrony.ReplyTracking{
	Tracking: chrony.Tracking{
		RefID:      123456,
		RefTime:    refTime,
		IPAddr:     net.ParseIP("192.168.0.1"),
		Stratum:    3,
		LeapStatus: 0,
		RMSOffset:  0.001,
	},
}

var replyServerStats = &chrony.ReplyServerStats{
	ServerStats: chrony.ServerStats{
		NTPHits:  1234,
		NTPDrops: 5678,
	},
}

var replySources = &chrony.ReplySources{
	NSources: 2,
}

var replySD0 = &chrony.ReplySourceData{
	SourceData: chrony.SourceData{
		IPAddr:       net.ParseIP("192.168.0.2"),
		Flags:        chrony.NTPFlagsTests,
		Poll:         10,
		Stratum:      2,
		State:        chrony.SourceStateSync,
		Mode:         chrony.SourceModePeer,
		Reachability: 255,
	},
}

var replyND0 = &chrony.ReplyNTPData{
	NTPData: chrony.NTPData{
		Poll:   10,
		Offset: 0.03,
	},
}
var replySD1 = &chrony.ReplySourceData{
	SourceData: chrony.SourceData{
		IPAddr:       net.ParseIP("192.168.0.4"),
		Flags:        chrony.NTPFlagsTests,
		Poll:         11,
		Stratum:      2,
		State:        chrony.SourceStateCandidate,
		Mode:         chrony.SourceModePeer,
		Reachability: 200,
	},
}
var replyND1 = &chrony.ReplyNTPData{
	NTPData: chrony.NTPData{
		Poll:   11,
		Offset: 0.02,
	},
}

// run over TCP/IP, no ReplySourceData availabe
func TestChronyCheck_RunDegraded(t *testing.T) {
	prepdOutputs := []chrony.ResponsePacket{
		// tracking
		replyTracking,
		// server stats
		replyServerStats,
		// get list of sources
		replySources,
		// first source
		replySD0,
		&chrony.ReplyNTPData{},
		// second source
		replySD1,
		&chrony.ReplyNTPData{},
	}

	want := &NTPCheckResult{
		LI:          0,
		LIDesc:      "none",
		ClockSource: "ntp",
		Event:       "clock_sync",
		SysVars: &SystemVariables{
			Stratum: 3,
			Offset:  1,
			RefID:   "0001E240",
			RefTime: refTime.String(),
		},
		Peers: map[uint16]*Peer{
			0: &Peer{
				Configured: true,
				Reachable:  true,
				Selection:  control.SelSYSPeer,
				Condition:  "sync",
				SRCAdr:     "192.168.0.2",
				DSTAdr:     "<nil>",
				Stratum:    2,
				RefID:      "00000000",
				RefTime:    "0001-01-01 00:00:00 +0000 UTC",
				Reach:      255,
				Flashers:   []string{},
			},
			1: &Peer{
				Configured: true,
				Reachable:  false,
				Selection:  control.SelCandidate,
				Condition:  "candidate",
				SRCAdr:     "192.168.0.4",
				DSTAdr:     "<nil>",
				Stratum:    2,
				RefID:      "00000000",
				RefTime:    "0001-01-01 00:00:00 +0000 UTC",
				Reach:      200,
				Flashers:   []string{},
			},
		},
		ServerStats: &ServerStats{
			PacketsReceived: 1234,
			PacketsDropped:  5678,
		},
	}

	check := &ChronyCheck{
		Client: &fakeChronyClient{readCount: 0, outputs: prepdOutputs},
	}

	got, err := check.Run()
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestChronyCheck_Run(t *testing.T) {
	prepdOutputs := []chrony.ResponsePacket{
		// tracking
		replyTracking,
		// server stats
		replyServerStats,
		// get list of sources
		replySources,
		// first source
		replySD0,
		replyND0,
		// second source
		replySD1,
		replyND1,
	}

	want := &NTPCheckResult{
		LI:          0,
		LIDesc:      "none",
		ClockSource: "ntp",
		Event:       "clock_sync",
		SysVars: &SystemVariables{
			Stratum: 3,
			RefID:   "0001E240",
			RefTime: refTime.String(),
			Offset:  1,
		},
		Peers: map[uint16]*Peer{
			0: &Peer{
				Configured: true,
				Reachable:  true,
				Selection:  6,
				Condition:  "sync",
				SRCAdr:     "192.168.0.2",
				DSTAdr:     "<nil>",
				Stratum:    2,
				RefID:      "00000000",
				RefTime:    "0001-01-01 00:00:00 +0000 UTC",
				Reach:      255,
				PPoll:      10,
				HPoll:      10,
				Offset:     30,
				Flashers:   []string{},
			},
			1: &Peer{
				Configured: true,
				Reachable:  false,
				Selection:  4,
				Condition:  "candidate",
				SRCAdr:     "192.168.0.4",
				DSTAdr:     "<nil>",
				Stratum:    2,
				RefID:      "00000000",
				RefTime:    "0001-01-01 00:00:00 +0000 UTC",
				Reach:      200,
				PPoll:      11,
				HPoll:      11,
				Offset:     20,
				Flashers:   []string{},
			},
		},
		ServerStats: &ServerStats{
			PacketsReceived: 1234,
			PacketsDropped:  5678,
		},
	}

	check := &ChronyCheck{
		Client: &fakeChronyClient{readCount: 0, outputs: prepdOutputs},
	}

	got, err := check.Run()
	require.NoError(t, err)
	require.Equal(t, want, got)
}
