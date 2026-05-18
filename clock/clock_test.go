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

package clock

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestSetFreq(t *testing.T) {
	tx := &unix.Timex{}
	setFreq(tx, 1000.0)
	require.Equal(t, int64(65536), tx.Freq)

	setFreq(tx, -500.0)
	require.Equal(t, int64(-32768), tx.Freq)

	setFreq(tx, 0)
	require.Equal(t, int64(0), tx.Freq)
}

func TestSetTime(t *testing.T) {
	tx := &unix.Timex{}
	setTime(tx, 5*time.Second, 123*time.Nanosecond)
	require.Equal(t, int64(5*time.Second), tx.Time.Sec)
	require.Equal(t, int64(123*time.Nanosecond), tx.Time.Usec)
}

func TestSetTimeNegative(t *testing.T) {
	tx := &unix.Timex{}
	setTime(tx, -1*time.Second, -500*time.Nanosecond)
	require.Equal(t, int64(-1*time.Second), tx.Time.Sec)
	require.Equal(t, int64(-500*time.Nanosecond), tx.Time.Usec)
}

func TestFrequencyPPB(t *testing.T) {
	tx := &unix.Timex{}
	wantState, err := unix.Adjtimex(tx)
	require.NoError(t, err)

	freqPPB, state, err := FrequencyPPB(0)
	require.NoError(t, err)
	require.Equal(t, wantState, state)
	require.Equal(t, float64(tx.Freq)/PPBToTimexPPM, freqPPB)
}

func TestMaxFreqPPB(t *testing.T) {
	tx := &unix.Timex{}
	wantState, err := unix.Adjtimex(tx)
	require.NoError(t, err)

	maxFreq, state, err := MaxFreqPPB(0)
	require.NoError(t, err)
	require.Equal(t, wantState, state)
	require.Equal(t, 500000.0, maxFreq)
}

func TestFrequencyPPBInvalidClock(t *testing.T) {
	_, _, err := FrequencyPPB(-1)
	require.Error(t, err)
}

func TestMaxFreqPPBInvalidClock(t *testing.T) {
	_, _, err := MaxFreqPPB(-1)
	require.Error(t, err)
}
