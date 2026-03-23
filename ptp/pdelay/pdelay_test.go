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

package pdelay

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResult_PathDelay(t *testing.T) {
	tests := []struct {
		name     string
		result   Result
		expected time.Duration
	}{
		{
			name: "valid timestamps no CF",
			result: Result{
				T1: time.Unix(1000, 0),
				T2: time.Unix(1000, 500000),  // 500µs later
				T3: time.Unix(1000, 1000000), // 1ms later
				T4: time.Unix(1000, 1500000), // 1.5ms later
			},
			// PathDelay = ((T2 - T1 - CFReq) + (T4 - T3 - CFResp)) / 2
			// = ((0.5ms - 0) + (0.5ms - 0)) / 2
			// = 0.5ms
			expected: 500 * time.Microsecond,
		},
		{
			name: "valid timestamps with CF",
			result: Result{
				T1:                  time.Unix(1000, 0),
				T2:                  time.Unix(1000, 500000),  // 500µs later
				T3:                  time.Unix(1000, 1000000), // 1ms later
				T4:                  time.Unix(1000, 1500000), // 1.5ms later
				CorrectionFieldReq:  100 * time.Microsecond,   // 100µs TC residence on req path
				CorrectionFieldResp: 100 * time.Microsecond,   // 100µs TC residence on resp path
			},
			// PathDelay = ((T2 - T1 - CFReq) + (T4 - T3 - CFResp)) / 2
			// = ((0.5ms - 0.1ms) + (0.5ms - 0.1ms)) / 2
			// = (0.4ms + 0.4ms) / 2
			// = 0.4ms
			expected: 400 * time.Microsecond,
		},
		{
			name: "zero timestamps",
			result: Result{
				T1: time.Time{},
				T2: time.Time{},
				T3: time.Time{},
				T4: time.Time{},
			},
			expected: 0,
		},
		{
			name: "partial timestamps",
			result: Result{
				T1: time.Unix(1000, 0),
				T2: time.Unix(1000, 500000),
				T3: time.Time{},
				T4: time.Time{},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.PathDelay()
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestResult_Offset(t *testing.T) {
	tests := []struct {
		name     string
		result   Result
		expected time.Duration
	}{
		{
			name: "zero offset (symmetric) no CF",
			result: Result{
				T1: time.Unix(1000, 0),
				T2: time.Unix(1000, 500000),  // 500µs later
				T3: time.Unix(1000, 1000000), // 1ms later
				T4: time.Unix(1000, 1500000), // 1.5ms later
			},
			// Offset = ((T2 - T1 - CFReq) - (T4 - T3 - CFResp)) / 2
			// = ((0.5ms - 0) - (0.5ms - 0)) / 2
			// = 0
			expected: 0,
		},
		{
			name: "positive offset (remote ahead) no CF",
			result: Result{
				T1: time.Unix(1000, 0),
				T2: time.Unix(1000, 1000000), // 1ms later (remote sees it later)
				T3: time.Unix(1000, 1500000), // 1.5ms later
				T4: time.Unix(1000, 2000000), // 2ms later
			},
			// Offset = ((T2 - T1 - CFReq) - (T4 - T3 - CFResp)) / 2
			// = ((1ms - 0) - (0.5ms - 0)) / 2
			// = 0.25ms
			expected: 250 * time.Microsecond,
		},
		{
			name: "offset with asymmetric CF",
			result: Result{
				T1:                  time.Unix(1000, 0),
				T2:                  time.Unix(1000, 500000),  // 500µs later
				T3:                  time.Unix(1000, 1000000), // 1ms later
				T4:                  time.Unix(1000, 1500000), // 1.5ms later
				CorrectionFieldReq:  200 * time.Microsecond,   // 200µs TC residence on req path
				CorrectionFieldResp: 100 * time.Microsecond,   // 100µs TC residence on resp path
			},
			// Offset = ((T2 - T1 - CFReq) - (T4 - T3 - CFResp)) / 2
			// = ((0.5ms - 0.2ms) - (0.5ms - 0.1ms)) / 2
			// = (0.3ms - 0.4ms) / 2
			// = -0.05ms = -50µs
			expected: -50 * time.Microsecond,
		},
		{
			name: "zero timestamps",
			result: Result{
				T1: time.Time{},
				T2: time.Time{},
				T3: time.Time{},
				T4: time.Time{},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.Offset()
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestResult_Valid(t *testing.T) {
	tests := []struct {
		name     string
		result   Result
		expected bool
	}{
		{
			name: "all timestamps set",
			result: Result{
				T1: time.Unix(1000, 0),
				T2: time.Unix(1000, 500000),
				T3: time.Unix(1000, 1000000),
				T4: time.Unix(1000, 1500000),
			},
			expected: true,
		},
		{
			name: "missing T1",
			result: Result{
				T1: time.Time{},
				T2: time.Unix(1000, 500000),
				T3: time.Unix(1000, 1000000),
				T4: time.Unix(1000, 1500000),
			},
			expected: false,
		},
		{
			name: "missing T4",
			result: Result{
				T1: time.Unix(1000, 0),
				T2: time.Unix(1000, 500000),
				T3: time.Unix(1000, 1000000),
				T4: time.Time{},
			},
			expected: false,
		},
		{
			name:     "all zero",
			result:   Result{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.Valid()
			require.Equal(t, tt.expected, got)
		})
	}
}
