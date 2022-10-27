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
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"time"
)

// LogSample has all the measurements we may want to log
type LogSample struct {
	MasterOffsetNS          float64
	MasterOffsetMeanNS      float64
	MasterOffsetStddevNS    float64
	PathDelayNS             float64
	PathDelayMeanNS         float64
	PathDelayStddevNS       float64
	FreqAdjustmentPPB       float64
	FreqAdjustmentMeanPPB   float64
	FreqAdjustmentStddevPPB float64
	MeasurementNS           float64
	MeasurementMeanNS       float64
	MeasurementStddevNS     float64
	WindowNS                float64
	ClockAccuracyMean       float64
}

var header = []string{
	"offset",
	"offset_mean",
	"offset_stddev",
	"delay",
	"delay_mean",
	"delay_stddev",
	"freq",
	"freq_mean",
	"freq_stddev",
	"measurement",
	"measurement_mean",
	"measurement_stddev",
	"window",
	"clock_accuracy_mean",
}

// CSVRecords returns all data from this sample as CSV. Must by synced with `header` variable.
func (s *LogSample) CSVRecords() []string {
	return []string{
		strconv.FormatFloat(s.MasterOffsetNS, 'f', -1, 64),
		strconv.FormatFloat(s.MasterOffsetMeanNS, 'f', -1, 64),
		strconv.FormatFloat(s.MasterOffsetStddevNS, 'f', -1, 64),
		strconv.FormatFloat(s.PathDelayNS, 'f', -1, 64),
		strconv.FormatFloat(s.PathDelayMeanNS, 'f', -1, 64),
		strconv.FormatFloat(s.PathDelayStddevNS, 'f', -1, 64),
		strconv.FormatFloat(s.FreqAdjustmentPPB, 'f', -1, 64),
		strconv.FormatFloat(s.FreqAdjustmentMeanPPB, 'f', -1, 64),
		strconv.FormatFloat(s.FreqAdjustmentStddevPPB, 'f', -1, 64),
		strconv.FormatFloat(s.MeasurementNS, 'f', -1, 64),
		strconv.FormatFloat(s.MeasurementMeanNS, 'f', -1, 64),
		strconv.FormatFloat(s.MeasurementStddevNS, 'f', -1, 64),
		strconv.FormatFloat(s.WindowNS, 'f', -1, 64),
		strconv.FormatFloat(s.ClockAccuracyMean, 'f', -1, 64),
	}
}

// Logger is something that can store LogSample somewhere
type Logger interface {
	Log(*LogSample) error
}

// CSVLogger logs Sample as CSV into given writer
type CSVLogger struct {
	csvwriter     *csv.Writer
	printedHeader bool
}

// NewCSVLogger returns new CSVLogger
func NewCSVLogger(w io.Writer) *CSVLogger {
	return &CSVLogger{
		csvwriter: csv.NewWriter(w),
	}
}

// Log implements Logger interface
func (l *CSVLogger) Log(s *LogSample) error {
	if !l.printedHeader {
		if err := l.csvwriter.Write(header); err != nil {
			return err
		}
		l.printedHeader = true
	}
	csv := s.CSVRecords()
	if err := l.csvwriter.Write(csv); err != nil {
		return err
	}
	l.csvwriter.Flush()
	return nil
}

// DummyLogger logs M and W to given writer
type DummyLogger struct {
	w io.Writer
}

// NewDummyLogger returns new DummyLogger
func NewDummyLogger(w io.Writer) *DummyLogger {
	return &DummyLogger{w: w}
}

// Log implements Logger interface
func (l *DummyLogger) Log(s *LogSample) error {
	_, err := fmt.Fprintf(l.w, "m = %v, w = %v\n", time.Duration(s.MeasurementNS), time.Duration(s.WindowNS))
	return err
}
