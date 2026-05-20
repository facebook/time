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

package bmc

import (
	ptp "github.com/facebook/time/ptp/protocol"
)

// ComparisonResult is the type to represent comparisons
type ComparisonResult int8

const (
	// ABetterTopo means A is better based on topology
	ABetterTopo ComparisonResult = 2
	// ABetter means A is better based on Announce Response
	ABetter ComparisonResult = 1
	// Unknown means we failed to determine better
	Unknown ComparisonResult = 0
	// BBetter means B is better based on Announce Response
	BBetter ComparisonResult = -1
	// BBetterTopo means B is better based on topology
	BBetterTopo ComparisonResult = -2
)

// Dscmp2 finds better Announce based on network topology
func Dscmp2(a, b *ptp.Announce) ComparisonResult {
	if a.StepsRemoved+1 < b.StepsRemoved {
		return ABetter
	}
	if b.StepsRemoved+1 < a.StepsRemoved {
		return BBetter
	}

	p1, p2 := a.SourcePortIdentity, b.SourcePortIdentity
	switch p1.Compare(p2) {
	case -1:
		return ABetterTopo
	case 1:
		return BBetterTopo
	default:
		return Unknown
	}
}

// Base comparison on all attributes
func dscmp(a *ptp.Announce, b *ptp.Announce) ComparisonResult {
	if a.GrandmasterClockQuality.ClockClass < b.GrandmasterClockQuality.ClockClass {
		return ABetter
	}
	if a.GrandmasterClockQuality.ClockClass > b.GrandmasterClockQuality.ClockClass {
		return BBetter
	}
	if a.GrandmasterClockQuality.ClockAccuracy < b.GrandmasterClockQuality.ClockAccuracy {
		return ABetter
	}
	if a.GrandmasterClockQuality.ClockAccuracy > b.GrandmasterClockQuality.ClockAccuracy {
		return BBetter
	}
	if a.GrandmasterClockQuality.OffsetScaledLogVariance < b.GrandmasterClockQuality.OffsetScaledLogVariance {
		return ABetter
	}
	if a.GrandmasterClockQuality.OffsetScaledLogVariance > b.GrandmasterClockQuality.OffsetScaledLogVariance {
		return BBetter
	}
	if a.GrandmasterPriority2 < b.GrandmasterPriority2 {
		return ABetter
	}
	if a.GrandmasterPriority2 > b.GrandmasterPriority2 {
		return BBetter
	}

	return Unknown
}

// Dscmp finds better Announce based on Announce response content
func Dscmp(a *ptp.Announce, b *ptp.Announce) ComparisonResult {
	if a.AnnounceBody == b.AnnounceBody {
		return Unknown
	}
	diff := int64(a.GrandmasterIdentity) - int64(b.GrandmasterIdentity)
	if diff == 0 {
		return Dscmp2(a, b)
	}
	if a.GrandmasterPriority1 < b.GrandmasterPriority1 {
		return ABetter
	}
	if a.GrandmasterPriority1 > b.GrandmasterPriority1 {
		return BBetter
	}

	// Base comparison on all attributes
	if cr := dscmp(a, b); cr != Unknown {
		return cr
	}

	if diff < 0 {
		return ABetter
	}
	return BBetter
}

// TelcoDscmp finds better Announce based on Announce response content and local priorities
func TelcoDscmp(a *ptp.Announce, b *ptp.Announce, localPrioA int, localPrioB int) ComparisonResult {
	if a.AnnounceBody == b.AnnounceBody {
		return Unknown
	}
	if a != nil && b == nil {
		return ABetter
	}
	if b != nil && a == nil {
		return BBetter
	}

	// Base comparison on all attributes
	if cr := dscmp(a, b); cr != Unknown {
		return cr
	}

	if localPrioA < localPrioB {
		return ABetter
	}
	if localPrioA > localPrioB {
		return BBetter
	}
	if a.GrandmasterClockQuality.ClockClass <= 127 {
		return Dscmp2(a, b)
	}
	diff := int64(a.GrandmasterIdentity) - int64(b.GrandmasterIdentity)
	if diff == 0 {
		return Dscmp2(a, b)
	}

	if diff < 0 {
		return ABetter
	}
	return BBetter
}
