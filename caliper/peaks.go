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

// OB window: the back connector (Q-ODC-12 or FO In) is typically 11-13 ns
// after OA. Peaks <10 ns after OA are internal tap splitters (~7 ns offset).
// We search the 10-20 ns window after OA to find the real OB.
const (
	minOBOffsetNs = 10.0
	maxOBOffsetNs = 20.0
)

// DetectPeaks finds the 4 reflective peaks (OA, OB, OC, OD) in the OTDR trace.
// It uses local prominence detection: for each point, it compares the amplitude
// against a local baseline computed from surrounding points. This catches both
// strong reflections (connectors) and subtle bumps (e.g. Q-ODC-12 connectors).
//
// launchCableLengthM specifies the length of the launch cable in meters. Any
// peaks within this distance from the start of the trace are ignored, as they
// are reflections from the launch cable connectors rather than the system under
// test. Use 0 if no launch cable is present.
//
// OA, OC, OD are found via top-prominence selection (they produce strong
// reflections). OB is found by searching for the highest-amplitude raw data
// point in the 10-20 ns window after OA, because the Q-ODC-12 connector can
// produce a very subtle bump that doesn't exceed the prominence threshold.
func DetectPeaks(tor *TORFile, launchCableLengthM float64) ([]Peak, error) {
	n := len(tor.DataPoints)
	if n < 10 {
		return nil, fmt.Errorf("insufficient data points for peak detection: %d", n)
	}

	// Compute local prominence for each point
	prominences := computeLocalProminence(tor.DataPoints)

	// Find local maxima in the prominence signal
	groups := findProminentGroups(tor.DataPoints, prominences, 0.3)

	// Merge groups that are very close together (within 0.5m)
	groups = mergeCloseGroups(tor.DataPoints, groups, 0.5)

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
	// highest amplitude point. The Q-ODC-12 connector can produce a very
	// subtle reflection that doesn't exceed the prominence threshold.
	obGroup, err := findOBInWindow(tor.DataPoints, oaTimeNs, ri)
	if err != nil {
		return nil, err
	}

	// Determine end-of-cable: find where the trace amplitude drops to the
	// noise floor. The cable section (between OB and the cable end) has a
	// characteristic amplitude around -30 to -50 dB with gradual loss. After
	// the cable ends the amplitude drops sharply. We detect this by computing
	// the median amplitude of the cable section, then finding where the
	// amplitude drops more than 10 dB below that median for a sustained run.
	obMaxDistM := (oaTimeNs + maxOBOffsetNs) * SpeedOfLight / (ri * 1e9)
	cableEndDistM := findCableEnd(tor.DataPoints, obMaxDistM)

	// OC and OD: the two most prominent peaks after OB but before the cable
	// end. Cable-end filtering prevents post-cable noise from being selected,
	// while prominence ranking ensures we pick the real connector reflections
	// rather than minor noise between OB and OC.
	var afterOB []PeakGroup
	for _, g := range groups {
		if g.PeakDistM > obMaxDistM && g.PeakDistM <= cableEndDistM {
			afterOB = append(afterOB, g)
		}
	}
	if len(afterOB) < 2 {
		return nil, fmt.Errorf(
			"expected at least 2 prominent peaks after OB (before cable end at %.1f m) but found %d",
			cableEndDistM, len(afterOB),
		)
	}
	sort.Slice(afterOB, func(i, j int) bool {
		return afterOB[i].Prominence > afterOB[j].Prominence
	})
	ocod := afterOB[:2]
	// Re-sort OC/OD by distance so OC < OD
	sort.Slice(ocod, func(i, j int) bool {
		return ocod[i].PeakDistM < ocod[j].PeakDistM
	})

	selected := []PeakGroup{oa, obGroup, ocod[0], ocod[1]}
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

// findCableEnd finds the distance where the OTDR trace drops to the noise floor,
// indicating the physical end of the fiber cable.
func findCableEnd(points []DataPoint, startDistM float64) float64 {
	startIdx := sort.Search(len(points), func(i int) bool {
		return points[i].DistanceM >= startDistM
	})
	if startIdx >= len(points) {
		return points[len(points)-1].DistanceM
	}

	baselineEnd := min(startIdx+50, len(points))
	baselineAmps := make([]float64, baselineEnd-startIdx)
	for i := startIdx; i < baselineEnd; i++ {
		baselineAmps[i-startIdx] = points[i].AmplitudeDB
	}
	sort.Float64s(baselineAmps)
	cableBaseline := baselineAmps[len(baselineAmps)/2]

	const dropThresholdDB = 10.0
	const sustainedCount = 10
	threshold := cableBaseline - dropThresholdDB
	consecutive := 0
	for i := startIdx; i < len(points); i++ {
		if points[i].AmplitudeDB < threshold {
			consecutive++
			if consecutive >= sustainedCount {
				return points[i-sustainedCount+1].DistanceM
			}
		} else {
			consecutive = 0
		}
	}

	return points[len(points)-1].DistanceM
}

// computeLocalProminence computes how far above the local baseline each point is.
func computeLocalProminence(points []DataPoint) []float64 {
	n := len(points)
	prominences := make([]float64, n)

	outerRadius := 50
	innerRadius := 5

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
