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

package client

import (
	ptp "github.com/facebook/time/ptp/protocol"

	"github.com/facebook/time/ptp/sptp/bmc"
)

func bmca(msgs []*ptp.Announce, prios map[ptp.ClockIdentity]int, cfg *Config) *ptp.Announce {
	if len(msgs) == 0 {
		return nil
	}
	best := msgs[0]
	for _, msg := range msgs[1:] {
		a := best
		b := msg
		localPrioA := prios[a.AnnounceBody.GrandmasterIdentity]
		localPrioB := prios[b.AnnounceBody.GrandmasterIdentity]
		if bmc.TelcoDscmp(a, b, localPrioA, localPrioB) < 0 {
			best = b
		}
	}
	// Never select GM if worse (greater) than MaxClockClass or with clock accuracy worse than MaxClockAccuracy
	if best.AnnounceBody.GrandmasterClockQuality.ClockClass > cfg.MaxClockClass || best.AnnounceBody.GrandmasterClockQuality.ClockAccuracy > cfg.MaxClockAccuracy {
		return nil
	}
	return best
}
