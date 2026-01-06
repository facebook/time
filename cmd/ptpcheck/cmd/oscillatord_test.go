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
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/facebook/time/oscillatord"
)

func TestPrintOscillatord(t *testing.T) {
	tests := []struct {
		name           string
		status         *oscillatord.Status
		expectedOutput []string
	}{
		{
			name: "basic status with offset in nanoseconds",
			status: &oscillatord.Status{
				Oscillator: oscillatord.Oscillator{
					Model:       "sa3x",
					FineCtrl:    100,
					CoarseCtrl:  50,
					Lock:        true,
					Temperature: 45.5,
				},
				GNSS: oscillatord.GNSS{
					Fix:             oscillatord.Fix3D,
					FixOK:           true,
					AntennaPower:    oscillatord.AntPowerOn,
					AntennaStatus:   oscillatord.AntStatusOK,
					LSChange:        oscillatord.LeapNoWarning,
					LeapSeconds:     18,
					SatellitesCount: 10,
					TimeAccuracy:    13,
				},
				Clock: oscillatord.Clock{
					Class:  oscillatord.ClockClassLock,
					Offset: 12345,
				},
			},
			expectedOutput: []string{
				"Oscillator:",
				"\tmodel: sa3x",
				"\tfine_ctrl: 100",
				"\tcoarse_ctrl: 50",
				"\tlock: true",
				"\ttemperature: 45.50C",
				"GNSS:",
				"\tfix: 3D (5)",
				"\tfixOk: true",
				"\tantenna_power: ON (1)",
				"\tantenna_status: OK (2)",
				"\tleap_second_change: NO WARNING (0)",
				"\tleap_seconds: 18",
				"\tsatellites_count: 10",
				"\ttime_accuracy: 13",
				"Clock:",
				"\tclass: Lock (6)",
				"\toffset: 12345ns",
			},
		},
		{
			name: "negative offset in nanoseconds",
			status: &oscillatord.Status{
				Oscillator: oscillatord.Oscillator{
					Model:       "sa5x",
					FineCtrl:    0,
					CoarseCtrl:  0,
					Lock:        false,
					Temperature: 25.0,
				},
				GNSS: oscillatord.GNSS{
					Fix:             oscillatord.FixNoFix,
					FixOK:           false,
					AntennaPower:    oscillatord.AntPowerOff,
					AntennaStatus:   oscillatord.AntStatusOpen,
					LSChange:        oscillatord.LeapAddSecond,
					LeapSeconds:     37,
					SatellitesCount: 0,
					TimeAccuracy:    0,
				},
				Clock: oscillatord.Clock{
					Class:  oscillatord.ClockClassHoldover,
					Offset: -265095,
				},
			},
			expectedOutput: []string{
				"\toffset: -265095ns",
			},
		},
		{
			name: "zero offset in nanoseconds",
			status: &oscillatord.Status{
				Oscillator: oscillatord.Oscillator{
					Model:       "test",
					FineCtrl:    0,
					CoarseCtrl:  0,
					Lock:        true,
					Temperature: 30.0,
				},
				GNSS: oscillatord.GNSS{
					Fix:             oscillatord.Fix2D,
					FixOK:           true,
					AntennaPower:    oscillatord.AntPowerOn,
					AntennaStatus:   oscillatord.AntStatusOK,
					LSChange:        oscillatord.LeapNoWarning,
					LeapSeconds:     18,
					SatellitesCount: 5,
					TimeAccuracy:    10,
				},
				Clock: oscillatord.Clock{
					Class:  oscillatord.ClockClassCalibrating,
					Offset: 0,
				},
			},
			expectedOutput: []string{
				"\toffset: 0ns",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, err := os.Pipe()
			require.NoError(t, err)
			os.Stdout = w

			printOscillatord(tt.status)

			// Restore stdout and read captured output
			w.Close()
			os.Stdout = oldStdout
			var buf bytes.Buffer
			_, err = io.Copy(&buf, r)
			require.NoError(t, err)
			output := buf.String()

			// Verify expected output is present
			for _, expected := range tt.expectedOutput {
				require.True(t, strings.Contains(output, expected),
					"Expected output to contain %q, got:\n%s", expected, output)
			}
		})
	}
}

func TestPrintOscillatordOffsetFormat(t *testing.T) {
	// Specific test to verify offset has "ns" unit suffix
	status := &oscillatord.Status{
		Oscillator: oscillatord.Oscillator{
			Model:       "test",
			FineCtrl:    0,
			CoarseCtrl:  0,
			Lock:        true,
			Temperature: 25.0,
		},
		GNSS: oscillatord.GNSS{
			Fix:             oscillatord.Fix3D,
			FixOK:           true,
			AntennaPower:    oscillatord.AntPowerOn,
			AntennaStatus:   oscillatord.AntStatusOK,
			LSChange:        oscillatord.LeapNoWarning,
			LeapSeconds:     18,
			SatellitesCount: 8,
			TimeAccuracy:    15,
		},
		Clock: oscillatord.Clock{
			Class:  oscillatord.ClockClassLock,
			Offset: 42,
		},
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	printOscillatord(status)

	// Restore stdout and read captured output
	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	output := buf.String()

	// Verify offset line has "ns" suffix
	require.Contains(t, output, "\toffset: 42ns\n",
		"Offset should include 'ns' unit suffix")

	// Verify offset line does NOT have just the number without units
	require.NotContains(t, output, "\toffset: 42\n",
		"Offset should not be printed without units")
}
