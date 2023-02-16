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

// ComparePortIdentity compares two port identities
func ComparePortIdentity(this *ptp.PortIdentity, that *ptp.PortIdentity) int64 {
	diff := int64(this.ClockIdentity) - int64(that.ClockIdentity)
	if diff == 0 {
		diff = int64(this.PortNumber) - int64(that.PortNumber)
	}
	return diff
}

// Dscmp2 finds better Announce based on network topology
func Dscmp2(a *ptp.Announce, b *ptp.Announce) ComparisonResult {
	if a.AnnounceBody.StepsRemoved+1 < b.AnnounceBody.StepsRemoved {
		return ABetter
	}
	if b.AnnounceBody.StepsRemoved+1 < a.AnnounceBody.StepsRemoved {
		return BBetter
	}

	diff := ComparePortIdentity(&a.Header.SourcePortIdentity, &b.Header.SourcePortIdentity)
	if diff < 0 {
		return ABetterTopo
	}
	if diff > 0 {
		return BBetterTopo
	}
	return Unknown
}

// Dscmp finds better Announce based on Announce response content
func Dscmp(a *ptp.Announce, b *ptp.Announce) ComparisonResult {
	if a.AnnounceBody == b.AnnounceBody {
		return Unknown
	}
	diff := int64(a.AnnounceBody.GrandmasterIdentity) - int64(b.AnnounceBody.GrandmasterIdentity)
	if diff == 0 {
		return Dscmp2(a, b)
	}
	if a.AnnounceBody.GrandmasterPriority1 < b.AnnounceBody.GrandmasterPriority1 {
		return ABetter
	}
	if a.AnnounceBody.GrandmasterPriority1 > b.AnnounceBody.GrandmasterPriority1 {
		return BBetter
	}

	if a.AnnounceBody.GrandmasterClockQuality.ClockClass < b.AnnounceBody.GrandmasterClockQuality.ClockClass {
		return ABetter
	}
	if a.AnnounceBody.GrandmasterClockQuality.ClockClass > b.AnnounceBody.GrandmasterClockQuality.ClockClass {
		return BBetter
	}
	if a.AnnounceBody.GrandmasterClockQuality.ClockAccuracy < b.AnnounceBody.GrandmasterClockQuality.ClockAccuracy {
		return ABetter
	}
	if a.AnnounceBody.GrandmasterClockQuality.ClockAccuracy > b.AnnounceBody.GrandmasterClockQuality.ClockAccuracy {
		return BBetter
	}
	if a.AnnounceBody.GrandmasterClockQuality.OffsetScaledLogVariance < b.AnnounceBody.GrandmasterClockQuality.OffsetScaledLogVariance {
		return ABetter
	}
	if a.AnnounceBody.GrandmasterClockQuality.OffsetScaledLogVariance > b.AnnounceBody.GrandmasterClockQuality.OffsetScaledLogVariance {
		return BBetter
	}
	if a.AnnounceBody.GrandmasterPriority2 < b.AnnounceBody.GrandmasterPriority2 {
		return ABetter
	}
	if a.AnnounceBody.GrandmasterPriority2 > b.AnnounceBody.GrandmasterPriority2 {
		return BBetter
	}
	if diff < 0 {
		return ABetter
	}
	return BBetter
}

// TelcoDscmp Dscmp finds better Announce based on Announce response content and local priorities
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

	if a.AnnounceBody.GrandmasterClockQuality.ClockClass < b.AnnounceBody.GrandmasterClockQuality.ClockClass {
		return ABetter
	}
	if a.AnnounceBody.GrandmasterClockQuality.ClockClass > b.AnnounceBody.GrandmasterClockQuality.ClockClass {
		return BBetter
	}
	if a.AnnounceBody.GrandmasterClockQuality.ClockAccuracy < b.AnnounceBody.GrandmasterClockQuality.ClockAccuracy {
		return ABetter
	}
	if a.AnnounceBody.GrandmasterClockQuality.ClockAccuracy > b.AnnounceBody.GrandmasterClockQuality.ClockAccuracy {
		return BBetter
	}
	if a.AnnounceBody.GrandmasterClockQuality.OffsetScaledLogVariance < b.AnnounceBody.GrandmasterClockQuality.OffsetScaledLogVariance {
		return ABetter
	}
	if a.AnnounceBody.GrandmasterClockQuality.OffsetScaledLogVariance > b.AnnounceBody.GrandmasterClockQuality.OffsetScaledLogVariance {
		return BBetter
	}
	if a.AnnounceBody.GrandmasterPriority2 < b.AnnounceBody.GrandmasterPriority2 {
		return ABetter
	}
	if a.GrandmasterPriority2 > b.AnnounceBody.GrandmasterPriority2 {
		return BBetter
	}
	if localPrioA < localPrioB {
		return ABetter
	}
	if localPrioA > localPrioB {
		return BBetter
	}
	if a.AnnounceBody.GrandmasterClockQuality.ClockClass <= 127 {
		return Dscmp2(a, b)
	}
	diff := int64(a.AnnounceBody.GrandmasterIdentity) - int64(b.AnnounceBody.GrandmasterIdentity)
	if diff == 0 {
		return Dscmp2(a, b)
	}

	if diff < 0 {
		return ABetter
	}
	return BBetter
}
