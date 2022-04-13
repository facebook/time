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
	"fmt"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	log "github.com/sirupsen/logrus"
)

const (
	ClockClassLocked       ptp.ClockClass = ptp.ClockClass6
	ClockClassHoldover     ptp.ClockClass = ptp.ClockClass7
	ClockClassCalibrating  ptp.ClockClass = ptp.ClockClass13
	ClockClassUncalibrated ptp.ClockClass = ptp.ClockClass52
)

type DataPoint struct {
	PHCOffset            time.Duration
	OscillatorOffset     time.Duration
	OscillatorClockClass ptp.ClockClass
}

// RingBuffer is a ring buffer of ClockQuality data
type RingBuffer struct {
	data  []*DataPoint
	index int
	size  int
}

// NewRingBuffer creates new RingBuffer of a defined size
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{size: size, data: make([]*DataPoint, size)}
}

// Write new element to a ring buffer
func (rb *RingBuffer) Write(c *DataPoint) {
	if rb.index >= rb.size {
		rb.index = 0
	}
	rb.data[rb.index] = c
	rb.index++
}

// Export data from the ring buffer
func (rb *RingBuffer) Data() []*DataPoint {
	return rb.data
}

func Worst(points []*DataPoint, accuracyExpr string) (*ptp.ClockQuality, error) {
	expr, err := prepareExpression(accuracyExpr)
	if err != nil {
		return nil, fmt.Errorf("evaluating accuracy math: %w", err)
	}
	phcOffsets := []float64{}
	oscillatorOffsets := []float64{}
	var w *ptp.ClockQuality
	for _, c := range points {
		if c == nil {
			continue
		}
		if w == nil {
			w = &ptp.ClockQuality{}
		}
		phcOffsets = append(phcOffsets, float64(c.PHCOffset))
		// if oscillator is uncalibrated, ignore the offset as it's meaningless
		if c.OscillatorClockClass != ClockClassUncalibrated {
			oscillatorOffsets = append(oscillatorOffsets, float64(c.OscillatorOffset))
		}

		// TODO: consider better way to select clock class
		// Assuming higher class means worse
		if c.OscillatorClockClass > w.ClockClass {
			w.ClockClass = c.OscillatorClockClass
		}
	}
	if w == nil {
		return nil, nil
	}

	parameters := map[string]interface{}{
		"phcoffset":        phcOffsets,
		"oscillatoroffset": oscillatorOffsets,
	}
	vRaw, err := expr.Evaluate(parameters)
	if err != nil {
		return nil, err
	}
	v := time.Duration(vRaw.(float64))
	accFromOffset := ptp.ClockAccuracyFromOffset(v)
	log.Debugf("result of %q = %v", accuracyExpr, v)
	log.Debugf("clockAccuracy: %v\n", accFromOffset)
	w.ClockAccuracy = accFromOffset

	return w, nil
}

func Run() (*DataPoint, error) {
	oscillatord, err := oscillatord()
	if err != nil {
		return nil, err
	}

	phcOffset, err := ts2phc()
	if err != nil {
		return nil, err
	}

	d := &DataPoint{
		PHCOffset:            phcOffset,
		OscillatorOffset:     oscillatord.Offset,
		OscillatorClockClass: oscillatord.ClockClass,
	}

	return d, nil
}
