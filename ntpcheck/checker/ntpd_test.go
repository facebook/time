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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/facebookincubator/ntp/protocol/control"
)

type fakeNTPClient struct {
	readCount int
	outputs   []*control.NTPControlMsg
}

func (c *fakeNTPClient) Communicate(packet *control.NTPControlMsgHead) (*control.NTPControlMsg, error) {
	pos := c.readCount
	if c.readCount < len(c.outputs) {
		c.readCount++
		return c.outputs[pos], nil
	}
	return nil, fmt.Errorf("EOF")
}

func (c *fakeNTPClient) CommunicateWithData(packet *control.NTPControlMsgHead, data []uint8) (*control.NTPControlMsg, error) {
	return c.Communicate(packet)
}

func uint16to2x8(d uint16) []uint8 {
	return []uint8{uint8((d & 65280) >> 8), uint8(d & 255)}
}

func assocIDpair(id, psWord uint16) []uint8 {
	return append(uint16to2x8(id), uint16to2x8(psWord)...)
}

var psWord = &control.PeerStatusWord{
	PeerStatus: control.PeerStatus{
		Broadcast:   false,
		Reachable:   true,
		AuthEnabled: false,
		AuthOK:      false,
		Configured:  true,
	},
	PeerSelection:    control.SelSYSPeer,
	PeerEventCounter: 1,
	PeerEventCode:    2,
}

var psWordBinary = psWord.Word()

var assocData = assocIDpair(2, psWordBinary)

func TestNTPCheck_Run(t *testing.T) {
	prepdOutputs := []*control.NTPControlMsg{
		// system status
		&control.NTPControlMsg{
			NTPControlMsgHead: control.NTPControlMsgHead{
				VnMode: vnMode,
				REMOp:  control.MakeREMOp(true, false, false, control.OpReadStatus),
				Status: (&control.SystemStatusWord{
					LI:                 0, // add_sec
					ClockSource:        6, // ntp
					SystemEventCounter: 0,
					SystemEventCode:    5, // clock_sync
				}).Word(),
				Count: uint16(len(assocData)),
			},
			Data: assocData,
		},
		// read system variables
		&control.NTPControlMsg{
			NTPControlMsgHead: control.NTPControlMsgHead{
				VnMode:        vnMode,
				REMOp:         control.MakeREMOp(true, false, false, control.OpReadVariables),
				AssociationID: 0,
			},
			Data: []uint8("stratum=3,offset=0.1,hpoll=1024,ppoll=10,refid=0001E240,reftime=0x01"),
		},
		// read peer variables
		&control.NTPControlMsg{
			NTPControlMsgHead: control.NTPControlMsgHead{
				VnMode:        vnMode,
				REMOp:         control.MakeREMOp(true, false, false, control.OpReadVariables),
				AssociationID: 2,
				Status:        psWordBinary,
			},
			Data: []uint8("reach=255,srcadr=192.168.0.4,dstadr=10.3.2.4,stratum=2,offset=20,hpoll=11,ppoll=11,refid=20012210,reftime=0x02"),
		},
	}

	want := &NTPCheckResult{
		LI:          0,
		LIDesc:      "none",
		ClockSource: "ntp",
		Event:       "clock_sync",
		SysVars: &SystemVariables{
			Stratum: 3,
			RefID:   "0001E240",
			RefTime: "0x01",
			Offset:  0.1,
		},
		Peers: map[uint16]*Peer{
			2: &Peer{
				Configured: true,
				Reachable:  true,
				Selection:  control.SelSYSPeer,
				Condition:  "sys.peer",
				SRCAdr:     "192.168.0.4",
				DSTAdr:     "10.3.2.4",
				Stratum:    2,
				RefID:      "20012210",
				RefTime:    "0x02",
				Reach:      255,
				PPoll:      11,
				HPoll:      11,
				Offset:     20,
				Flashers:   []string{},
			},
		},
	}

	check := &NTPCheck{
		Client: &fakeNTPClient{readCount: 0, outputs: prepdOutputs},
	}

	got, err := check.Run()
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestNTPCheck_ServerStats(t *testing.T) {
	prepdOutputs := []*control.NTPControlMsg{
		// read server variables
		&control.NTPControlMsg{
			NTPControlMsgHead: control.NTPControlMsgHead{
				VnMode:        vnMode,
				REMOp:         control.MakeREMOp(true, false, false, control.OpReadVariables),
				AssociationID: 0,
			},
			Data: []uint8("ss_received=1234,ss_badformat=5670,ss_badauth=5,ss_declined=1,ss_restricted=1,ss_limited=1"),
		},
	}

	want := &ServerStats{
		PacketsReceived: 1234,
		PacketsDropped:  5678,
	}

	check := &NTPCheck{
		Client: &fakeNTPClient{readCount: 0, outputs: prepdOutputs},
	}

	got, err := check.ServerStats()
	require.NoError(t, err)
	require.Equal(t, want, got)
}
