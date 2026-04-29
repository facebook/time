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

package caliper

import (
	"fmt"
	"math"
	"sort"
)

// Peak represents a detected reflective event in the OTDR trace
type Peak struct {
	Label       string  // OA, OB, OC, OD
	DistanceM   float64 // distance in meters at peak maximum
	AmplitudeDB float64 // amplitude in dB at peak maximum
	TimeNs      float64 // time in nanoseconds
	Index       int     // index into DataPoints
}

// PeakGroup is a cluster of consecutive elevated data points forming a single reflection event
type PeakGroup struct {
	StartIdx   int
	EndIdx     int
	PeakIdx    int
	PeakDistM  float64
	PeakAmpDB  float64
	Prominence float64
}

// Peak detection parameters tuned for Luciol LOR-220 OTDR traces.
// These values are specific to the LOR-220's resolution and signal
// characteristics. They may need adjustment for other OTDR models.
const (
	// prominenceOuterRadius is the number of points on each side used to
	// compute the local baseline median for prominence calculation.
	prominenceOuterRadius = 50
	// prominenceInnerRadius is the number of points on each side excluded
	// from the baseline (the immediate neighborhood of the point under test).
	prominenceInnerRadius = 5
	// minProminenceDB is the minimum local prominence (in dB above local
	// median) for a point to be considered part of a reflective peak group.
	minProminenceDB = 0.3
	// mergeGapM is the maximum distance in meters between two peak groups
	// for them to be merged into a single group.
	mergeGapM = 0.5
	// minOBOffsetNs is the minimum time offset (ns) after OA to search for OB.
	// Peaks <10 ns after OA are internal tap splitters (~7 ns offset).
	minOBOffsetNs = 10.0
	// maxOBOffsetNs is the maximum time offset (ns) after OA to search for OB.
	maxOBOffsetNs = 20.0
	// minOCOffsetNs is the minimum time offset (ns) before OD to search for OC.
	// The lower bound prevents the search from capturing the rising edge of
	// OD's reflection.
	minOCOffsetNs = 2.0
	// maxOCOffsetNs is the maximum time offset (ns) before OD to search for OC.
	// The antenna-side connector is typically ~5 ns before the optical isolator;
	// 10 ns provides margin for variations in antenna geometry.
	maxOCOffsetNs = 10.0
)

// DetectPeaks finds the 4 reflective peaks (OA, OB, OC, OD) in the OTDR trace.
//
// OA is found via local prominence detection (it is a strong reflection from
// the FO Out connector). OB, OC, OD are found by searching for the
// highest-amplitude raw data point inside fixed time windows, because the
// connectors at OB and OC can produce subtle reflections (e.g. Q-ODC-12 and
// FC APC) that don't exceed the prominence threshold.
//
// OD is anchored to the strongest reflection past OB (the antenna optical
// isolator), and OC is then located in a small window before OD. This avoids
// any reliance on cable-end detection and prevents post-OD saturation noise
// from being mistaken for a peak.
//
// launchCableLengthM specifies the length of the launch cable in meters. Any
// peaks within this distance from the start of the trace are ignored, as they
// are reflections from the launch cable connectors rather than the system under
// test. Use 0 if no launch cable is present.
func DetectPeaks(tor *TORFile, launchCableLengthM float64) ([]Peak, error) {
	n := len(tor.DataPoints)
	if n < 10 {
		return nil, fmt.Errorf("insufficient data points for peak detection: %d", n)
	}

	// Compute local prominence for each point
	prominences := computeLocalProminence(tor.DataPoints)

	// Find local maxima in the prominence signal
	groups := findProminentGroups(tor.DataPoints, prominences, minProminenceDB)

	// Merge groups that are very close together
	groups = mergeCloseGroups(tor.DataPoints, groups, mergeGapM)

	// Filter out peaks within the launch cable region
	if launchCableLengthM > 0 {
		filtered := groups[:0]
		for _, g := range groups {
			if g.PeakDistM >= launchCableLengthM {
				filtered = append(filtered, g)
			}
		}
		groups = filtered
	}

	ri := tor.RefractiveIndex

	// Sort groups by distance for ordered selection
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].PeakDistM < groups[j].PeakDistM
	})

	if len(groups) < 1 {
		return nil, fmt.Errorf("expected at least 1 prominent peak but found 0")
	}

	// OA: the first prominent peak by distance (nearest connector)
	oa := groups[0]
	oaTimeNs := oa.PeakDistM * ri / SpeedOfLight * 1e9

	// OB: search raw data points in the 10-20 ns window after OA for the
	// highest amplitude point. The Q-ODC-12 and LC connectors can produce a
	// very subtle reflection that doesn't exceed the prominence threshold.
	obGroup, err := findOBInWindow(tor.DataPoints, oaTimeNs, ri)
	if err != nil {
		return nil, err
	}
	obMaxDistM := (oaTimeNs + maxOBOffsetNs) * SpeedOfLight / (ri * 1e9)

	// OD: the antenna optical isolator. It is always the strongest reflection
	// past OB, so we find it as the highest-amplitude raw data point after the
	// OB window. Anchoring on amplitude (not prominence) prevents post-OD
	// saturation noise from competing.
	odGroup, err := findODAfter(tor.DataPoints, obMaxDistM)
	if err != nil {
		return nil, err
	}
	odTimeNs := odGroup.PeakDistM * ri / SpeedOfLight * 1e9

	// OC: the antenna-side connector (Q-ODC-12 or FC APC). These connectors
	// are intentionally low-reflection so they produce only a subtle bump
	// shortly before OD. We find OC as the highest-amplitude raw data point
	// in a small window before OD.
	ocGroup, err := findOCInWindow(tor.DataPoints, odTimeNs, ri)
	if err != nil {
		return nil, err
	}

	selected := []PeakGroup{oa, obGroup, ocGroup, odGroup}
	labels := []string{"OA", "OB", "OC", "OD"}
	peaks := make([]Peak, 4)
	for i, g := range selected {
		peaks[i] = Peak{
			Label:       labels[i],
			DistanceM:   g.PeakDistM,
			AmplitudeDB: g.PeakAmpDB,
			TimeNs:      g.PeakDistM * ri / SpeedOfLight * 1e9,
			Index:       g.PeakIdx,
		}
	}

	return peaks, nil
}

