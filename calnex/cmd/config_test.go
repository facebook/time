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

package cmd

import (
	"encoding/json"
	"net"
	"testing"

	"github.com/facebook/time/calnex/api"
	"github.com/facebook/time/calnex/config"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalConfig(t *testing.T) {
	testConfig := `
	{
		"calnex01.example.com": {
			"network" :{
				"eth1": "fd00::11",
				"gw1": "fd00::a",
				"eth2": "fd00::12",
				"gw2": "fd00::a"
			},
			"calnex": {
				"a": {
					"target": "fd00::d",
					"probe": "pps"
				},
				"1": {
					"target": "fd00::d",
					"probe": "ntp"
				},
				"2": {
					"target": "fd00::d",
					"probe": "ptp"
				}
			}
		}
	}
`

	var d devices
	err := json.Unmarshal([]byte(testConfig), &d)
	require.NoError(t, err)

	expected := devices{}
	expected["calnex01.example.com"] = deviceConfig{
		Calnex: config.CalnexConfig{
			api.ChannelA: config.MeasureConfig{
				Target: "fd00::d",
				Probe:  api.ProbePPS,
			},
			api.ChannelONE: config.MeasureConfig{
				Target: "fd00::d",
				Probe:  api.ProbeNTP,
			},
			api.ChannelTWO: config.MeasureConfig{
				Target: "fd00::d",
				Probe:  api.ProbePTP,
			},
		},
		Network: &config.NetworkConfig{
			Eth1: net.ParseIP("fd00::11"),
			Gw1:  net.ParseIP("fd00::a"),
			Eth2: net.ParseIP("fd00::12"),
			Gw2:  net.ParseIP("fd00::a"),
		},
	}
	require.Equal(t, expected, d)
}
