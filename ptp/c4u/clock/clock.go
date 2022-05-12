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
	ClockClassLock         ptp.ClockClass = ptp.ClockClass6
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

func Worst(points []*DataPoint, accuracyExpr, classExpr string) (*ptp.ClockQuality, error) {
	aexpr, err := prepareExpression(accuracyExpr)
	if err != nil {
		return nil, fmt.Errorf("evaluating accuracy math: %w", err)
	}

	cexpr, err := prepareExpression(classExpr)
	if err != nil {
		return nil, fmt.Errorf("evaluating class math: %w", err)
	}

	phcOffsets := []float64{}
	oscillatorOffsets := []float64{}
	oscillatorClasses := []float64{}

	var w *ptp.ClockQuality
	for _, c := range points {
		if c == nil {
			continue
		}
		if w == nil {
			w = &ptp.ClockQuality{}
		}
		phcOffsets = append(phcOffsets, float64(c.PHCOffset))

		oscillatorOffsets = append(oscillatorOffsets, float64(c.OscillatorOffset))
		oscillatorClasses = append(oscillatorClasses, float64(c.OscillatorClockClass))
	}
	if w == nil {
		return nil, nil
	}

	offsets := map[string]interface{}{
		"phcoffset":        phcOffsets,
		"oscillatoroffset": oscillatorOffsets,
	}
	oRaw, err := aexpr.Evaluate(offsets)
	if err != nil {
		return nil, err
	}
	o := time.Duration(oRaw.(float64))
	accFromOffset := ptp.ClockAccuracyFromOffset(o)
	log.Debugf("result of %q = %v", accuracyExpr, o)
	log.Debugf("clockAccuracy: %v\n", accFromOffset)

	classes := map[string]interface{}{
		"oscillatorclass": oscillatorClasses,
	}
	cRaw, err := cexpr.Evaluate(classes)
	if err != nil {
		return nil, err
	}
	c := ptp.ClockClass(cRaw.(float64))
	log.Debugf("result of %q = %v", classExpr, c)

	w.ClockClass = c
	w.ClockAccuracy = accFromOffset

	// Some override logic to represent the situation better:
	// * In holdover we don't know the offset. Let's fallback to 1us or worse
	// * In uncalibrated state we don't know the offset - let's say it
	if w.ClockClass == ClockClassHoldover && w.ClockAccuracy < ptp.ClockAccuracyMicrosecond1 {
		w.ClockAccuracy = ptp.ClockAccuracyMicrosecond1
	} else if w.ClockClass == ClockClassUncalibrated {
		w.ClockAccuracy = ptp.ClockAccuracyUnknown
	}

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
