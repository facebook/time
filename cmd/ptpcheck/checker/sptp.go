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
	"github.com/facebook/time/ptp/sptp/stats"

	log "github.com/sirupsen/logrus"
)

// RunSPTP will run checker against SPTP and return PTPCheckResult
func RunSPTP(address string) (*PTPCheckResult, error) {
	sm, err := stats.FetchStats(address)
	if err != nil {
		return nil, err
	}
	var selected *stats.Stats
	for _, s := range sm {
		if s.Selected {
			selected = &s
			break
		}
	}
	result := &PTPCheckResult{
		PortStatsTX: map[string]uint64{},
		PortStatsRX: map[string]uint64{},
	}
	if selected != nil {
		gmPresent := false
		if selected.GMPresent != 0 {
			gmPresent = true
		}
		result.OffsetFromMasterNS = selected.Offset
		result.IngressTimeNS = selected.IngressTime
		result.GrandmasterPresent = gmPresent
		result.MeanPathDelayNS = selected.MeanPathDelay
		result.StepsRemoved = selected.StepsRemoved
		result.GrandmasterIdentity = selected.PortIdentity
		result.CorrectionFieldRxNS = selected.CorrectionFieldRX
		result.CorrectionFieldTxNS = selected.CorrectionFieldTX
	}

	// port stats
	tx, rx, err := stats.FetchPortStats(address)
	if err != nil {
		log.Warningf("couldn't get SPTP counters: %v", err)
	} else {
		for k, v := range tx {
			result.PortStatsTX[k] = v
		}
		for k, v := range rx {
			result.PortStatsRX[k] = v
		}
	}

	return result, nil
}
