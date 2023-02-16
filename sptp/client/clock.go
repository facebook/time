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

package client

import (
	"fmt"
	"time"

	"github.com/facebook/time/phc"
)

// PHCIface is the iface for phc device controls
type PHCIface interface {
	AdjFreqPPB(freq float64) error
	Step(step time.Duration) error
	FrequencyPPB() (float64, error)
	MaxFreqPPB() (float64, error)
}

// PHC groups methods for interactions with PHC devices
type PHC struct {
	devicePath string
}

// NewPHC creates new PHC device abstraction from network interface name
func NewPHC(iface string) (*PHC, error) {
	device, err := phc.IfaceToPHCDevice(iface)
	if err != nil {
		return nil, fmt.Errorf("failed to map iface to device: %w", err)
	}
	return &PHC{
		devicePath: device,
	}, nil
}

// AdjFreqPPB adjusts PHC frequency
func (p *PHC) AdjFreqPPB(freq float64) error {
	return phc.ClockAdjFreq(p.devicePath, freq)
}

// Step jumps time on PHC
func (p *PHC) Step(step time.Duration) error {
	return phc.ClockStep(p.devicePath, step)
}

// FrequencyPPB returns current PHC frequency
func (p *PHC) FrequencyPPB() (float64, error) {
	return phc.FrequencyPPBFromDevice(p.devicePath)
}

// MaxFreqPPB returns maximum frequency adjustment supported by PHC
func (p *PHC) MaxFreqPPB() (float64, error) {
	return phc.MaxFreqAdjPPBFromDevice(p.devicePath)
}
