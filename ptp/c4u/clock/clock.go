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
	"sort"

	ptp "github.com/facebook/time/ptp/protocol"
)

const (
	ClockClassLocked       ptp.ClockClass = ptp.ClockClass6
	ClockClassHoldover     ptp.ClockClass = ptp.ClockClass7
	ClockClassCalibrating  ptp.ClockClass = ptp.ClockClass13
	ClockClassUncalibrated ptp.ClockClass = ptp.ClockClass52
)

// RingBuffer is a ring buffer of ClockQuality data
type RingBuffer struct {
	data  []*ptp.ClockQuality
	index int
	size  int
}

// NewRingBuffer creates new RingBuffer of a defined size
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{size: size, data: make([]*ptp.ClockQuality, size)}
}

// Write new element to a ring buffer
func (rb *RingBuffer) Write(c *ptp.ClockQuality) {
	if rb.index >= rb.size {
		rb.index = 0
	}
	rb.data[rb.index] = c
	rb.index++
}

// Export data from the ring buffer
func (rb *RingBuffer) Data() []*ptp.ClockQuality {
	return rb.data
}

func Worst(clocks []*ptp.ClockQuality) *ptp.ClockQuality {
	var w ptp.ClockQuality
	var ca []ptp.ClockAccuracy
	var cc []ptp.ClockClass

	for _, c := range clocks {
		if c == nil {
			continue
		}

		ca = append(ca, c.ClockAccuracy)
		cc = append(cc, c.ClockClass)
	}

	if len(ca) == 0 || len(cc) == 0 {
		return nil
	}

	sort.Slice(ca, func(i, j int) bool { return ca[i] < ca[j] })
	sort.Slice(cc, func(i, j int) bool { return cc[i] < cc[j] })

	ca1 := len(ca) / 100 * 1
	cc1 := len(cc) / 100 * 1

	w.ClockAccuracy = ca[len(ca)-1-ca1]
	w.ClockClass = cc[len(cc)-1-cc1]

	return &w
}

func Run() (*ptp.ClockQuality, error) {
	oscillatord, err := oscillatord()
	if err != nil {
		return nil, err
	}

	ts2phc, err := ts2phc()
	if err != nil {
		return nil, err
	}

	return Worst([]*ptp.ClockQuality{oscillatord, ts2phc}), nil
}
