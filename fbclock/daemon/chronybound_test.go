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
	"math"
	"testing"
	"time"

	"github.com/facebook/time/ntp/chrony"
	ntp "github.com/facebook/time/ntp/protocol"
	"github.com/stretchr/testify/require"
)

const testMaxDriftRatePPM = 50

// healthyTracking is a chronyd reply with every term in range: the bound comes
// out at |-1us| + 2us + 4us/2 = 5us, and the skew sits below the holdover floor.
func healthyTracking() *chrony.Tracking {
	return &chrony.Tracking{
		RefTime:           time.Unix(1755000000, 0),
		CurrentCorrection: -1 * time.Microsecond.Seconds(),
		RootDispersion:    2 * time.Microsecond.Seconds(),
		RootDelay:         4 * time.Microsecond.Seconds(),
		SkewPPM:           0.005,
	}
}

func TestChronyBound(t *testing.T) {
	testCases := []struct {
		name         string
		mutate       func(*chrony.Tracking)
		wantBoundNS  uint64
		wantHoldover float64
	}{
		{
			name:         "healthy reply",
			wantBoundNS:  5000,
			wantHoldover: 50000,
		},
		{
			name:         "chrony skew above the floor sets the holdover rate",
			mutate:       func(tr *chrony.Tracking) { tr.SkewPPM = 80 },
			wantBoundNS:  5000,
			wantHoldover: 80000,
		},
		{
			name:         "a bigger negative correction widens the bound",
			mutate:       func(tr *chrony.Tracking) { tr.CurrentCorrection = -3 * time.Microsecond.Seconds() },
			wantBoundNS:  7000,
			wantHoldover: 50000,
		},
		{
			name: "bounds round outward",
			mutate: func(tr *chrony.Tracking) {
				tr.CurrentCorrection, tr.RootDelay = 0, 0
				tr.RootDispersion = 1.5 * time.Nanosecond.Seconds()
			},
			wantBoundNS:  2,
			wantHoldover: 50000,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tracking := healthyTracking()
			if tc.mutate != nil {
				tc.mutate(tracking)
			}

			boundNS, holdoverNS, err := ChronyBound(tracking, testMaxDriftRatePPM)
			require.NoError(t, err)
			require.Equal(t, tc.wantBoundNS, boundNS)
			require.Equal(t, tc.wantHoldover, holdoverNS)
		})
	}
}

// TestChronyBoundMaxDriftRate covers the holdover floor, the one input that
// comes from the caller rather than from chronyd.
func TestChronyBoundMaxDriftRate(t *testing.T) {
	testCases := []struct {
		name            string
		maxDriftRatePPM float64
		wantHoldover    float64
		wantErr         bool
	}{
		{
			name:            "floors an optimistic skew",
			maxDriftRatePPM: 50,
			wantHoldover:    50000,
		},
		{
			// 0 is legal: no floor, so the reply's own 0.005 PPM skew stands
			name:            "a zero floor leaves the skew alone",
			maxDriftRatePPM: 0,
			wantHoldover:    5,
		},
		{name: "NaN", maxDriftRatePPM: math.NaN(), wantErr: true},
		{name: "+Inf", maxDriftRatePPM: math.Inf(1), wantErr: true},
		{name: "-Inf", maxDriftRatePPM: math.Inf(-1), wantErr: true},
		{name: "negative", maxDriftRatePPM: -1, wantErr: true},
		{name: "finite but overflows the scaling", maxDriftRatePPM: math.MaxFloat64, wantErr: true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, holdoverNS, err := ChronyBound(healthyTracking(), tc.maxDriftRatePPM)
			if tc.wantErr {
				require.ErrorIs(t, err, errCorrectness)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantHoldover, holdoverNS)
		})
	}
}

func TestChronyBoundRejectsBadTracking(t *testing.T) {
	testCases := []struct {
		name   string
		mutate func(*chrony.Tracking)
	}{
		{
			name:   "chronyd has no usable source",
			mutate: func(tr *chrony.Tracking) { tr.LeapStatus = ntp.LeapAlarm },
		},
		{
			name:   "chronyd never synchronised, so reference time is unset",
			mutate: func(tr *chrony.Tracking) { tr.RefTime = time.Time{} },
		},
		{
			name:   "current correction is NaN",
			mutate: func(tr *chrony.Tracking) { tr.CurrentCorrection = math.NaN() },
		},
		{
			name:   "current correction is +Inf",
			mutate: func(tr *chrony.Tracking) { tr.CurrentCorrection = math.Inf(1) },
		},
		{
			// math.Abs turns this into the +Inf case, so the same check must catch it
			name:   "current correction is -Inf",
			mutate: func(tr *chrony.Tracking) { tr.CurrentCorrection = math.Inf(-1) },
		},
		{
			name:   "root dispersion is NaN",
			mutate: func(tr *chrony.Tracking) { tr.RootDispersion = math.NaN() },
		},
		{
			name:   "root dispersion is +Inf",
			mutate: func(tr *chrony.Tracking) { tr.RootDispersion = math.Inf(1) },
		},
		{
			name:   "root dispersion is negative",
			mutate: func(tr *chrony.Tracking) { tr.RootDispersion = -2 * time.Microsecond.Seconds() },
		},
		{
			name:   "root delay is NaN",
			mutate: func(tr *chrony.Tracking) { tr.RootDelay = math.NaN() },
		},
		{
			name:   "root delay is +Inf",
			mutate: func(tr *chrony.Tracking) { tr.RootDelay = math.Inf(1) },
		},
		{
			name:   "root delay is negative",
			mutate: func(tr *chrony.Tracking) { tr.RootDelay = -4 * time.Microsecond.Seconds() },
		},
		{
			// not part of the bound; it becomes the holdover rate
			name:   "skew is NaN",
			mutate: func(tr *chrony.Tracking) { tr.SkewPPM = math.NaN() },
		},
		{
			name:   "skew is +Inf",
			mutate: func(tr *chrony.Tracking) { tr.SkewPPM = math.Inf(1) },
		},
		{
			name:   "skew is negative",
			mutate: func(tr *chrony.Tracking) { tr.SkewPPM = -1 },
		},
		{
			// the bound is fine here; only the holdover scaling overflows
			name:   "skew is finite but overflows the holdover scaling",
			mutate: func(tr *chrony.Tracking) { tr.SkewPPM = math.MaxFloat64 },
		},
		{
			name:   "a finite term overflows to +Inf once scaled to ns",
			mutate: func(tr *chrony.Tracking) { tr.RootDispersion = math.MaxFloat64 },
		},
		{
			name:   "the scaled bound stays finite but is absurdly wide",
			mutate: func(tr *chrony.Tracking) { tr.RootDispersion = 1e10 },
		},
		{
			name: "error bound rounds down to zero",
			mutate: func(tr *chrony.Tracking) {
				tr.CurrentCorrection, tr.RootDispersion, tr.RootDelay = 0, 0, 0
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tracking := healthyTracking()
			tc.mutate(tracking)

			_, _, err := ChronyBound(tracking, testMaxDriftRatePPM)
			require.ErrorIs(t, err, errCorrectness)
		})
	}
}