// findOBInWindow finds the highest-amplitude data point in the OB time window
// (minOBOffsetNs to maxOBOffsetNs after OA). This searches raw data points
// rather than prominent groups because the Q-ODC-12 connector can produce a
// very subtle reflection that doesn't exceed the prominence threshold.
func findOBInWindow(points []DataPoint, oaTimeNs, ri float64) (PeakGroup, error) {
	minDistM := (oaTimeNs + minOBOffsetNs) * SpeedOfLight / (ri * 1e9)
	maxDistM := (oaTimeNs + maxOBOffsetNs) * SpeedOfLight / (ri * 1e9)
	bestIdx := -1
	bestAmp := math.Inf(-1)
	startIdx := sort.Search(len(points), func(i int) bool {
		return points[i].DistanceM >= minDistM
	})
	for i := startIdx; i < len(points); i++ {
		dp := points[i]
		if dp.DistanceM > maxDistM {
			break
		}
		if dp.AmplitudeDB > bestAmp {
			bestAmp = dp.AmplitudeDB
			bestIdx = i
		}
	}
	if bestIdx == -1 {
		return PeakGroup{}, fmt.Errorf(
			"no data point found in OB window (%.0f-%.0f ns after OA)",
			minOBOffsetNs, maxOBOffsetNs,
		)
	}
	return PeakGroup{
		StartIdx:  bestIdx,
		EndIdx:    bestIdx,
		PeakIdx:   bestIdx,
		PeakDistM: points[bestIdx].DistanceM,
		PeakAmpDB: points[bestIdx].AmplitudeDB,
	}, nil
}

// findODAfter finds the highest-amplitude raw data point past the OB window.
// The antenna optical isolator produces the strongest reflection in the OTDR
// trace beyond OB, so its position is the global amplitude maximum.
func findODAfter(points []DataPoint, obMaxDistM float64) (PeakGroup, error) {
	startIdx := sort.Search(len(points), func(i int) bool {
		return points[i].DistanceM > obMaxDistM
	})
	bestIdx := -1
	bestAmp := math.Inf(-1)
	for i := startIdx; i < len(points); i++ {
		if points[i].AmplitudeDB > bestAmp {
			bestAmp = points[i].AmplitudeDB
			bestIdx = i
		}
	}
	if bestIdx == -1 {
		return PeakGroup{}, fmt.Errorf("no data point found past OB window (>%.1f m)", obMaxDistM)
	}
	return PeakGroup{
		StartIdx:  bestIdx,
		EndIdx:    bestIdx,
		PeakIdx:   bestIdx,
		PeakDistM: points[bestIdx].DistanceM,
		PeakAmpDB: points[bestIdx].AmplitudeDB,
	}, nil
}

