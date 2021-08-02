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
	"strconv"

	"github.com/facebookincubator/ntp/protocol/control"
)

// various counters for dropped packets
var droppedVars = []string{"ss_badformat", "ss_badauth", "ss_declined", "ss_restricted", "ss_limited"}

// ServerStats holds NTP server operational stats
type ServerStats struct {
	PacketsReceived uint64 `json:"ntp.server.packets_received"`
	PacketsDropped  uint64 `json:"ntp.server.packets_dropped"`
}

// NewServerStatsFromNTP constructs ServerStats from NTPControlMsg packet
func NewServerStatsFromNTP(p *control.NTPControlMsg) (*ServerStats, error) {
	data, err := control.NormalizeData(p.Data)
	if err != nil {
		return nil, err
	}
	received, err := strconv.ParseUint(data["ss_received"], 10, 64)
	if err != nil {
		return nil, err
	}
	var dropped uint64
	for _, varName := range droppedVars {
		v, err := strconv.ParseUint(data[varName], 10, 64)
		if err != nil {
			return nil, err
		}
		dropped += v
	}
	return &ServerStats{
		PacketsReceived: received,
		PacketsDropped:  dropped,
	}, nil
}
