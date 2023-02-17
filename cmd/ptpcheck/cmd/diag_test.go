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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/facebook/time/cmd/ptpcheck/checker"
	ptp "github.com/facebook/time/ptp/protocol"
)

func TestCheckAgainstThreshold(t *testing.T) {
	tests := []struct {
		testName      string
		name          string
		value         time.Duration
		warnThreshold time.Duration
		failThreshold time.Duration
		explanation   string
		failOnZero    bool
		wantStatus    status
		wantMsg       string
	}{
		{
			testName:      "below threshold",
			name:          "Period since last ingress",
			value:         time.Millisecond,
			warnThreshold: time.Second,
			failThreshold: 10 * time.Second,
			explanation:   "We expect to receive SYNC messages from GM very often",
			failOnZero:    false,
			wantStatus:    OK,
			wantMsg:       "Period since last ingress is 1ms, we expect it to be within 1s",
		},
		{
			testName:      "warn threshold",
			name:          "Period since last ingress",
			value:         2 * time.Second,
			warnThreshold: time.Second,
			failThreshold: 10 * time.Second,
			explanation:   "We expect to receive SYNC messages from GM very often",
			failOnZero:    false,
			wantStatus:    WARN,
			wantMsg:       "Period since last ingress is 2s, we expect it to be within 1s. We expect to receive SYNC messages from GM very often",
		},
		{
			testName:      "fail threshold",
			name:          "Period since last ingress",
			value:         20 * time.Second,
			warnThreshold: time.Second,
			failThreshold: 10 * time.Second,
			explanation:   "We expect to receive SYNC messages from GM very often",
			failOnZero:    false,
			wantStatus:    FAIL,
			wantMsg:       "Period since last ingress is 20s, we expect it to be within 1s. We expect to receive SYNC messages from GM very often",
		},
		{
			testName:      "fail on zero",
			name:          "GM mean path delay",
			value:         0,
			warnThreshold: 100 * time.Millisecond,
			failThreshold: 250 * time.Millisecond,
			explanation:   "Mean path delay is measured network delay between us and GM",
			failOnZero:    true,
			wantStatus:    FAIL,
			wantMsg:       "GM mean path delay is 0s, we expect it to be non-zero and within 100ms. Mean path delay is measured network delay between us and GM",
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			var (
				st  status
				msg string
			)
			if tt.failOnZero {
				st, msg = checkAgainstThresholdNonZero(
					tt.name,
					tt.value,
					tt.warnThreshold,
					tt.failThreshold,
					tt.explanation,
				)
			} else {
				st, msg = checkAgainstThreshold(
					tt.name,
					tt.value,
					tt.warnThreshold,
					tt.failThreshold,
					tt.explanation,
				)
			}
			require.Equal(t, tt.wantStatus, st)
			require.Equal(t, tt.wantMsg, msg)
		})
	}

	// check with float now just to exercise generics
	t.Run("ints", func(t *testing.T) {
		st, msg := checkAgainstThreshold(
			"some int",
			28,
			10,
			100,
			"oh no",
		)
		require.Equal(t, WARN, st)
		require.Equal(t, "some int is 28, we expect it to be within 10. oh no", msg)
	})

	// check with float now just to exercise generics
	t.Run("floats", func(t *testing.T) {
		st, msg := checkAgainstThreshold(
			"some float",
			3.14,
			4.0,
			10.1,
			"oh no",
		)
		require.Equal(t, OK, st)
		require.Equal(t, "some float is 3.14, we expect it to be within 4", msg)
	})
}

func TestCheckGMPresent(t *testing.T) {
	r := &checker.PTPCheckResult{
		GrandmasterPresent: true,
	}
	st, msg := checkGMPresent(r)
	require.Equal(t, OK, st)
	require.Equal(t, "GM is present", msg)

	r.GrandmasterPresent = false
	st, msg = checkGMPresent(r)
	require.Equal(t, FAIL, st)
	require.Equal(t, "GM is not present", msg)
}

