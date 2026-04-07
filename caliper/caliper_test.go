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
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTestTOR creates a synthetic TOR file with 4 clear reflective peaks.
func buildTestTOR() *TORFile {
	ri := 1.4682
	n := 2000

	peakPositions := []float64{0.5, 3.0, 25.0, 26.5}
	peakAmplitudes := []float64{15.0, 8.0, 10.0, 5.0}

	points := make([]DataPoint, n)
	for i := range points {
		dist := float64(i) * 0.05
		amp := -30.0 - dist*0.2

		for j, pos := range peakPositions {
			d := dist - pos
			if math.Abs(d) < 0.3 {
				amp += peakAmplitudes[j] * math.Exp(-d*d/(2*0.01))
			}
		}
		points[i] = DataPoint{DistanceM: dist, AmplitudeDB: amp}
	}

	return &TORFile{
		DateTime:               time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC),
		InstrumentInfo:         "1310  nm - 1  ns -",
		Wavelength:             1310,
		PulseWidth:             1,
		RefractiveIndex:        ri,
		Resolution:             0.05,
		Start:                  0,
		End:                    100,
		Average:                30000,
		BackscatterCoefficient: -81.0,
		DataPoints:             points,
	}
}

func TestParseTOR(t *testing.T) {
	content := strings.Join([]string{
		"Some header line",
		"[DateTime]",
		"1742036400",
		"[-]",
		"[InstrumentInfo]",
		"1310  nm - 1  ns -",
		"[-]",
		"[Wavelength]",
		"0",
		"[-]",
		"[PulseWidth]",
		"1",
		"[-]",
		"[RefractiveIndex]",
		"1.4682",
		"[-]",
		"[Resolution]",
		"0.05",
		"[-]",
		"[Start]",
		"0",
		"[-]",
		"[End]",
		"100",
		"[-]",
		"[Average]",
		"30000",
		"[-]",
		"[BackscatterCoefficient]",
		"-81.0",
		"[-]",
		"[DataPoints]",
		"0.00\t-30.00",
		"0.05\t-30.01",
		"0.10\t-30.02",
		"[-]",
	}, "\n")

	dir := t.TempDir()
	path := filepath.Join(dir, "test.tor")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))

	tor, err := ParseTOR(path)
	require.NoError(t, err)

	assert.Equal(t, 1310, tor.Wavelength, "wavelength should be extracted from InstrumentInfo when field is 0")
	assert.Equal(t, 1.4682, tor.RefractiveIndex)
	assert.Equal(t, 1, tor.PulseWidth)
	assert.Len(t, tor.DataPoints, 3)
	assert.InDelta(t, 0.05, tor.DataPoints[1].DistanceM, 0.001)
	assert.InDelta(t, -30.01, tor.DataPoints[1].AmplitudeDB, 0.001)
}

func TestParseTORReader(t *testing.T) {
	content := strings.Join([]string{
		"[DateTime]",
		"1742036400",
		"[-]",
		"[InstrumentInfo]",
		"1550 nm",
		"OTDR module S/N: ABC123",
		"[-]",
		"[Wavelength]",
		"1550",
		"[-]",
		"[RefractiveIndex]",
		"1.4680",
		"[-]",
		"[DataPoints]",
		"0.00\t-25.00",
		"0.10\t-25.50",
		"[-]",
	}, "\n")

	tor, err := ParseTORReader(strings.NewReader(content))
	require.NoError(t, err)

	assert.Equal(t, 1550, tor.Wavelength)
	assert.Equal(t, "ABC123", tor.ModuleSerialNumber)
	assert.Equal(t, 1.468, tor.RefractiveIndex)
	assert.Len(t, tor.DataPoints, 2)
}

func TestParseTORNoDataPoints(t *testing.T) {
	content := "[DateTime]\n1742036400\n[-]\n[RefractiveIndex]\n1.4682\n[-]\n"
	_, err := ParseTORReader(strings.NewReader(content))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no data points")
}

func TestParseTORFileNotFound(t *testing.T) {
	_, err := ParseTOR("/nonexistent/path/test.tor")
	assert.Error(t, err)
}

func TestParseWavelengthFromInfo(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		ok       bool
	}{
		{"1310  nm - 1  ns -", 1310, true},
		{"1550 nm", 1550, true},
		{"850nm", 850, true},
		{"no wavelength here", 0, false},
		{"", 0, false},
		{"abc nm", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			wl, ok := parseWavelengthFromInfo(tt.input)
			assert.Equal(t, tt.ok, ok)
			if ok {
				assert.Equal(t, tt.expected, wl)
			}
		})
	}
}

func TestDataPointTimeNs(t *testing.T) {
	dp := DataPoint{DistanceM: 100.0, AmplitudeDB: -30.0}
	ri := 1.4682

	expected := 100.0 * ri / SpeedOfLight * 1e9
	assert.InDelta(t, expected, dp.TimeNs(ri), 0.0001)
}

