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
