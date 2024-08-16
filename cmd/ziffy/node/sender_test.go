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
)

func TestPopAllQueue(t *testing.T) {
	s := Sender{
		Config: &Config{QueueCap: 10000},
	}
	numInfos := 5
	numRoutes := 2

	s.inputQueue = make([]chan *SwitchTrafficInfo, numRoutes)

	s.inputQueue[0] = make(chan *SwitchTrafficInfo, s.Config.QueueCap)
	s.inputQueue[1] = make(chan *SwitchTrafficInfo, s.Config.QueueCap)

	routes := []*PathInfo{
		{switches: nil},
		{switches: nil},
	}

	for i := 0; i <= numInfos; i++ {
		routeIdx := i % numRoutes
		s.inputQueue[routeIdx] <- &SwitchTrafficInfo{
			hop:      i,
			routeIdx: routeIdx,
		}
	}
	s.popAllQueue(routes)

	require.Equal(t, 0, routes[0].switches[0].hop)
	require.Equal(t, 2, routes[0].switches[1].hop)
	require.Equal(t, 4, routes[0].switches[2].hop)

	require.Equal(t, 1, routes[1].switches[0].hop)
	require.Equal(t, 3, routes[1].switches[1].hop)
	require.Equal(t, 5, routes[1].switches[2].hop)
}

func TestClearPaths(t *testing.T) {
	s := Sender{
		Config: &Config{DestinationAddress: "2401:db00:251c:2608:1:2:c:d"},
	}
	noOrderIndex := 0
	noOrderPath := []SwitchTrafficInfo{
		{hop: 1, routeIdx: 1},
		{hop: 4, routeIdx: 1},
		{hop: 2, routeIdx: 1},
	}
	duplicateIndex := 1
	duplicatePath := []SwitchTrafficInfo{
		{hop: 1, routeIdx: 1},
		{hop: 2, routeIdx: 1},
		{hop: 2, routeIdx: 1},
		{hop: 3, routeIdx: 1},
	}
	routes := []*PathInfo{
		{switches: noOrderPath},
		{switches: duplicatePath},
	}

	res := s.clearPaths(routes)
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
	c := &Config{DestinationAddress: "2401:db00:251c:2608:1:2:c:d"}

	ip := formNewDest(c, 4)
	require.Equal(t, "2401:db00:251c:2608:face:face:0:4", ip.String())
	ip = formNewDest(c, int(0xffff))
	require.Equal(t, "2401:db00:251c:2608:face:face:0:ffff", ip.String())
	ip = formNewDest(c, int(0xab))
	require.Equal(t, "2401:db00:251c:2608:face:face:0:ab", ip.String())
	ip = formNewDest(c, int(0xabcdef))
	require.Equal(t, "2401:db00:251c:2608:face:face:0:cdef", ip.String())
}

func TestRackSwHostnameMonitor(t *testing.T) {
	rackSwHostname, err := rackSwHostnameMonitor("non-existing-eth", 1*time.Second)
	require.NotNil(t, err)
	require.Equal(t, "", rackSwHostname)

	rackSwHostname, err = rackSwHostnameMonitor("another-non-eth", 1*time.Second)
	require.NotNil(t, err)
	require.NotNil(t, "", rackSwHostname)
}
