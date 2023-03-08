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

	"github.com/facebook/time/clock"
	"github.com/facebook/time/phc"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// Clock is the iface for clock device controls
type Clock interface {
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

// SysClock groups methods for interacting with system clock
type SysClock struct{}

// AdjFreqPPB adjusts PHC frequency
func (c *SysClock) AdjFreqPPB(freqPPB float64) error {
	state, err := clock.AdjFreqPPB(unix.CLOCK_REALTIME, freqPPB)
	if err == nil && state != unix.TIME_OK {
		log.Warningf("clock state %d is not TIME_OK after adjusting frequency", state)
	}
	return err
}

// SetSync sets clock status to TIME_OK
func (c *SysClock) SetSync() error {
	tx := &unix.Timex{}
	// man(2) clock_adjtime, turn ppb to ppm
	tx.Modes = clock.AdjStatus | clock.AdjMaxError
	state, err := clock.Adjtime(unix.CLOCK_REALTIME, tx)

	if err == nil && state != unix.TIME_OK {
		return fmt.Errorf("clock state %d is not TIME_OK after setting sync state", state)
	}
	return err
}

// Step jumps time on PHC
func (c *SysClock) Step(step time.Duration) error {
	state, err := clock.Step(unix.CLOCK_REALTIME, step)
	if err == nil && state != unix.TIME_OK {
		log.Warningf("clock state %d is not TIME_OK after stepping", state)
	}
	return err
}

// FrequencyPPB returns current PHC frequency
func (c *SysClock) FrequencyPPB() (float64, error) {
	freqPPB, state, err := clock.FrequencyPPB(unix.CLOCK_REALTIME)
	if err == nil && state != unix.TIME_OK {
		log.Warningf("clock state %d is not TIME_OK after getting current frequency", state)
	}
	return freqPPB, err
}

// MaxFreqPPB returns maximum frequency adjustment supported by PHC
func (c *SysClock) MaxFreqPPB() (float64, error) {
	freqPPB, state, err := clock.MaxFreqPPB(unix.CLOCK_REALTIME)
	if err == nil && state != unix.TIME_OK {
		log.Warningf("clock state %d is not TIME_OK after getting max frequency adjustment", state)
	}
	return freqPPB, err
}

// FreeRunningClock is a dummy clock that does nothing
type FreeRunningClock struct{}

// AdjFreqPPB adjusts PHC frequency
func (c *FreeRunningClock) AdjFreqPPB(freqPPB float64) error {
	return nil
}

// Step jumps time on PHC
func (c *FreeRunningClock) Step(step time.Duration) error {
	return nil
}

// FrequencyPPB returns current PHC frequency
func (c *FreeRunningClock) FrequencyPPB() (float64, error) {
	return 0.0, nil
}

// MaxFreqPPB returns maximum frequency adjustment supported by PHC
func (c *FreeRunningClock) MaxFreqPPB() (float64, error) {
	return 0.0, nil
}
