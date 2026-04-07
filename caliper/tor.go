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
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// SpeedOfLight in meters per second
const SpeedOfLight = 299792458.0

// TORFile represents a parsed Luciol LOR-220 OTDR .tor file
type TORFile struct {
	DateTime               time.Time
	InstrumentInfo         string
	ModuleSerialNumber     string
	CableID                string
	FiberID                string
	FiberType              int
	Wavelength             int
	PulseWidth             int
	RefractiveIndex        float64
	DistanceUnit           int
	Average                int
	End                    float64
	Start                  float64
	Resolution             float64
	DistanceRange          int
	LossThreshold          float64
	ReflectanceThreshold   float64
	EndOfFiberThreshold    float64
	BackscatterCoefficient float64
	HighResolution         int
	DataPoints             []DataPoint
}

// DataPoint is a single OTDR measurement
type DataPoint struct {
	DistanceM   float64
	AmplitudeDB float64
}

// TimeNs converts distance in meters to time in nanoseconds using the refractive index
func (d DataPoint) TimeNs(refractiveIndex float64) float64 {
	return d.DistanceM * refractiveIndex / SpeedOfLight * 1e9
}

// ParseTOR parses a Luciol OTDR .tor text file from a path
func ParseTOR(path string) (*TORFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening tor file: %w", err)
	}
	defer f.Close()
	return ParseTORReader(f)
}

// ParseTORReader parses a Luciol OTDR .tor text file from a reader
func ParseTORReader(r io.Reader) (*TORFile, error) {
	tor := &TORFile{}
	scanner := bufio.NewScanner(r)

	// Skip header lines until we find the first section
	for scanner.Scan() {
		line := cleanLine(scanner.Text())
		if line == "[DateTime]" {
			break
		}
	}

	currentSection := "DateTime"
	for scanner.Scan() {
		line := cleanLine(scanner.Text())

		if line == "[-]" {
			currentSection = ""
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = line[1 : len(line)-1]
			continue
		}

		if currentSection == "" && line == "" {
			continue
		}

		switch currentSection {
		case "DateTime":
			ts, err := strconv.ParseInt(line, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parsing DateTime %q: %w", line, err)
			}
			tor.DateTime = time.Unix(ts, 0)
		case "InstrumentInfo":
			if tor.InstrumentInfo == "" {
				tor.InstrumentInfo = line
			} else if sn, ok := strings.CutPrefix(line, "OTDR module S/N:"); ok {
				tor.ModuleSerialNumber = strings.TrimSpace(sn)
			}
		case "CableID":
			tor.CableID = line
		case "FiberID":
			tor.FiberID = line
		case "FiberType":
			v, err := strconv.Atoi(line)
			if err != nil {
				return nil, fmt.Errorf("parsing FiberType %q: %w", line, err)
			}
			tor.FiberType = v
		case "Wavelength":
			v, err := strconv.Atoi(line)
			if err != nil {
				return nil, fmt.Errorf("parsing Wavelength %q: %w", line, err)
			}
			tor.Wavelength = v
		case "PulseWidth":
			v, err := strconv.Atoi(line)
			if err != nil {
				return nil, fmt.Errorf("parsing PulseWidth %q: %w", line, err)
			}
			tor.PulseWidth = v
		case "RefractiveIndex":
			v, err := strconv.ParseFloat(line, 64)
			if err != nil {
				return nil, fmt.Errorf("parsing RefractiveIndex %q: %w", line, err)
			}
			tor.RefractiveIndex = v
		case "DistanceUnit":
			v, err := strconv.Atoi(line)
			if err != nil {
				return nil, fmt.Errorf("parsing DistanceUnit %q: %w", line, err)
			}
			tor.DistanceUnit = v
		case "Average":
			v, err := strconv.Atoi(line)
			if err != nil {
				return nil, fmt.Errorf("parsing Average %q: %w", line, err)
			}
			tor.Average = v
		case "End":
			v, err := strconv.ParseFloat(line, 64)
			if err != nil {
				return nil, fmt.Errorf("parsing End %q: %w", line, err)
			}
			tor.End = v
		case "Start":
			v, err := strconv.ParseFloat(line, 64)
			if err != nil {
				return nil, fmt.Errorf("parsing Start %q: %w", line, err)
			}
			tor.Start = v
		case "Resolution":
			v, err := strconv.ParseFloat(line, 64)
			if err != nil {
				return nil, fmt.Errorf("parsing Resolution %q: %w", line, err)
			}
			tor.Resolution = v
		case "DistanceRange":
			v, err := strconv.Atoi(line)
			if err != nil {
				return nil, fmt.Errorf("parsing DistanceRange %q: %w", line, err)
			}
			tor.DistanceRange = v
		case "LossThreshold":
			v, err := strconv.ParseFloat(line, 64)
			if err != nil {
				return nil, fmt.Errorf("parsing LossThreshold %q: %w", line, err)
			}
			tor.LossThreshold = v
		case "ReflectanceThreshold":
			v, err := strconv.ParseFloat(line, 64)
			if err != nil {
				return nil, fmt.Errorf("parsing ReflectanceThreshold %q: %w", line, err)
			}
			tor.ReflectanceThreshold = v
		case "EndOfFiberThreshold":
			v, err := strconv.ParseFloat(line, 64)
			if err != nil {
				return nil, fmt.Errorf("parsing EndOfFiberThreshold %q: %w", line, err)
			}
			tor.EndOfFiberThreshold = v
		case "BackscatterCoefficient":
			v, err := strconv.ParseFloat(line, 64)
			if err != nil {
				return nil, fmt.Errorf("parsing BackscatterCoefficient %q: %w", line, err)
			}
			tor.BackscatterCoefficient = v
		case "HighResolution":
			v, err := strconv.Atoi(line)
			if err != nil {
				return nil, fmt.Errorf("parsing HighResolution %q: %w", line, err)
			}
			tor.HighResolution = v
		case "DataPoints":
			dp, err := parseDataPoint(line)
			if err != nil {
				return nil, fmt.Errorf("parsing data point: %w", err)
			}
			tor.DataPoints = append(tor.DataPoints, dp)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading tor file: %w", err)
	}

	if len(tor.DataPoints) == 0 {
		return nil, fmt.Errorf("no data points found in tor file")
	}

	// Extract wavelength from InstrumentInfo if Wavelength field is an index
	if tor.Wavelength == 0 && tor.InstrumentInfo != "" {
		if wl, ok := parseWavelengthFromInfo(tor.InstrumentInfo); ok {
			tor.Wavelength = wl
		}
	}

	return tor, nil
}

func parseDataPoint(line string) (DataPoint, error) {
	parts := strings.Split(line, "\t")
	if len(parts) != 2 {
		return DataPoint{}, fmt.Errorf("invalid data point: %q", line)
	}
	dist, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return DataPoint{}, fmt.Errorf("parsing distance: %w", err)
	}
	amp, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return DataPoint{}, fmt.Errorf("parsing amplitude: %w", err)
	}
	return DataPoint{DistanceM: dist, AmplitudeDB: amp}, nil
}

// parseWavelengthFromInfo extracts the wavelength in nm from the InstrumentInfo string.
// Example: "1310  nm - 1  ns -" -> 1310
func parseWavelengthFromInfo(info string) (int, bool) {
	numStr, _, ok := strings.Cut(info, "nm")
	if !ok {
		return 0, false
	}
	numStr = strings.TrimSpace(numStr)
	wl, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, false
	}
	return wl, true
}

func cleanLine(line string) string {
	return strings.TrimRight(line, "\r\n ")
}