func TestDetectPeaks(t *testing.T) {
	tor := buildTestTOR()
	peaks, err := DetectPeaks(tor, 0)
	require.NoError(t, err)
	require.Len(t, peaks, 4)

	for i, label := range []string{"OA", "OB", "OC", "OD"} {
		assert.Equal(t, label, peaks[i].Label)
	}

	expectedDists := []float64{0.5, 3.0, 25.0, 26.5}
	for i, p := range peaks {
		assert.InDelta(t, expectedDists[i], p.DistanceM, 0.15, "peak %s distance", p.Label)
	}

	ri := tor.RefractiveIndex
	for _, p := range peaks {
		expectedTime := p.DistanceM * ri / SpeedOfLight * 1e9
		assert.InDelta(t, expectedTime, p.TimeNs, 0.001)
	}
}

func TestDetectPeaksTapSplitter(t *testing.T) {
	ri := 1.4682
	n := 2000

	peakPositions := []float64{0.5, 1.0, 3.0, 25.0, 26.5}
	peakAmplitudes := []float64{15.0, 12.0, 8.0, 10.0, 5.0}

	points := make([]DataPoint, n)
	for i := range points {
		dist := float64(i) * 0.05
		amp := -30.0 - dist*0.2
		for j, pos := range peakPositions {
			d := dist - pos
			if math.Abs(d) < 0.3 {
				amp += peakAmplitudes[j] * math.Exp(-d*d/(2*0.01))
			}
		}
		points[i] = DataPoint{DistanceM: dist, AmplitudeDB: amp}
	}

	tor := &TORFile{
		RefractiveIndex: ri,
		DataPoints:      points,
	}

	peaks, err := DetectPeaks(tor, 0)
	require.NoError(t, err)
	require.Len(t, peaks, 4)

	assert.Equal(t, "OA", peaks[0].Label)
	assert.InDelta(t, 0.5, peaks[0].DistanceM, 0.15)
	assert.Equal(t, "OB", peaks[1].Label)
	assert.InDelta(t, 3.0, peaks[1].DistanceM, 0.15)
	assert.Equal(t, "OC", peaks[2].Label)
	assert.Equal(t, "OD", peaks[3].Label)
}

func TestDetectPeaksLaunchCableFilter(t *testing.T) {
	ri := 1.4682
	n := 2000

	// Place a peak at 2.0m (within a 3m launch cable), then real peaks at 4.0, 7.0, 28.0, 29.5
	peakPositions := []float64{2.0, 4.0, 7.0, 28.0, 29.5}
	peakAmplitudes := []float64{15.0, 15.0, 8.0, 10.0, 5.0}

	points := make([]DataPoint, n)
	for i := range points {
		dist := float64(i) * 0.05
		amp := -30.0 - dist*0.2
		for j, pos := range peakPositions {
			d := dist - pos
			if math.Abs(d) < 0.3 {
				amp += peakAmplitudes[j] * math.Exp(-d*d/(2*0.01))
			}
		}
		points[i] = DataPoint{DistanceM: dist, AmplitudeDB: amp}
	}

	tor := &TORFile{RefractiveIndex: ri, DataPoints: points}

	// With launch cable = 3m, the peak at 2.0m should be filtered
	peaks, err := DetectPeaks(tor, 3.0)
	require.NoError(t, err)
	require.Len(t, peaks, 4)
	assert.Equal(t, "OA", peaks[0].Label)
	assert.InDelta(t, 4.0, peaks[0].DistanceM, 0.15, "OA should be at 4.0m, not 2.0m")
}

