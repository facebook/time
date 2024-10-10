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
	"net/netip"

	ptp "github.com/facebook/time/ptp/protocol"

	"github.com/facebook/time/ptp/sptp/bmc"
)

func bmca(results map[netip.Addr]*RunResult, prios map[ptp.ClockIdentity]int, cfg *Config) *ptp.Announce {
	if len(results) == 0 {
		return nil
	}
	var best *ptp.Announce
	for _, result := range results {
		if result.Measurement == nil || result.Error != nil || result.Measurement.CorrectionFieldRX < 0 || result.Measurement.CorrectionFieldTX < 0 {
			continue
		}
		if best == nil {
			best = &result.Measurement.Announce
			continue
		}
		a := best
		b := &result.Measurement.Announce
		localPrioA := prios[a.AnnounceBody.GrandmasterIdentity]
		localPrioB := prios[b.AnnounceBody.GrandmasterIdentity]
		if bmc.TelcoDscmp(a, b, localPrioA, localPrioB) < 0 {
			best = b
		}
	}
	// Never select GM if worse (greater) than MaxClockClass or with clock accuracy worse than MaxClockAccuracy
	if best == nil || best.AnnounceBody.GrandmasterClockQuality.ClockClass > cfg.MaxClockClass || best.AnnounceBody.GrandmasterClockQuality.ClockAccuracy > cfg.MaxClockAccuracy {
		return nil
	}
	return best
}
