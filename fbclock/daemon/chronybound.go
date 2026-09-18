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

package daemon

import (
	"fmt"
	"math"
	"time"

	"github.com/facebook/time/ntp/chrony"
	ntp "github.com/facebook/time/ntp/protocol"
)

// maxChronyBoundNS is a sanity limit, ~31 years in ns, to keep +Inf out of the
// float-to-uint64 conversion below.
const maxChronyBoundNS float64 = 1e18

// ChronyBound turns a chronyd tracking reply into the two numbers fbclock
// publishes: how far the local clock may be from true time, and how fast that
// uncertainty grows while chronyd has nothing fresh to report.
//
//	errorBoundNS = ceil((|CurrentCorrection| + RootDispersion + RootDelay/2) * 1e9)
//	holdoverNS   = max(SkewPPM, maxDriftRatePPM) * 1000, in ns per second
//
// It needs only the reply and the floor, no daemon state and no PHC, so a tool
// outside the daemon can compute the same bound the daemon publishes.
func ChronyBound(t *chrony.Tracking, maxDriftRatePPM float64) (errorBoundNS uint64, holdoverNS float64, err error) {
	// the floor is a flag, not chronyd data, and the config check on it does not
	// catch a NaN. Zero is allowed and means no floor.
	if math.IsNaN(maxDriftRatePPM) || math.IsInf(maxDriftRatePPM, 0) || maxDriftRatePPM < 0 {
		return 0, 0, fmt.Errorf("%w: max drift rate is %v", errCorrectness, maxDriftRatePPM)
	}
	// chronyd has no usable source, so its error model says nothing about our clock
	if t.LeapStatus == ntp.LeapAlarm {
		return 0, 0, fmt.Errorf("%w: chronyd is not synchronised", errCorrectness)
	}
	if t.RefTime.UnixNano() <= 0 {
		return 0, 0, fmt.Errorf("%w: chronyd reference time is not set", errCorrectness)
	}

	// the bound uses the size of the correction, not its direction; the other
	// three terms are distances and can never be negative
	correction := math.Abs(t.CurrentCorrection)
	// chronyd should never send a garbage term, and nothing downstream would
	// catch one: a negative term quietly narrows the bound, and a NaN skew
	// reaches clients as a holdover rate of zero.
	for _, term := range []struct {
		name  string
		value float64
	}{
		{"current correction", correction},
		{"root dispersion", t.RootDispersion},
		{"root delay", t.RootDelay},
		// not part of the bound, but it is the whole holdover rate
		{"skew", t.SkewPPM},
	} {
		if math.IsNaN(term.value) || math.IsInf(term.value, 0) || term.value < 0 {
			return 0, 0, fmt.Errorf("%w: chronyd %s is %v", errCorrectness, term.name, term.value)
		}
	}

	// chronyd reports every term in seconds
	boundNS := (correction + t.RootDispersion + t.RootDelay/2) * float64(time.Second)
	// checked after scaling, which can reach +Inf on its own; negated so a NaN
	// fails it too
	if !(boundNS >= 1 && boundNS <= maxChronyBoundNS) {
		return 0, 0, fmt.Errorf("%w: error bound is %vns", errCorrectness, boundNS)
	}

	// chronyd's skew is optimistically small just after startup, so floor it at
	// the configured max drift rate
	holdoverNS = math.Max(t.SkewPPM, maxDriftRatePPM) * nsPerSecondPerPPM
	// both inputs are finite by here, but the scaling can still overflow
	if math.IsInf(holdoverNS, 0) {
		return 0, 0, fmt.Errorf("%w: holdover rate overflowed: skew %v PPM, floor %v PPM",
			errCorrectness, t.SkewPPM, maxDriftRatePPM)
	}
	// round up: a bound must never understate the error
	return uint64(math.Ceil(boundNS)), holdoverNS, nil
}
