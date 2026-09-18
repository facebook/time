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

	"github.com/facebook/time/ntp/chrony"
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
var (
	refTime       = time.Unix(1587738257, 0)
	replyTracking = &chrony.ReplyTracking{
		Tracking: chrony.Tracking{
			RefID:      123456,
			RefTime:    refTime,
			IPAddr:     net.ParseIP("192.168.0.1"),
			Stratum:    3,
			LeapStatus: 0,
			LastOffset: 0.001,
		},
	}

	replyServerStats = &chrony.ReplyServerStats{
		ServerStats: chrony.ServerStats{
			NTPHits:  1234,
			NTPDrops: 5678,
		},
	}

	replySources = &chrony.ReplySources{
		NSources: 2,
	}

	replySD0 = &chrony.ReplySourceData{
		SourceData: chrony.SourceData{
			IPAddr:         &chrony.IPAddr{IP: chrony.IPToBytes(net.ParseIP("192.168.0.2")), Family: chrony.IPAddrInet4},
			Flags:          chrony.NTPFlagsTests,
			Poll:           10,
			Stratum:        2,
			State:          chrony.SourceStateSync,
			Mode:           chrony.SourceModePeer,
			Reachability:   255,
			OrigLatestMeas: -0.03,
		},
	}

	replySD1 = &chrony.ReplySourceData{
		SourceData: chrony.SourceData{
			IPAddr:         &chrony.IPAddr{IP: chrony.IPToBytes(net.ParseIP("192.168.0.4")), Family: chrony.IPAddrInet4},
			Flags:          chrony.NTPFlagsTests,
			Poll:           11,
			Stratum:        2,
			State:          chrony.SourceStateCandidate,
			Mode:           chrony.SourceModePeer,
			Reachability:   200,
			OrigLatestMeas: -0.02,
		},
	}

	replyNTPData0 = &chrony.ReplyNTPData2{
		NTPData2: chrony.NTPData2{
			NTPData: chrony.NTPData{
				RefID:          123456,
				RefTime:        refTime,
				Leap:           0,
				Poll:           10,
				Offset:         0.03,
				Stratum:        2,
				PeerDispersion: 0.01,
				LocalAddr:      net.ParseIP("192.168.0.1"),
			},
		},
	}
	replyNTPData1 = &chrony.ReplyNTPData2{
		NTPData2: chrony.NTPData2{
			NTPData: chrony.NTPData{
				RefID:          654321,
				RefTime:        refTime,
				Leap:           0,
				Poll:           11,
				Offset:         0.02,
				Stratum:        2,
				PeerDispersion: 0.02,
				LocalAddr:      net.ParseIP("192.168.0.1"),
			},
		},
	}
	replyNTPSourceName0 = &chrony.ReplyNTPSourceName{
		NTPSourceName: chrony.NTPSourceName{
			Name: "ntp_peer001.sample.facebook.com",
		},
	}
	replyNTPSourceName1 = &chrony.ReplyNTPSourceName{
		NTPSourceName: chrony.NTPSourceName{
			Name: "",
		},
	}
)

func TestChronyCheckRun(t *testing.T) {
	prepdOutputs := []chrony.ResponsePacket{
		// tracking
		replyTracking,
		// get list of sources
		replySources,
		// first source
		replySD0,
		replyNTPSourceName0,
		// second source
		replySD1,
		replyNTPSourceName1,
	}

	want := &NTPCheckResult{
		LI:          0,
		LIDesc:      "none",
		ClockSource: chrony.ClockSourceNTP,
		Event:       "clock_sync",
		SysVars: &SystemVariables{
			Stratum: 3,
			RefID:   "0001E240",
			RefTime: refTime.String(),
			Offset:  1,
		},
		Peers: map[uint16]*Peer{
			0: {
				Configured: true,
				Reachable:  true,
				Selection:  6,
				Condition:  "sync",
				SRCAdr:     "192.168.0.2",
				Hostname:   "ntp_peer001.sample.facebook.com",
				Stratum:    2,
				Reach:      255,
				PPoll:      10,
				HPoll:      10,
				Offset:     30,
				Flashers:   []string{},
			},
			1: {
				Configured: true,
				Reachable:  false,
				Selection:  4,
				Condition:  "candidate",
				SRCAdr:     "192.168.0.4",
				Stratum:    2,
				Reach:      200,
				PPoll:      11,
				HPoll:      11,
				Offset:     20,
				Flashers:   []string{},
			},
		},
	}

	check := &ChronyCheck{
		Client: &fakeChronyClient{readCount: 0, outputs: prepdOutputs},
	}

	got, err := check.Run()
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestChronyCheckRunUnix(t *testing.T) {
	prepdOutputs := []chrony.ResponsePacket{
		// tracking
		replyTracking,
		// get list of sources
		replySources,
		// first source
		replySD0,
		replyNTPData0,
		replyNTPSourceName0,
		// second source
		replySD1,
		replyNTPData1,
		replyNTPSourceName1,
	}

	want := &NTPCheckResult{
		LI:          0,
		LIDesc:      "none",
		ClockSource: chrony.ClockSourceNTP,
		Event:       "clock_sync",
		SysVars: &SystemVariables{
			Stratum: 3,
			RefID:   "0001E240",
			RefTime: refTime.String(),
			Offset:  1,
		},
		Peers: map[uint16]*Peer{
			0: {
				RefID:      "0001E240",
				RefTime:    refTime.String(),
				Configured: true,
				Reachable:  true,
				Selection:  6,
				Condition:  "sync",
				SRCAdr:     "192.168.0.2",
				DSTAdr:     "192.168.0.1",
				Hostname:   "ntp_peer001.sample.facebook.com",
				Stratum:    2,
				Reach:      255,
				PPoll:      10,
				HPoll:      10,
				Offset:     30,
				Dispersion: 10,
				Jitter:     10,
				Flashers:   []string{},
			},
			1: {
				RefID:      "0009FBF1",
				RefTime:    refTime.String(),
				Configured: true,
				Reachable:  false,
				Selection:  4,
				Condition:  "candidate",
				SRCAdr:     "192.168.0.4",
				DSTAdr:     "192.168.0.1",
				Stratum:    2,
				Reach:      200,
				PPoll:      11,
				HPoll:      11,
				Offset:     20,
				Dispersion: 20,
				Jitter:     20,
				Flashers:   []string{},
			},
		},
	}

	check := &ChronyCheck{
		Client:   &fakeChronyClient{readCount: 0, outputs: prepdOutputs},
		unixConn: true,
	}

	got, err := check.Run()
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestChronyCheckServerStats(t *testing.T) {
	prepdOutputs := []chrony.ResponsePacket{
		// server stats
		replyServerStats,
	}

	want := &ServerStats{
		PacketsReceived: 1234,
		PacketsDropped:  5678,
	}

	check := &ChronyCheck{
		Client: &fakeChronyClient{readCount: 0, outputs: prepdOutputs},
	}

	got, err := check.ServerStats()
	require.NoError(t, err)
	require.Equal(t, want, got)
}
