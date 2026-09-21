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
	"os"
	"time"

	"github.com/facebook/time/clock"
	"github.com/facebook/time/phc"
	"github.com/facebook/time/phc/unix"
	log "github.com/sirupsen/logrus"
)

// Clock is the iface for clock device controls
type Clock interface {
	AdjFreqPPB(freq float64) error
	Step(step time.Duration) error
	Time() (time.Time, error)
	SetTime(t time.Time) error
	FrequencyPPB() (float64, error)
	MaxFreqPPB() (float64, error)
	SetSync() error
}

// PHC groups methods for interactions with PHC devices
type PHC struct {
	dev *phc.Device
}

// NewPHC creates new PHC device abstraction from network interface name
func NewPHC(iface string) (*PHC, error) {
	devicePath, err := phc.IfaceToPHCDevice(iface)
	if err != nil {
		return nil, fmt.Errorf("failed to map iface to device: %w", err)
	}

	// Keep file open for the lifetime of the sptp
	f, err := os.OpenFile(devicePath, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("opening device %s error: %w", devicePath, err)
	}

	return &PHC{dev: phc.FromFile(f)}, nil
}

// AdjFreqPPB adjusts PHC frequency
func (p *PHC) AdjFreqPPB(freqPPB float64) error {
	return p.dev.AdjFreq(freqPPB)
}

// Step jumps time on PHC
func (p *PHC) Step(step time.Duration) error {
	return p.dev.Step(step)
}

// Time returns current PHC time
func (p *PHC) Time() (time.Time, error) {
	return p.dev.Time()
}

// SetTime sets PHC to an absolute time
func (p *PHC) SetTime(t time.Time) error {
	return p.dev.SetTime(t)
}

// FrequencyPPB returns current PHC frequency
func (p *PHC) FrequencyPPB() (float64, error) {
	return p.dev.FreqPPB()
}

// MaxFreqPPB returns maximum frequency adjustment supported by PHC
func (p *PHC) MaxFreqPPB() (float64, error) {
	return p.dev.MaxFreqAdjPPB()
}

// SetSync is a no-op for PHC
func (p *PHC) SetSync() error {
	return nil
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
	return clock.SetSync(unix.CLOCK_REALTIME)
}

// Step jumps time on PHC
func (c *SysClock) Step(step time.Duration) error {
	state, err := clock.Step(unix.CLOCK_REALTIME, step)
	if err == nil && state != unix.TIME_OK {
		log.Warningf("clock state %d is not TIME_OK after stepping", state)
	}
	return err
}

// Time returns current system time
func (c *SysClock) Time() (time.Time, error) {
	// Round(0) strips the monotonic reading, which Sub would otherwise use in
	// preference to the wall clock that SetTime moves
	return time.Now().Round(0), nil
}

// SetTime sets the system clock to an absolute time
func (c *SysClock) SetTime(t time.Time) error {
	ts, err := unix.TimeToTimespec(t)
	if err != nil {
		return err
	}
	return unix.ClockSettime(unix.CLOCK_REALTIME, &ts)
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
type FreeRunningClock struct {
	offset time.Duration
}

// AdjFreqPPB adjusts PHC frequency
func (c *FreeRunningClock) AdjFreqPPB(_ float64) error {
	return nil
}

// Step jumps time on PHC
func (c *FreeRunningClock) Step(_ time.Duration) error {
	return nil
}

// Time returns current system time shifted by whatever SetTime was last asked for
func (c *FreeRunningClock) Time() (time.Time, error) {
	return time.Now().Round(0).Add(c.offset), nil
}

// SetTime records the requested time without touching any real clock
func (c *FreeRunningClock) SetTime(t time.Time) error {
	c.offset = time.Until(t)
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

// SetSync sets clock status to TIME_OK
func (c *FreeRunningClock) SetSync() error {
	return nil
}
