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

package cmd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCalculateJitter(t *testing.T) {
	tests := []struct {
		name      string
		maxJitter time.Duration
	}{
		{
			name:      "zero jitter",
			maxJitter: 0,
		},
		{
			name:      "negative jitter",
			maxJitter: -time.Second,
		},
		{
			name:      "positive jitter",
			maxJitter: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jitter := CalculateJitter(tt.maxJitter)

			if tt.maxJitter <= 0 {
				require.Zero(t, jitter)
			} else {
				require.GreaterOrEqual(t, jitter, time.Duration(0))
				require.Less(t, jitter, tt.maxJitter)
			}
		})
	}
}

func TestCalculateJitterDistribution(t *testing.T) {
	maxJitter := 30 * time.Second
	seen := make(map[time.Duration]bool)

	// Run multiple times to verify randomness produces different values
	for range 100 {
		jitter := CalculateJitter(maxJitter)
		require.GreaterOrEqual(t, jitter, time.Duration(0))
		require.Less(t, jitter, maxJitter)
		seen[jitter] = true
	}

	// Verify that different values are produced (randomness check)
	require.Greater(t, len(seen), 1, "CalculateJitter should produce different values across multiple calls")
}
