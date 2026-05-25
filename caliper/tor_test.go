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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTORReaderEmpty(t *testing.T) {
	_, err := ParseTORReader(strings.NewReader(""))
	require.Error(t, err)
	require.Contains(t, err.Error(), "no data points")
}

func TestParseTORReaderMissingHeader(t *testing.T) {
	// No [DateTime] header anywhere; the header-skip loop will exhaust the
	// scanner without entering the section parser.
	content := strings.Join([]string{
		"Some random text",
		"not a section",
		"still nothing",
	}, "\n")
	_, err := ParseTORReader(strings.NewReader(content))
	require.Error(t, err)
	require.Contains(t, err.Error(), "no data points")
}

func TestParseTORReaderValid(t *testing.T) {
	// Exercises every section the parser knows about (including the ones
	// not touched by TestParseTOR/TestParseTORReader in caliper_test.go:
	// CableID, FiberID, FiberType, DistanceUnit, DistanceRange,
	// LossThreshold, ReflectanceThreshold, EndOfFiberThreshold,
	// HighResolution) plus the [-] section terminator and a blank line
	// inside the empty-section state.
	content := strings.Join([]string{
		"header noise",
		"more noise",
		"[DateTime]",
		"1742036400",
		"[-]",
		"",
		"[InstrumentInfo]",
		"1310  nm - 1  ns -",
		"OTDR module S/N: SN-XYZ",
		"[-]",
		"[CableID]",
		"cable-1",
		"[-]",
		"[FiberID]",
		"fiber-7",
		"[-]",
		"[FiberType]",
		"2",
		"[-]",
		"[Wavelength]",
		"1310",
		"[-]",
		"[PulseWidth]",
		"1",
		"[-]",
		"[RefractiveIndex]",
		"1.4682",
		"[-]",
		"[DistanceUnit]",
		"1",
		"[-]",
		"[Average]",
		"30000",
		"[-]",
		"[End]",
		"100",
		"[-]",
		"[Start]",
		"0",
		"[-]",
		"[Resolution]",
		"0.05",
		"[-]",
		"[DistanceRange]",
		"100",
		"[-]",
		"[LossThreshold]",
		"0.05",
		"[-]",
		"[ReflectanceThreshold]",
		"-40.0",
		"[-]",
		"[EndOfFiberThreshold]",
		"5.0",
		"[-]",
		"[BackscatterCoefficient]",
		"-81.0",
		"[-]",
		"[HighResolution]",
		"1",
		"[-]",
		"[DataPoints]",
		"0.00\t-30.00",
		"0.05\t-30.01",
		"[-]",
	}, "\n")

	tor, err := ParseTORReader(strings.NewReader(content))
	require.NoError(t, err)

	require.Equal(t, "1310  nm - 1  ns -", tor.InstrumentInfo)
	require.Equal(t, "SN-XYZ", tor.ModuleSerialNumber)
	require.Equal(t, "cable-1", tor.CableID)
	require.Equal(t, "fiber-7", tor.FiberID)
	require.Equal(t, 2, tor.FiberType)
	require.Equal(t, 1310, tor.Wavelength)
	require.Equal(t, 1, tor.PulseWidth)
	require.InDelta(t, 1.4682, tor.RefractiveIndex, 1e-9)
	require.Equal(t, 1, tor.DistanceUnit)
	require.Equal(t, 30000, tor.Average)
	require.InDelta(t, 100.0, tor.End, 1e-9)
	require.InDelta(t, 0.0, tor.Start, 1e-9)
	require.InDelta(t, 0.05, tor.Resolution, 1e-9)
	require.Equal(t, 100, tor.DistanceRange)
	require.InDelta(t, 0.05, tor.LossThreshold, 1e-9)
	require.InDelta(t, -40.0, tor.ReflectanceThreshold, 1e-9)
	require.InDelta(t, 5.0, tor.EndOfFiberThreshold, 1e-9)
	require.InDelta(t, -81.0, tor.BackscatterCoefficient, 1e-9)
	require.Equal(t, 1, tor.HighResolution)
	require.Len(t, tor.DataPoints, 2)
}

func TestParseTORReaderInvalidDateTime(t *testing.T) {
	content := strings.Join([]string{
		"[DateTime]",
		"not-a-timestamp",
		"[-]",
	}, "\n")
	_, err := ParseTORReader(strings.NewReader(content))
	require.Error(t, err)
	require.Contains(t, err.Error(), "parsing DateTime")
}

func TestParseTORReaderInvalidValues(t *testing.T) {
	// Every numeric section has a strconv parse step that returns a wrapped
	// error; this table exercises each branch with a non-numeric value.
	// DateTime is covered separately in TestParseTORReaderInvalidDateTime
	// since the bad value replaces the DateTime payload itself.
	tests := []struct {
		name    string
		section string
		value   string
		wantMsg string
	}{
		{"FiberType", "FiberType", "x", "parsing FiberType"},
		{"Wavelength", "Wavelength", "x", "parsing Wavelength"},
		{"PulseWidth", "PulseWidth", "x", "parsing PulseWidth"},
		{"RefractiveIndex", "RefractiveIndex", "x", "parsing RefractiveIndex"},
		{"DistanceUnit", "DistanceUnit", "x", "parsing DistanceUnit"},
		{"Average", "Average", "x", "parsing Average"},
		{"End", "End", "x", "parsing End"},
		{"Start", "Start", "x", "parsing Start"},
		{"Resolution", "Resolution", "x", "parsing Resolution"},
		{"DistanceRange", "DistanceRange", "x", "parsing DistanceRange"},
		{"LossThreshold", "LossThreshold", "x", "parsing LossThreshold"},
		{"ReflectanceThreshold", "ReflectanceThreshold", "x", "parsing ReflectanceThreshold"},
		{"EndOfFiberThreshold", "EndOfFiberThreshold", "x", "parsing EndOfFiberThreshold"},
		{"BackscatterCoefficient", "BackscatterCoefficient", "x", "parsing BackscatterCoefficient"},
		{"HighResolution", "HighResolution", "x", "parsing HighResolution"},
		{"DataPoints", "DataPoints", "bad-line", "parsing data point"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := strings.Join([]string{
				"[DateTime]",
				"1742036400",
				"[-]",
				"[" + tt.section + "]",
				tt.value,
				"[-]",
			}, "\n")
			_, err := ParseTORReader(strings.NewReader(content))
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

func TestParseDataPointMalformed(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantMsg string
	}{
		{"empty line", "", "invalid data point"},
		{"single field no tab", "1.0", "invalid data point"},
		{"three fields", "1.0\t2.0\t3.0", "invalid data point"},
		{"non-numeric distance", "abc\t-30.0", "parsing distance"},
		{"non-numeric amplitude", "1.0\txyz", "parsing amplitude"},
		{"empty distance", "\t-30.0", "parsing distance"},
		{"empty amplitude", "1.0\t", "parsing amplitude"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseDataPoint(tt.line)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

func TestParseDataPointValid(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantDM  float64
		wantAmp float64
	}{
		{"basic", "1.5\t-30.25", 1.5, -30.25},
		{"zero distance", "0\t-25.0", 0.0, -25.0},
		{"whitespace around fields", " 2.0 \t -10.5 ", 2.0, -10.5},
		{"negative distance", "-1.0\t-30.0", -1.0, -30.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dp, err := parseDataPoint(tt.line)
			require.NoError(t, err)
			require.InDelta(t, tt.wantDM, dp.DistanceM, 1e-9)
			require.InDelta(t, tt.wantAmp, dp.AmplitudeDB, 1e-9)
		})
	}
}
