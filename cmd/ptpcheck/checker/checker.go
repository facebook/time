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
	ptp "github.com/facebook/time/ptp/protocol"
)

// PTPCheckResult is selected parts of various stats we expose to users, abstracting away protocol implementation
type PTPCheckResult struct {
	OffsetFromMasterNS  float64
	GrandmasterPresent  bool
	StepsRemoved        int
	MeanPathDelayNS     float64
	GrandmasterIdentity string
	IngressTimeNS       int64
	CorrectionFieldTxNS int64
	CorrectionFieldRxNS int64
	PortStatsTX         map[string]uint64
	PortStatsRX         map[string]uint64
	PortServiceStats    *ptp.PortServiceStats
}