func TestCheckOffset(t *testing.T) {
	r := &checker.PTPCheckResult{
		OffsetFromMasterNS: 100.0,
	}
	st, msg := checkOffset(r)
	require.Equal(t, OK, st)
	require.Equal(t, "GM offset is 100ns, we expect it to be within 250µs", msg)

	r.OffsetFromMasterNS = 251000.0
	st, msg = checkOffset(r)
	require.Equal(t, WARN, st)
	require.Equal(t, "GM offset is 251µs, we expect it to be within 250µs. Offset is the difference between our clock and remote server (time error).", msg)

	r.OffsetFromMasterNS = -251000.0
	st, msg = checkOffset(r)
	require.Equal(t, WARN, st)
	require.Equal(t, "GM offset is 251µs, we expect it to be within 250µs. Offset is the difference between our clock and remote server (time error).", msg)
}

func TestCheckPathDelay(t *testing.T) {
	r := &checker.PTPCheckResult{
		MeanPathDelayNS: 100.0,
	}
	st, msg := checkPathDelay(r)
	require.Equal(t, OK, st)
	require.Equal(t, "GM mean path delay is 100ns, we expect it to be within 100ms", msg)

	r.MeanPathDelayNS = 151000000.0
	st, msg = checkPathDelay(r)
	require.Equal(t, WARN, st)
	require.Equal(t, "GM mean path delay is 151ms, we expect it to be within 100ms. Mean path delay is measured network delay between us and GM", msg)

	r.MeanPathDelayNS = -151000000.0
	st, msg = checkPathDelay(r)
	require.Equal(t, FAIL, st)
	require.Equal(t, "GM mean path delay is -151ms, we expect it to be positive and within 100ms. Mean path delay is measured network delay between us and GM", msg)
}

func TestExpandDiagnosers(t *testing.T) {
	r := &checker.PTPCheckResult{}
	extra := expandDiagnosers(r)
	require.Equal(t, 0, len(extra))
	r.PortServiceStats = &ptp.PortServiceStats{}
	extra = expandDiagnosers(r)
	require.Equal(t, 4, len(extra))
}

func TestPortServiceStatsDiagnosers(t *testing.T) {
	r := &checker.PTPCheckResult{}
	r.PortServiceStats = &ptp.PortServiceStats{
		SyncTimeout:      200,
		AnnounceTimeout:  10,
		SyncMismatch:     0,
		FollowupMismatch: 2000,
	}
	diagnosers := portServiceStatsDiagnosers(r)
	require.Equal(t, 4, len(diagnosers))

	st, msg := diagnosers[0](r)
	assert.Equal(t, WARN, st)
	assert.Equal(t, "Sync timeout count is 200, we expect it to be within 100. We expect to not skip sync packets", msg)
	st, msg = diagnosers[1](r)
	assert.Equal(t, OK, st)
	assert.Equal(t, "Announce timeout count is 10, we expect it to be within 100", msg)
	st, msg = diagnosers[2](r)
	assert.Equal(t, OK, st)
	assert.Equal(t, "Sync mismatch count is 0, we expect it to be within 100", msg)
	st, msg = diagnosers[3](r)
	assert.Equal(t, FAIL, st)
	assert.Equal(t, "FollowUp mismatch count is 2000, we expect it to be within 100. We expect FollowUp packets to arrive in correct order", msg)
}

func TestRunDiagnosers(t *testing.T) {
	toRun := []diagnoser{
		checkGMPresent,
		checkOffset,
		checkPathDelay,
	}
	r := &checker.PTPCheckResult{
		MeanPathDelayNS: 100.0,
	}
	exitCode := runDiagnosers(r, toRun)
	require.Equal(t, 2, exitCode)
	r.GrandmasterPresent = true
	exitCode = runDiagnosers(r, toRun)
	require.Equal(t, 1, exitCode)
	r.OffsetFromMasterNS = 100.0
	exitCode = runDiagnosers(r, toRun)
	require.Equal(t, 0, exitCode)
}
