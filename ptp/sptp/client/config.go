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
	"net"
	"os"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/timestamp"
	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

// BackoffConfig describes configuration for backoff in case of unavailable GM
type BackoffConfig struct {
	Mode     string
	Step     int
	MaxValue int
}

// Validate BackoffConfig is sane
func (c *BackoffConfig) Validate() error {
	if c.Mode != backoffNone && c.Mode != backoffFixed && c.Mode != backoffLinear && c.Mode != backoffExponential {
		return fmt.Errorf("mode must be either %q, %q, %q or %q", backoffNone, backoffFixed, backoffLinear, backoffExponential)
	}
	if c.Mode != backoffNone {
		if c.Step <= 0 {
			return fmt.Errorf("step must be positive")
		}
		if c.Mode != backoffFixed && c.MaxValue <= 0 {
			return fmt.Errorf("maxvalue must be positive")
		}
	}
	return nil
}

// MeasurementConfig describes configuration for how we measure offset
type MeasurementConfig struct {
	PathDelayFilterLength         int           `yaml:"path_delay_filter_length"`          // over how many last path delays we filter
	PathDelayFilter               string        `yaml:"path_delay_filter"`                 // which filter to use, see supported path delay filters const
	PathDelayDiscardFilterEnabled bool          `yaml:"path_delay_discard_filter_enabled"` // controls filter that allows us to discard anomalously small path delays
	PathDelayDiscardBelow         time.Duration `yaml:"path_delay_discard_below"`          // discard path delays that are below this threshold
	PathDelayDiscardAbove         time.Duration `yaml:"path_delay_discard_above"`          // discard path delays that are above this threshold
}

// Validate MeasurementConfig is sane
func (c *MeasurementConfig) Validate() error {
	if c.PathDelayFilterLength < 0 {
		return fmt.Errorf("path_delay_filter_length must be 0 or positive")
	}
	if c.PathDelayFilter != FilterNone && c.PathDelayFilter != FilterMean && c.PathDelayFilter != FilterMedian {
		return fmt.Errorf("path_delay_filter must be either %q, %q or %q", FilterNone, FilterMean, FilterMedian)
	}
	if c.PathDelayDiscardFilterEnabled && (c.PathDelayDiscardAbove < c.PathDelayDiscardBelow) {
		return fmt.Errorf("path_delay_discard_below must be less than path_delay_discard_above")
	}
	return nil
}

// Config specifies SPTP run options
type Config struct {
	Iface                    string
	Timestamping             timestamp.Timestamp
	MonitoringPort           int
	Interval                 time.Duration
	ExchangeTimeout          time.Duration
	DSCP                     int
	FirstStepThreshold       time.Duration
	Servers                  map[string]int
	MaxClockClass            ptp.ClockClass
	MaxClockAccuracy         ptp.ClockAccuracy
	Measurement              MeasurementConfig
	MetricsAggregationWindow time.Duration
	AttemptsTXTS             int
	TimeoutTXTS              time.Duration
	FreeRunning              bool
	Backoff                  BackoffConfig
	SequenceIDMaskBits       uint
	SequenceIDMaskValue      uint
}

// DefaultConfig returns Config initialized with default values
func DefaultConfig() *Config {
	return &Config{
		Interval:                 time.Second,
		ExchangeTimeout:          100 * time.Millisecond,
		MaxClockClass:            ptp.ClockClass7,
		MaxClockAccuracy:         ptp.ClockAccuracyMicrosecond10,
		MetricsAggregationWindow: time.Duration(60) * time.Second,
		AttemptsTXTS:             10,
		TimeoutTXTS:              time.Duration(50) * time.Millisecond,
		Timestamping:             timestamp.HW,
		Measurement: MeasurementConfig{
			PathDelayDiscardAbove: time.Second,
		},
	}
}

