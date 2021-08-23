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

package node

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	ptp "github.com/facebookincubator/ptp/protocol"
)

func TestPopAllQueue(t *testing.T) {
	s := Sender{
		Config: &Config{QueueCap: 10000},
	}
	numInfos := 5
	numRoutes := 2

	s.inputQueue = make(chan *SwitchTrafficInfo, s.Config.QueueCap)

	s.routes = append(s.routes, PathInfo{switches: nil})
	s.routes = append(s.routes, PathInfo{switches: nil})

	for i := 0; i <= numInfos; i++ {
		s.inputQueue <- &SwitchTrafficInfo{
			hop:      i,
			routeIdx: i % numRoutes,
		}
	}
	s.popAllQueue()

	require.Equal(t, 0, s.routes[0].switches[0].hop)
	require.Equal(t, 2, s.routes[0].switches[1].hop)
	require.Equal(t, 4, s.routes[0].switches[2].hop)

	require.Equal(t, 1, s.routes[1].switches[0].hop)
	require.Equal(t, 3, s.routes[1].switches[1].hop)
	require.Equal(t, 5, s.routes[1].switches[2].hop)
}

func TestClearPaths(t *testing.T) {
	s := Sender{
		Config: &Config{DestinationAddress: "2401:db00:251c:2608:1:2:c:d"},
	}
	noOrderIndex := 0
	noOrderPath := []SwitchTrafficInfo{
		SwitchTrafficInfo{hop: 1, routeIdx: 1},
		SwitchTrafficInfo{hop: 4, routeIdx: 1},
		SwitchTrafficInfo{hop: 2, routeIdx: 1},
	}
	duplicateIndex := 1
	duplicatePath := []SwitchTrafficInfo{
		SwitchTrafficInfo{hop: 1, routeIdx: 1},
		SwitchTrafficInfo{hop: 2, routeIdx: 1},
		SwitchTrafficInfo{hop: 2, routeIdx: 1},
		SwitchTrafficInfo{hop: 3, routeIdx: 1},
	}
	s.routes = append(s.routes, PathInfo{switches: noOrderPath})
	s.routes = append(s.routes, PathInfo{switches: duplicatePath})

	res := s.clearPaths()
	require.Equal(t, 1, res[noOrderIndex].switches[0].hop)
	require.Equal(t, 2, res[noOrderIndex].switches[1].hop)
	require.Equal(t, 4, res[noOrderIndex].switches[2].hop)

	require.Equal(t, 1, res[duplicateIndex].switches[0].hop)
	require.Equal(t, 2, res[duplicateIndex].switches[1].hop)
	require.Equal(t, 3, res[duplicateIndex].switches[2].hop)
	require.Equal(t, 3, len(res[duplicateIndex].switches))
}

func TestSortSwitchesByHop(t *testing.T) {
	var switches []SwitchTrafficInfo
	switches = append(switches, SwitchTrafficInfo{hop: 3, routeIdx: 1})
	switches = append(switches, SwitchTrafficInfo{hop: 1, routeIdx: 2})
	switches = append(switches, SwitchTrafficInfo{hop: 10, routeIdx: 5})
	switches = append(switches, SwitchTrafficInfo{hop: 21, routeIdx: 5})
	switches = append(switches, SwitchTrafficInfo{hop: 5, routeIdx: 5})

	sortSwitchesByHop(switches)
	require.Equal(t, 1, switches[0].hop)
	require.Equal(t, 2, switches[0].routeIdx)

	require.Equal(t, 3, switches[1].hop)
	require.Equal(t, 1, switches[1].routeIdx)

	require.Equal(t, 5, switches[2].hop)
	require.Equal(t, 5, switches[2].routeIdx)

	require.Equal(t, 10, switches[3].hop)
	require.Equal(t, 5, switches[3].routeIdx)

	require.Equal(t, 21, switches[4].hop)
	require.Equal(t, 5, switches[4].routeIdx)
}

func TestFormNewDest(t *testing.T) {
	s := Sender{
		Config: &Config{DestinationAddress: "2401:db00:251c:2608:1:2:c:d"},
	}

	ip := s.formNewDest(4)
	require.Equal(t, "2401:db00:251c:2608:face:face:0:4", ip.String())
	ip = s.formNewDest(int(0xffff))
	require.Equal(t, "2401:db00:251c:2608:face:face:0:ffff", ip.String())
	ip = s.formNewDest(int(0xab))
	require.Equal(t, "2401:db00:251c:2608:face:face:0:ab", ip.String())
	ip = s.formNewDest(int(0xabcdef))
	require.Equal(t, "2401:db00:251c:2608:face:face:0:cdef", ip.String())
}

func TestSweepRackPrefix(t *testing.T) {
	s := Sender{
		Config: &Config{
			DestinationAddress: "2401:db00:251c:2608:1:2:c:d",
			IPCount:            3,
			PortCount:          5,
		},
	}

	s.routes = append(s.routes, PathInfo{switches: []SwitchTrafficInfo{}})
	s.sweepRackPrefix()
	require.Equal(t, 16, len(s.routes))
	s.routes = append(s.routes, PathInfo{switches: []SwitchTrafficInfo{}})
	s.routes = append(s.routes, PathInfo{switches: []SwitchTrafficInfo{}})
	s.sweepRackPrefix()
	require.Equal(t, 33, len(s.routes))
}

func TestFormSyncPacket(t *testing.T) {
	s := Sender{
		Config: &Config{
			DestinationAddress: "2401:db00:251c:2608:1:2:c:d",
			IPCount:            3,
			PortCount:          5,
			HopMax:             3,
			HopMin:             1,
			MessageType:        ptp.MessageSync,
		},
	}

	pkt := s.formSyncPacket(4, 33000)
	require.Equal(t, uint8(ZiffyHexa), pkt.ControlField)
	require.Equal(t, uint16(4), pkt.SequenceID)
	require.Equal(t, uint16(33000), pkt.SourcePortIdentity.PortNumber)
	require.Equal(t, ptp.NewSdoIDAndMsgType(ptp.MessageSync, 0), pkt.SdoIDAndMsgType)

	s.Config.MessageType = ptp.MessageDelayReq
	pkt = s.formSyncPacket(7, 12345)
	require.Equal(t, uint8(ZiffyHexa), pkt.ControlField)
	require.Equal(t, uint16(7), pkt.SequenceID)
	require.Equal(t, uint16(12345), pkt.SourcePortIdentity.PortNumber)
	require.Equal(t, ptp.NewSdoIDAndMsgType(ptp.MessageDelayReq, 0), pkt.SdoIDAndMsgType)
}

func TestRackSwHostnameMonitor(t *testing.T) {
	rackSwHostname, err := rackSwHostnameMonitor("non-existing-eth", 1*time.Second)
	require.NotNil(t, err)
	require.Equal(t, "", rackSwHostname)

	rackSwHostname, err = rackSwHostnameMonitor("another-non-eth", 1*time.Second)
	require.NotNil(t, err)
	require.NotNil(t, "", rackSwHostname)
}
