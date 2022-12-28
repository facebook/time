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
	"testing"

	"github.com/facebook/time/calnex/api"
	"github.com/facebook/time/calnex/config"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalConfig(t *testing.T) {
	testConfig := `
	{
		"calnex01.example.com": {
			"antennaDelayNS": 42,
			"measure": {
				"a": {
					"target": "fd00::d",
					"probe": "pps"
				},
				"VP1": {
					"target": "fd00::d",
					"probe": "ntp"
				},
				"VP22": {
					"target": "fd00::d",
					"probe": "ptp"
				}
			}
		}
	}
`

	var cs calnexes
	err := json.Unmarshal([]byte(testConfig), &cs)
	require.NoError(t, err)

	expected := calnexes{}
	expected["calnex01.example.com"] = config.CalnexConfig{
		AntennaDelayNS: 42,
		Measure: map[api.Channel]config.MeasureConfig{
			api.ChannelA: {
				Target: "fd00::d",
				Probe:  api.ProbePPS,
			},
			api.ChannelVP1: {
				Target: "fd00::d",
				Probe:  api.ProbeNTP,
			},
			api.ChannelVP22: {
				Target: "fd00::d",
				Probe:  api.ProbePTP,
			},
		},
	}
	require.Equal(t, expected, cs)
}
