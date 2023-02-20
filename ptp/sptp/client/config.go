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
	"os"
	"time"

	yaml "gopkg.in/yaml.v2"
)

// MeasurementConfig describes configuration for how we measure offset
type MeasurementConfig struct {
	PathDelayFilterLength         int           `yaml:"path_delay_filter_length"`          // over how many last path delays we filter
	PathDelayFilter               string        `yaml:"path_delay_filter"`                 // which filter to use, see supported path delay filters const
	PathDelayDiscardFilterEnabled bool          `yaml:"path_delay_discard_filter_enabled"` // controls filter that allows us to discard anomalously small path delays
	PathDelayDiscardBelow         time.Duration `yaml:"path_delay_discard_below"`          // discard path delays that are below this threshold
}

// Config specifies PTPNG run options
type Config struct {
	Iface                    string
	Timestamping             string
	MonitoringPort           int
	Interval                 time.Duration
	DSCP                     int
	FirstStepThreshold       time.Duration
	Servers                  map[string]int
	Measurement              MeasurementConfig
	MetricsAggregationWindow time.Duration
}

// ReadConfig reads config from the file
func ReadConfig(path string) (*Config, error) {
	c := &Config{MetricsAggregationWindow: time.Duration(60) * time.Second}
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