// findOCInWindow finds the highest-amplitude raw data point in the OC window
// (minOCOffsetNs to maxOCOffsetNs before OD). Searches raw data points rather
// than prominent groups because the antenna-side connector can produce a very
// subtle reflection that doesn't exceed the prominence threshold.
func findOCInWindow(points []DataPoint, odTimeNs, ri float64) (PeakGroup, error) {
	minDistM := (odTimeNs - maxOCOffsetNs) * SpeedOfLight / (ri * 1e9)
	maxDistM := (odTimeNs - minOCOffsetNs) * SpeedOfLight / (ri * 1e9)
	bestIdx := -1
	bestAmp := math.Inf(-1)
	startIdx := sort.Search(len(points), func(i int) bool {
		return points[i].DistanceM >= minDistM
	})
	for i := startIdx; i < len(points); i++ {
		dp := points[i]
		if dp.DistanceM > maxDistM {
			break
		}
		if dp.AmplitudeDB > bestAmp {
			bestAmp = dp.AmplitudeDB
			bestIdx = i
		}
	}
	if bestIdx == -1 {
		return PeakGroup{}, fmt.Errorf(
			"no data point found in OC window (%.0f-%.0f ns before OD)",
			minOCOffsetNs, maxOCOffsetNs,
		)
	}
	return PeakGroup{
		StartIdx:  bestIdx,
		EndIdx:    bestIdx,
		PeakIdx:   bestIdx,
		PeakDistM: points[bestIdx].DistanceM,
		PeakAmpDB: points[bestIdx].AmplitudeDB,
	}, nil
}

// computeLocalProminence computes how far above the local baseline each point is.
func computeLocalProminence(points []DataPoint) []float64 {
	n := len(points)
	prominences := make([]float64, n)

	outerRadius := prominenceOuterRadius
	innerRadius := prominenceInnerRadius

	localAmps := make([]float64, 0, 2*outerRadius)
	for i := range points {
		localAmps = localAmps[:0]
		for j := max(0, i-outerRadius); j <= min(n-1, i+outerRadius); j++ {
			if j < i-innerRadius || j > i+innerRadius {
				localAmps = append(localAmps, points[j].AmplitudeDB)
			}
		}
		if len(localAmps) == 0 {
			continue
		}
		sort.Float64s(localAmps)
		localMedian := localAmps[len(localAmps)/2]
		prominences[i] = points[i].AmplitudeDB - localMedian
	}

	return prominences
}

// findProminentGroups finds contiguous regions where local prominence exceeds the threshold
func findProminentGroups(points []DataPoint, prominences []float64, minProminence float64) []PeakGroup {
	var groups []PeakGroup
	inGroup := false
	var current PeakGroup

	for i := range points {
		if prominences[i] > minProminence {
			if !inGroup {
				inGroup = true
				current = PeakGroup{
					StartIdx:   i,
					PeakIdx:    i,
					PeakDistM:  points[i].DistanceM,
					PeakAmpDB:  points[i].AmplitudeDB,
					Prominence: prominences[i],
				}
			}
			if prominences[i] > current.Prominence {
				current.PeakIdx = i
				current.PeakDistM = points[i].DistanceM
				current.PeakAmpDB = points[i].AmplitudeDB
				current.Prominence = prominences[i]
			}
		} else {
			if inGroup {
				current.EndIdx = i - 1
				groups = append(groups, current)
				inGroup = false
			}
		}
	}
	if inGroup {
		current.EndIdx = len(points) - 1
		groups = append(groups, current)
	}

	return groups
}

// mergeCloseGroups merges peak groups that are within maxGapM meters of each other
func mergeCloseGroups(points []DataPoint, groups []PeakGroup, maxGapM float64) []PeakGroup {
	if len(groups) <= 1 {
		return groups
	}

	var merged []PeakGroup
	current := groups[0]

	for i := 1; i < len(groups); i++ {
		gapM := points[groups[i].StartIdx].DistanceM - points[current.EndIdx].DistanceM
		if math.Abs(gapM) <= maxGapM {
			current.EndIdx = groups[i].EndIdx
			if groups[i].Prominence > current.Prominence {
				current.PeakIdx = groups[i].PeakIdx
				current.PeakDistM = groups[i].PeakDistM
				current.PeakAmpDB = groups[i].PeakAmpDB
				current.Prominence = groups[i].Prominence
			}
		} else {
			merged = append(merged, current)
			current = groups[i]
		}
	}
	merged = append(merged, current)

	return merged
}