func TestDetectPeaksInsufficientData(t *testing.T) {
	tor := &TORFile{
		RefractiveIndex: 1.4682,
		DataPoints:      make([]DataPoint, 5),
	}
	_, err := DetectPeaks(tor, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient data points")
}

func TestComputeResult(t *testing.T) {
	tor := buildTestTOR()
	peaks, err := DetectPeaks(tor, 0)
	require.NoError(t, err)

	result, err := ComputeResult(tor, peaks, "gnss01.cln1", "PF000142", GNSSoF16RxE, Gen2Phase2, 1.5, 3.0)
	require.NoError(t, err)

	assert.Equal(t, "gnss01.cln1", result.DeviceName)
	assert.Equal(t, "PF000142", result.SerialNumber)
	assert.Equal(t, GNSSoF16RxE, result.Model)
	assert.Equal(t, Gen2Phase2, result.AntennaGen)

	assert.Equal(t, 2.0, result.Delays.SMAPortOffsetNs)
	assert.Equal(t, 39.3, result.Delays.AntennaElectricalDelayNs)
	assert.Greater(t, result.Delays.RxDelayNs, 0.0)
	assert.Greater(t, result.Delays.CableDelayNs, result.Delays.RxDelayNs)

	assert.InDelta(t, 1.5*CoaxDelayNsPerM, result.Delays.CoaxCableDelayNs, 0.001)
	assert.Equal(t, 1.5, result.Delays.CoaxCableLengthM)

	expectedTotal := result.Delays.SMAPortOffsetNs +
		result.Delays.RxDelayNs +
		result.Delays.CableDelayNs +
		result.Delays.AntennaOpticalDelayNs +
		result.Delays.AntennaElectricalDelayNs
	assert.InDelta(t, expectedTotal, result.Delays.TotalDelayNs, 0.001)
	assert.InDelta(t, result.Delays.TotalDelayNs+result.Delays.CoaxCableDelayNs,
		result.Delays.EndToEndDelayNs, 0.001)

	assert.Len(t, result.Trace, len(tor.DataPoints))

	assert.Equal(t, 1310, result.Settings.Wavelength)
	assert.Equal(t, tor.RefractiveIndex, result.Settings.RefractiveIndex)
	assert.Equal(t, 3.0, result.Settings.LaunchCableLengthM)
}

func TestComputeResultModels(t *testing.T) {
	tests := []struct {
		model          ReceiverModel
		expectedSMANs  float64
		antennaGen     AntennaGen
		expectedElecNs float64
	}{
		{GNSSoF16RxE, 2.0, Gen2Phase0, 20.5},
		{GNSSoF16RxE, 2.0, Gen2Phase1, 20.5},
		{GNSSoF16RxE, 2.0, Gen2Phase2, 39.3},
		{GNSSPoF164RxE, 5.0, Gen2aPhase2, 39.3},
	}

	tor := buildTestTOR()
	peaks, err := DetectPeaks(tor, 0)
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(string(tt.model)+"_"+string(tt.antennaGen), func(t *testing.T) {
			result, err := ComputeResult(tor, peaks, "test", "PF000001", tt.model, tt.antennaGen, 0, 0)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedSMANs, result.Delays.SMAPortOffsetNs)
			assert.Equal(t, tt.expectedElecNs, result.Delays.AntennaElectricalDelayNs)
		})
	}
}

func TestComputeResultMissingPeaks(t *testing.T) {
	tor := buildTestTOR()
	peaks := []Peak{
		{Label: "OA", TimeNs: 1.0},
		{Label: "OB", TimeNs: 2.0},
		{Label: "OC", TimeNs: 3.0},
	}
	_, err := ComputeResult(tor, peaks, "test", "PF000001", GNSSoF16RxE, Gen2Phase2, 0, 0)
	assert.Error(t, err)
}

func TestComputeResultZeroCoax(t *testing.T) {
	tor := buildTestTOR()
	peaks, err := DetectPeaks(tor, 0)
	require.NoError(t, err)

	result, err := ComputeResult(tor, peaks, "test", "PF000001", GNSSoF16RxE, Gen2Phase2, 0, 0)
	require.NoError(t, err)

	assert.Equal(t, 0.0, result.Delays.CoaxCableLengthM)
	assert.Equal(t, 0.0, result.Delays.CoaxCableDelayNs)
	assert.Equal(t, result.Delays.TotalDelayNs, result.Delays.EndToEndDelayNs)
}

