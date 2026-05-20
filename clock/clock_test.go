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

	"github.com/facebook/time/phc/unix"
	"github.com/stretchr/testify/require"
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

func TestFrequencyPPBInvalidClock(t *testing.T) {
	_, _, err := FrequencyPPB(-1)
	require.Error(t, err)
}

func TestMaxFreqPPBInvalidClock(t *testing.T) {
	_, _, err := MaxFreqPPB(-1)
	require.Error(t, err)
}