// Validate config is sane
func (c *Config) Validate() error {
	if c.Interval <= 0 {
		return fmt.Errorf("interval must be greater than zero")
	}
	if c.AttemptsTXTS <= 0 {
		return fmt.Errorf("attemptstxts must be greater than zero")
	}
	if c.TimeoutTXTS <= 0 {
		return fmt.Errorf("timeouttxts must be greater than zero")
	}
	if c.MaxClockClass < ptp.ClockClass6 || c.MaxClockClass > ptp.ClockClass58 {
		return fmt.Errorf("invalid range of allowed clock class")
	}
	if c.MaxClockAccuracy < ptp.ClockAccuracyNanosecond25 || c.MaxClockAccuracy > ptp.ClockAccuracySecondGreater10 {
		return fmt.Errorf("invalid range of allowed clock accuracy")
	}
	if c.MetricsAggregationWindow <= 0 {
		return fmt.Errorf("metricsaggregationwindow must be greater than zero")
	}
	if c.MonitoringPort < 0 {
		return fmt.Errorf("monitoringport must be 0 or positive")
	}
	if c.DSCP < 0 {
		return fmt.Errorf("dscp must be 0 or positive")
	}
	if c.ExchangeTimeout <= 0 || c.ExchangeTimeout >= c.Interval {
		return fmt.Errorf("exchangetimeout must be greater than zero but less than interval")
	}
	if len(c.Servers) == 0 {
		return fmt.Errorf("at least one server must be specified")
	}
	if c.Timestamping != timestamp.HW && c.Timestamping != timestamp.SW {
		return fmt.Errorf("only %q and %q timestamping is supported", timestamp.HW, timestamp.SW)
	}
	if c.Iface == "" {
		return fmt.Errorf("iface must be specified")
	}
	if err := c.Measurement.Validate(); err != nil {
		return fmt.Errorf("invalid measurement config: %w", err)
	}
	if err := c.Backoff.Validate(); err != nil {
		return fmt.Errorf("invalid backoff config: %w", err)
	}
	if c.SequenceIDMaskBits > 15 {
		return fmt.Errorf("invalid value for SequenceIDMaskBits: %d (must be 0 <= value < 16)", c.SequenceIDMaskBits)
	}
	if c.SequenceIDMaskValue & ^((1<<c.SequenceIDMaskBits)-1) > 0 {
		return fmt.Errorf("invalid value for SequenceIDMaskValue: %d is more than mask %d can handle", c.SequenceIDMaskValue, c.SequenceIDMaskBits)
	}
	return nil
}

// GenerateMaskAndValue returns the mask that must be applied to sequence id and the constant value to use
func (c *Config) GenerateMaskAndValue() (uint16, uint16) {
	sequenceIDMask := (uint16)(^(((1 << c.SequenceIDMaskBits) - 1) << (16 - c.SequenceIDMaskBits)))
	sequenceIDMaskedValue := (uint16)(c.SequenceIDMaskValue << (16 - c.SequenceIDMaskBits))
	return sequenceIDMask, sequenceIDMaskedValue
}

// ReadConfig reads config from the file
func ReadConfig(path string) (*Config, error) {
	c := DefaultConfig()
	cData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(cData, &c)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func addrToIPstr(address string) string {
	if net.ParseIP(address) == nil {
		names, err := net.LookupHost(address)
		if err == nil && len(names) > 0 {
			address = names[0]
		}
	}
	return address
}

// PrepareConfig prepares final version of config based on defaults, CLI flags and on-disk config, and validates resulting config
func PrepareConfig(cfgPath string, targets []string, iface string, monitoringPort int, interval time.Duration, dscp int) (*Config, error) {
	cfg := DefaultConfig()
	var err error
	warn := func(name string) {
		log.Warningf("overriding %s from CLI flag", name)
	}
	if cfgPath != "" {
		cfg, err = ReadConfig(cfgPath)
		if err != nil {
			return nil, fmt.Errorf("reading config from %q: %w", cfgPath, err)
		}
	}
	if len(targets) > 0 {
		warn("targets")
		cfg.Servers = map[string]int{}
		for i, t := range targets {
			address := addrToIPstr(t)
			cfg.Servers[address] = i
		}
	} else {
		newServers := map[string]int{}
		for t, i := range cfg.Servers {
			address := addrToIPstr(t)
			newServers[address] = i
		}
		cfg.Servers = newServers
	}
	if iface != "" && iface != cfg.Iface {
		warn("iface")
		cfg.Iface = iface
	}

	if monitoringPort != 0 && monitoringPort != cfg.MonitoringPort {
		warn("monitoringPort")
		cfg.MonitoringPort = monitoringPort
	}

	if interval != 0 && interval != cfg.Interval {
		warn("interval")
		cfg.Interval = interval
	}

	if dscp != 0 && dscp != cfg.DSCP {
		warn("dscp")
		cfg.DSCP = dscp
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}
	log.Debugf("config: %+v", cfg)
	return cfg, nil
}