func TestComputeResultUnknownModel(t *testing.T) {
	tor := buildTestTOR()
	peaks, err := DetectPeaks(tor, 0)
	require.NoError(t, err)

	_, err = ComputeResult(tor, peaks, "test", "PF000001", "UnknownModel", Gen2Phase2, 0, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown receiver model")
}

func TestComputeResultUnknownAntennaGen(t *testing.T) {
	tor := buildTestTOR()
	peaks, err := DetectPeaks(tor, 0)
	require.NoError(t, err)

	_, err = ComputeResult(tor, peaks, "test", "PF000001", GNSSoF16RxE, "unknown-gen", 0, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown antenna generation")
}

func TestComputeResultJSONRoundTrip(t *testing.T) {
	tor := buildTestTOR()
	peaks, err := DetectPeaks(tor, 0)
	require.NoError(t, err)

	result, err := ComputeResult(tor, peaks, "gnss01.cln1", "PF000142", GNSSoF16RxE, Gen2Phase2, 1.5, 3.0)
	require.NoError(t, err)

	data, err := json.MarshalIndent(result, "", "  ")
	require.NoError(t, err)

	var decoded MeasurementResult
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, result.DeviceName, decoded.DeviceName)
	assert.Equal(t, result.SerialNumber, decoded.SerialNumber)
	assert.Equal(t, result.Model, decoded.Model)
	assert.Equal(t, result.AntennaGen, decoded.AntennaGen)
	assert.Equal(t, result.DateTime, decoded.DateTime)
	assert.Equal(t, result.Settings.LaunchCableLengthM, decoded.Settings.LaunchCableLengthM)
	assert.Equal(t, result.Settings.Wavelength, decoded.Settings.Wavelength)
	assert.Equal(t, result.Settings.RefractiveIndex, decoded.Settings.RefractiveIndex)
	assert.Len(t, decoded.Peaks, 4)
	assert.InDelta(t, result.Delays.EndToEndDelayNs, decoded.Delays.EndToEndDelayNs, 0.001)
	assert.Len(t, decoded.Trace, len(result.Trace))
}

func TestAntennaElectricalDelayNs(t *testing.T) {
	tests := []struct {
		gen      AntennaGen
		expected float64
		wantErr  bool
	}{
		{Gen2Phase0, 20.5, false},
		{Gen2Phase1, 20.5, false},
		{Gen2Phase2, 39.3, false},
		{Gen2aPhase2, 39.3, false},
		{"unknown", 0, true},
	}
	for _, tt := range tests {
		t.Run(string(tt.gen), func(t *testing.T) {
			got, err := AntennaElectricalDelayNs(tt.gen)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

func TestSMAPortOffsetNs(t *testing.T) {
	tests := []struct {
		model    ReceiverModel
		expected float64
		wantErr  bool
	}{
		{GNSSoF16RxE, 2.0, false},
		{GNSSPoF164RxE, 5.0, false},
		{"unknown", 0, true},
	}
	for _, tt := range tests {
		t.Run(string(tt.model), func(t *testing.T) {
			got, err := SMAPortOffsetNs(tt.model)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

func TestPeakDescriptions(t *testing.T) {
	rxeDescs := PeakDescription(GNSSoF16RxE)
	assert.Contains(t, rxeDescs["OA"], "FO Out")
	assert.Contains(t, rxeDescs["OB"], "Q-ODC-12")

	pofDescs := PeakDescription(GNSSPoF164RxE)
	assert.Contains(t, pofDescs["OA"], "FO Out")
	assert.Contains(t, pofDescs["OB"], "LC connector")
}

func TestGenerateSVG(t *testing.T) {
	tor := buildTestTOR()
	peaks, err := DetectPeaks(tor, 0)
	require.NoError(t, err)

	result, err := ComputeResult(tor, peaks, "gnss01.cln1", "PF000142", GNSSoF16RxE, Gen2Phase2, 0, 0)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = GenerateSVG(tor, peaks, result, &buf)
	require.NoError(t, err)

	svg := buf.String()
	assert.True(t, strings.HasPrefix(svg, "<svg"))
	assert.True(t, strings.HasSuffix(strings.TrimSpace(svg), "</svg>"))

	for _, p := range peaks {
		assert.Contains(t, svg, p.Label)
	}
	assert.Contains(t, svg, "gnss01.cln1")
	assert.Contains(t, svg, "GNSSoF16-RxE")
}

func TestGenerateSVGZoomed(t *testing.T) {
	tor := buildTestTOR()
	peaks, err := DetectPeaks(tor, 0)
	require.NoError(t, err)

	result, err := ComputeResult(tor, peaks, "test", "PF000001", GNSSoF16RxE, Gen2Phase2, 0, 0)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = GenerateSVGZoomed(tor, peaks, result, &buf, 50)
	require.NoError(t, err)

	svg := buf.String()
	assert.Contains(t, svg, "first 50 ns")
	assert.Contains(t, svg, "<svg")
}

func TestGenerateSVGWindow(t *testing.T) {
	tor := buildTestTOR()
	peaks, err := DetectPeaks(tor, 0)
	require.NoError(t, err)

	result, err := ComputeResult(tor, peaks, "test", "PF000001", GNSSoF16RxE, Gen2Phase2, 0, 0)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = GenerateSVGWindow(tor, peaks, result, &buf, 100, 150)
	require.NoError(t, err)

	svg := buf.String()
	assert.Contains(t, svg, "100-150 ns")
	assert.Contains(t, svg, "<svg")
}

func TestGenerateSVGNilTOR(t *testing.T) {
	var buf bytes.Buffer
	err := GenerateSVG(nil, nil, &MeasurementResult{}, &buf)
	assert.Error(t, err)
}

func TestGenerateSVGNilResult(t *testing.T) {
	tor := buildTestTOR()
	var buf bytes.Buffer
	err := GenerateSVG(tor, nil, nil, &buf)
	assert.Error(t, err)
}

func TestGenerateSVGEmptyDataPoints(t *testing.T) {
	tor := &TORFile{RefractiveIndex: 1.4682}
	var buf bytes.Buffer
	err := GenerateSVG(tor, nil, &MeasurementResult{}, &buf)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no data points")
}
