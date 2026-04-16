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
	"fmt"
	"math"
	"time"
)

// AntennaGen represents antenna generation and phase
type AntennaGen string

const (
	Gen2Phase0  AntennaGen = "gen2-p0"
	Gen2Phase1  AntennaGen = "gen2-p1"
	Gen2Phase2  AntennaGen = "gen2-p2"
	Gen2aPhase2 AntennaGen = "gen2a-p2"
)

// AntennaElectricalDelayNs returns the electrical delay in nanoseconds for a given antenna generation
func AntennaElectricalDelayNs(gen AntennaGen) (float64, error) {
	switch gen {
	case Gen2Phase0, Gen2Phase1:
		return 20.5, nil
	case Gen2Phase2, Gen2aPhase2:
		return 39.3, nil
	default:
		return 0, fmt.Errorf("unknown antenna generation: %q", gen)
	}
}

// ReceiverModel represents the Huber-Suhner GNSS receiver model
type ReceiverModel string

const (
	GNSSoF16RxE   ReceiverModel = "GNSSoF16-RxE"
	GNSSPoF164RxE ReceiverModel = "GNSSPoF16-4RxE"
)

// SMAPortOffsetNs returns the delay in nanoseconds between the FO Out port
// (where OA is measured) and the SMA port (where downstream devices connect).
func SMAPortOffsetNs(model ReceiverModel) (float64, error) {
	switch model {
	case GNSSoF16RxE:
		return 2.0, nil
	case GNSSPoF164RxE:
		return 5.0, nil
	default:
		return 0, fmt.Errorf("unknown receiver model: %q", model)
	}
}

// PeakDescription returns the descriptions for OA/OB/OC/OD based on receiver model
func PeakDescription(model ReceiverModel) map[string]string {
	switch model {
	case GNSSPoF164RxE:
		return map[string]string{
			"OA": "FO Out",
			"OB": "FO In (LC connector)",
			"OC": "FC APC connector (antenna)",
			"OD": "Antenna optical isolator",
		}
	default: // GNSSoF16-RxE
		return map[string]string{
			"OA": "FO Out (front port)",
			"OB": "Q-ODC-12 connector (back of unit)",
			"OC": "Q-ODC-12 connector (antenna)",
			"OD": "Antenna optical isolator",
		}
	}
}

// TracePoint is a single data point in the OTDR trace for the report
type TracePoint struct {
	TimeNs      float64 `json:"time_ns"`
	AmplitudeDB float64 `json:"amplitude_db"`
}

// MeasurementResult is the detailed per-device JSON output
type MeasurementResult struct {
	DeviceName   string        `json:"device_name"`
	SerialNumber string        `json:"serial_number"`
	Model        ReceiverModel `json:"model"`
	AntennaGen   AntennaGen    `json:"antenna_gen"`
	TORFile      string        `json:"tor_file"`
	DateTime     string        `json:"date_time"`
	Settings     TORSettings   `json:"settings"`
	Peaks        []PeakResult  `json:"peaks"`
	Delays       DelayResult   `json:"delays"`
	Trace        []TracePoint  `json:"trace"`
}

// TORSettings captures the relevant OTDR measurement settings
type TORSettings struct {
	Wavelength             int     `json:"wavelength_nm"`
	PulseWidth             int     `json:"pulse_width"`
	RefractiveIndex        float64 `json:"refractive_index"`
	Resolution             float64 `json:"resolution_m"`
	Start                  float64 `json:"start_m"`
	End                    float64 `json:"end_m"`
	BackscatterCoefficient float64 `json:"backscatter_coefficient"`
	Average                int     `json:"average"`
	LaunchCableLengthM     float64 `json:"launch_cable_length_m"`
}

// PeakResult captures a single detected peak
type PeakResult struct {
	Label       string  `json:"label"`
	Description string  `json:"description"`
	DistanceM   float64 `json:"distance_m"`
	AmplitudeDB float64 `json:"amplitude_db"`
	TimeNs      float64 `json:"time_ns"`
}

// CoaxDelayNsPerM is the propagation delay of RG58 coaxial cable in ns/m
const CoaxDelayNsPerM = 5.05

