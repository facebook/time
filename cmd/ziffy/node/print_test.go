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
	"strconv"
	"testing"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/stretchr/testify/require"
)

func TestMin(t *testing.T) {
	require.Equal(t, 2, min(2, 3))
	require.Equal(t, 2, min(2, 100))
	require.Equal(t, -3, min(2, -3))
	require.Equal(t, 1, min(1, 1))
	require.Equal(t, -4, min(-2, -4))
}

func TestComputeInfo(t *testing.T) {
	s := Sender{
		Config: &Config{},
	}
	numInfos := 5
	cfThold := 250
	cfIncrement := 100
	currentCf := 0

	s.routes = append(s.routes, PathInfo{})

	for i := 0; i <= numInfos; i++ {
		s.routes[0].switches = append(s.routes[0].switches, SwitchTrafficInfo{
			ip:        strconv.Itoa(i),
			hop:       i,
			corrField: ptp.NewCorrection(float64(currentCf)),
		})
		currentCf += cfIncrement
	}

	info := computeInfo(s.routes, ptp.NewCorrection(float64(cfThold)))

	for i := 0; i <= numInfos; i++ {
		if info[keyPair{strconv.Itoa(i), i}].last {
			require.Equal(t, tcNA, int(info[keyPair{strconv.Itoa(i), i}].tcEnable))
			continue
		}
		if info[keyPair{strconv.Itoa(i), i}].avgCF > ptp.NewCorrection(float64(cfThold)) {
			require.Equal(t, tcTrue, int(info[keyPair{strconv.Itoa(i), i}].tcEnable))
		} else {
			require.Equal(t, tcFalse, int(info[keyPair{strconv.Itoa(i), i}].tcEnable))
		}
	}
}

func TestGetHostNoPrefix(t *testing.T) {
	require.Equal(t, "localhost", getHostNoPrefix("eth1.localhost"))
	require.Equal(t, "localhost", getHostNoPrefix("eth1-432.localhost"))
	require.Equal(t, "sswyyy.asd.asd.asd.tfbnw.net.", getHostNoPrefix("eth4-4-1.sswyyy.asd.asd.asd.tfbnw.net."))
	require.Equal(t, "sswyyy.asd.asd.asd.tfbnw.net.", getHostNoPrefix("sswyyy.asd.asd.asd.tfbnw.net."))
	require.Equal(t, "2401:face:face::", getHostNoPrefix("2401:face:face::"))
	require.Equal(t, "1.2.3.4", getHostNoPrefix("1.2.3.4"))
	require.Equal(t, "1.2.3.4", getHostNoPrefix("eth0.1.2.3.4"))
}

func TestGetHostIfPrefix(t *testing.T) {
	require.Equal(t, "eth1", getHostIfPrefix("eth1.localhost"))
	require.Equal(t, "eth1-432", getHostIfPrefix("eth1-432.localhost"))
	require.Equal(t, "eth4-4-1", getHostIfPrefix("eth4-4-1.sswyyy.asd.asd.asd.tfbnw.net."))
	require.Equal(t, "", getHostIfPrefix("sswyyy.asd.asd.asd.tfbnw.net."))
	require.Equal(t, "", getHostIfPrefix("2401:face:face::"))
	require.Equal(t, "", getHostIfPrefix("1.2.3.4"))
	require.Equal(t, "eth0", getHostIfPrefix("eth0.1.2.3.4"))
}

func TestHopCount(t *testing.T) {
	swInfo := []SwitchPrintInfo{
		{hop: 1},
		{hop: 1},
		{hop: 2},
		{hop: 2},
		{hop: 2},
		{hop: 3},
		{hop: 4},
		{hop: 4},
		{hop: 4},
		{hop: 4},
	}

	cnt := hopCount(swInfo, 1)
	require.Equal(t, 2, cnt)

	cnt = hopCount(swInfo, 2)
	require.Equal(t, 3, cnt)

	cnt = hopCount(swInfo, 3)
	require.Equal(t, 1, cnt)

	cnt = hopCount(swInfo, 4)
	require.Equal(t, 4, cnt)

	cnt = hopCount(swInfo, 5)
	require.Equal(t, 0, cnt)

	cnt = hopCount(swInfo, -1)
	require.Equal(t, 0, cnt)
}

func TestColNumber(t *testing.T) {
	header := []string{"uniq", "width", "hop", "ip_address", "intf", "hostname", "flows", "TC", "avg_CF(ns)", "max_CF(ns)", "min_CF(ns)"}

	nr, err := colNumber(header, "uniq")
	require.Nil(t, err)
	require.Equal(t, 0, nr)

	nr, err = colNumber(header, "ip_address")
	require.Nil(t, err)
	require.Equal(t, 3, nr)

	nr, err = colNumber(header, "avg_CF(ns)")
	require.Nil(t, err)
	require.Equal(t, 8, nr)

	nr, err = colNumber(header, "random_col_name")
	require.NotNil(t, err)
	require.Equal(t, -1, nr)

	nr, err = colNumber(header, "TCtypo")
	require.NotNil(t, err)
	require.Equal(t, -1, nr)
}
