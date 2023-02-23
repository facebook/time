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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReadConfigMissing(t *testing.T) {
	_, err := ReadConfig("/does/not/exist")
	require.Error(t, err)
}

func TestReadConfigDefaults(t *testing.T) {
	f, err := os.CreateTemp("", "sptp")
	require.NoError(t, err)
	defer os.Remove(f.Name()) // clean up
	cfg, err := ReadConfig(f.Name())
	require.NoError(t, err)
	want := &Config{
		MetricsAggregationWindow: 60 * time.Second,
		AttemptsTXTS:             10,
		TimeoutTXTS:              time.Duration(50) * time.Millisecond,
	}
	require.Equal(t, want, cfg)
}

func TestReadConfig(t *testing.T) {
	f, err := os.CreateTemp("", "sptp")
	require.NoError(t, err)
	defer os.Remove(f.Name()) // clean up
	_, err = f.Write([]byte(`iface: eth0
interval: 1s
timestamping: hardware
monitoringport: 4269
dscp: 35
firststepthreshold: 1s
metricsaggregationwindow: 10s
attemptstxts: 12
timeouttxts: 40ms
servers:
  192.168.0.10: 2
  192.168.0.13: 3
  192.168.0.15: 1
measurement:
  path_delay_filter_length: 59
  path_delay_filter: "median"
  path_delay_discard_filter_enabled: true
  path_delay_discard_below: 2us
`))
	require.NoError(t, err)
	cfg, err := ReadConfig(f.Name())
	require.NoError(t, err)
	want := &Config{
		Iface:                    "eth0",
		Timestamping:             "hardware",
		MonitoringPort:           4269,
		Interval:                 time.Second,
		DSCP:                     35,
		FirstStepThreshold:       time.Second,
		MetricsAggregationWindow: 10 * time.Second,
		Servers: map[string]int{
			"192.168.0.10": 2,
			"192.168.0.13": 3,
			"192.168.0.15": 1,
		},
		AttemptsTXTS: 12,
		TimeoutTXTS:  time.Duration(40) * time.Millisecond,
		Measurement: MeasurementConfig{
			PathDelayFilterLength:         59,
			PathDelayFilter:               "median",
			PathDelayDiscardFilterEnabled: true,
			PathDelayDiscardBelow:         2 * time.Microsecond,
		},
	}
	require.Equal(t, want, cfg)
}