// DelayResult captures the computed delays between peaks
type DelayResult struct {
	SMAPortOffsetNs          float64 `json:"sma_port_offset_ns"`
	RxDelayNs                float64 `json:"rx_delay_ns"`
	CableDelayNs             float64 `json:"cable_delay_ns"`
	AntennaOpticalDelayNs    float64 `json:"antenna_optical_delay_ns"`
	AntennaElectricalDelayNs float64 `json:"antenna_electrical_delay_ns"`
	TotalDelayNs             float64 `json:"total_delay_ns"`
	CoaxCableLengthM         float64 `json:"coax_cable_length_m"`
	CoaxCableDelayNs         float64 `json:"coax_cable_delay_ns"`
	EndToEndDelayNs          float64 `json:"end_to_end_delay_ns"`
}

// ComputeResult computes the full measurement result from parsed TOR data and peaks.
func ComputeResult(
	tor *TORFile,
	peaks []Peak,
	name, serial string,
	model ReceiverModel,
	antennaGen AntennaGen,
	coaxCableLengthM, launchCableLengthM float64,
) (*MeasurementResult, error) {
	if len(peaks) < 4 {
		return nil, fmt.Errorf("expected 4 peaks (OA, OB, OC, OD) but got %d", len(peaks))
	}
	for _, label := range []string{"OA", "OB", "OC", "OD"} {
		found := false
		for _, p := range peaks {
			if p.Label == label {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("missing expected peak %s", label)
		}
	}

	elecDelay, err := AntennaElectricalDelayNs(antennaGen)
	if err != nil {
		return nil, err
	}

	smaOffset, err := SMAPortOffsetNs(model)
	if err != nil {
		return nil, err
	}

	descriptions := PeakDescription(model)

	peakResults := make([]PeakResult, len(peaks))
	for i, p := range peaks {
		peakResults[i] = PeakResult{
			Label:       p.Label,
			Description: descriptions[p.Label],
			DistanceM:   p.DistanceM,
			AmplitudeDB: p.AmplitudeDB,
			TimeNs:      p.TimeNs,
		}
	}

	peakByLabel := make(map[string]Peak, len(peaks))
	for _, p := range peaks {
		peakByLabel[p.Label] = p
	}
	oa, ob, oc, od := peakByLabel["OA"], peakByLabel["OB"], peakByLabel["OC"], peakByLabel["OD"]

	coaxDelay := CoaxDelayNsPerM * coaxCableLengthM
	delays := DelayResult{
		SMAPortOffsetNs:          smaOffset,
		RxDelayNs:                ob.TimeNs - oa.TimeNs,
		CableDelayNs:             oc.TimeNs - ob.TimeNs,
		AntennaOpticalDelayNs:    od.TimeNs - oc.TimeNs,
		AntennaElectricalDelayNs: elecDelay,
		CoaxCableLengthM:         coaxCableLengthM,
		CoaxCableDelayNs:         coaxDelay,
	}
	delays.TotalDelayNs = delays.SMAPortOffsetNs + delays.RxDelayNs + delays.CableDelayNs +
		delays.AntennaOpticalDelayNs + delays.AntennaElectricalDelayNs
	delays.EndToEndDelayNs = delays.TotalDelayNs + delays.CoaxCableDelayNs

	ri := tor.RefractiveIndex
	trace := make([]TracePoint, len(tor.DataPoints))
	for i, dp := range tor.DataPoints {
		trace[i] = TracePoint{
			TimeNs:      math.Round(dp.TimeNs(ri)*1000) / 1000,
			AmplitudeDB: dp.AmplitudeDB,
		}
	}

	return &MeasurementResult{
		DeviceName:   name,
		SerialNumber: serial,
		Model:        model,
		AntennaGen:   antennaGen,
		TORFile:      tor.DateTime.Format("2006-01-02") + "_" + name + ".tor",
		DateTime:     tor.DateTime.Format(time.RFC3339),
		Settings: TORSettings{
			Wavelength:             tor.Wavelength,
			PulseWidth:             tor.PulseWidth,
			RefractiveIndex:        tor.RefractiveIndex,
			Resolution:             tor.Resolution,
			Start:                  tor.Start,
			End:                    tor.End,
			BackscatterCoefficient: tor.BackscatterCoefficient,
			Average:                tor.Average,
			LaunchCableLengthM:     launchCableLengthM,
		},
		Peaks:  peakResults,
		Delays: delays,
		Trace:  trace,
	}, nil
}
