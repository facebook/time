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
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

var testSample0 = &LogSample{
	MasterOffsetNS:          1.1,
	MasterOffsetMeanNS:      1.2,
	MasterOffsetStddevNS:    1.3,
	PathDelayNS:             1.4,
	PathDelayMeanNS:         1.5,
	PathDelayStddevNS:       1.6,
	FreqAdjustmentPPB:       1.7,
	FreqAdjustmentMeanPPB:   1.8,
	FreqAdjustmentStddevPPB: 1.9,
	MeasurementNS:           2.0,
	MeasurementMeanNS:       2.1,
	MeasurementStddevNS:     2.2,
	WindowNS:                2.3,
	ClockAccuracyMean:       25.1,
}

var testSample1 = &LogSample{
	MasterOffsetNS:          0.1,
	MasterOffsetMeanNS:      0.2,
	MasterOffsetStddevNS:    0.3,
	PathDelayNS:             0.4,
	PathDelayMeanNS:         0.5,
	PathDelayStddevNS:       0.6,
	FreqAdjustmentPPB:       0.7,
	FreqAdjustmentMeanPPB:   0.8,
	FreqAdjustmentStddevPPB: 0.9,
	MeasurementNS:           1.0,
	MeasurementMeanNS:       1.1,
	MeasurementStddevNS:     1.2,
	WindowNS:                1.3,
	ClockAccuracyMean:       100.1,
}

func TestShouldLog(t *testing.T) {
	require.False(t, shouldLog(0))
	require.True(t, shouldLog(1))
}

func TestLogSample_CSVRecords(t *testing.T) {
	got := testSample0.CSVRecords()
	want := []string{"1.1", "1.2", "1.3", "1.4", "1.5", "1.6", "1.7", "1.8", "1.9", "2", "2.1", "2.2", "2.3", "25.1"}

	// make sure we are in sync with header
	require.Equal(t, len(header), len(got))

	require.Equal(t, want, got)
}

func TestCSVLogger_Log(t *testing.T) {
	b := &bytes.Buffer{}
	l := NewCSVLogger(b, 1)

	err := l.Log(testSample0)
	require.NoError(t, err)
	err = l.Log(testSample1)
	require.NoError(t, err)
	err = l.Log(testSample0)
	require.NoError(t, err)

	got := b.String()
	want := `offset,offset_mean,offset_stddev,delay,delay_mean,delay_stddev,freq,freq_mean,freq_stddev,measurement,measurement_mean,measurement_stddev,window,clock_accuracy_mean
1.1,1.2,1.3,1.4,1.5,1.6,1.7,1.8,1.9,2,2.1,2.2,2.3,25.1
0.1,0.2,0.3,0.4,0.5,0.6,0.7,0.8,0.9,1,1.1,1.2,1.3,100.1
1.1,1.2,1.3,1.4,1.5,1.6,1.7,1.8,1.9,2,2.1,2.2,2.3,25.1
`

	require.Equal(t, want, got)
}
