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
		Interval:                 time.Second,
		ExchangeTimeout:          100 * time.Millisecond,
		MetricsAggregationWindow: time.Duration(60) * time.Second,
		AttemptsTXTS:             10,
		TimeoutTXTS:              time.Duration(50) * time.Millisecond,
		Timestamping:             HWTIMESTAMP,
	}
	require.Equal(t, want, cfg)
}

func TestReadConfig(t *testing.T) {
	f, err := os.CreateTemp("", "sptp")
	require.NoError(t, err)
	defer os.Remove(f.Name()) // clean up
	_, err = f.Write([]byte(`iface: eth0
interval: 1s
exchangetimeout: 200ms
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
		ExchangeTimeout:          200 * time.Millisecond,
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

func TestBackoffConfigValidate(t *testing.T) {
	testCases := []struct {
		name    string
		in      BackoffConfig
		wantErr bool
	}{
		{
			name:    "empty",
			in:      BackoffConfig{},
			wantErr: false,
		},
		{
			name: "zero step and maxvalue",
			in: BackoffConfig{
				Mode: backoffLinear,
			},
			wantErr: true,
		},
		{
			name: "negative step",
			in: BackoffConfig{
				Mode:     backoffFixed,
				Step:     -10,
				MaxValue: 50,
			},
			wantErr: true,
		},
		{
			name: "zero maxvalue",
			in: BackoffConfig{
				Mode: backoffLinear,
				Step: 10,
			},
			wantErr: true,
		},
		{
			name: "zero maxvalue, fixed mode",
			in: BackoffConfig{
				Mode: backoffFixed,
				Step: 10,
			},
			wantErr: false,
		},
		{
			name: "negative step",
			in: BackoffConfig{
				Mode:     backoffLinear,
				Step:     10,
				MaxValue: -10,
			},
			wantErr: true,
		},
		{
			name: "unsupported mode",
			in: BackoffConfig{
				Mode: "blah",
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.in.Validate()
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMeasurementConfigValidate(t *testing.T) {
	testCases := []struct {
		name    string
		in      MeasurementConfig
		wantErr bool
	}{
		{
			name:    "empty",
			in:      MeasurementConfig{},
			wantErr: false,
		},
		{
			name: "negative path_delay_filter_length",
			in: MeasurementConfig{
				PathDelayFilterLength: -1,
			},
			wantErr: true,
		},
		{
			name: "unsupported filter",
			in: MeasurementConfig{
				PathDelayFilter: "blah",
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.in.Validate()
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfigValidate(t *testing.T) {
	testCases := []struct {
		name    string
		in      Config
		wantErr bool
	}{
		{
			name:    "empty",
			in:      Config{},
			wantErr: true,
		},
		{
			name:    "default, no servers",
			in:      *DefaultConfig(),
			wantErr: true,
		},
		{
			name: "default, one server, no iface",
			in: Config{
				Interval:                 time.Second,
				ExchangeTimeout:          100 * time.Millisecond,
				MetricsAggregationWindow: time.Duration(60) * time.Second,
				AttemptsTXTS:             10,
				TimeoutTXTS:              time.Duration(50) * time.Millisecond,
				Timestamping:             HWTIMESTAMP,
				Servers: map[string]int{
					"192.168.0.10": 0,
				},
			},
			wantErr: true,
		},
		{
			name: "default, one server, iface",
			in: Config{
				Iface:                    "eth0",
				Interval:                 time.Second,
				ExchangeTimeout:          100 * time.Millisecond,
				MetricsAggregationWindow: time.Duration(60) * time.Second,
				AttemptsTXTS:             10,
				TimeoutTXTS:              time.Duration(50) * time.Millisecond,
				Timestamping:             HWTIMESTAMP,
				Servers: map[string]int{
					"192.168.0.10": 0,
				},
			},
			wantErr: false,
		},
		{
			name: "negative interval",
			in: Config{
				Iface:                    "eth0",
				Interval:                 -1 * time.Second,
				ExchangeTimeout:          100 * time.Millisecond,
				MetricsAggregationWindow: time.Duration(60) * time.Second,
				AttemptsTXTS:             10,
				TimeoutTXTS:              time.Duration(50) * time.Millisecond,
				Timestamping:             HWTIMESTAMP,
				Servers: map[string]int{
					"192.168.0.10": 0,
				},
			},
			wantErr: true,
		},
		{
			name: "negative attemptstxts",
			in: Config{
				Iface:                    "eth0",
				Interval:                 time.Second,
				ExchangeTimeout:          100 * time.Millisecond,
				MetricsAggregationWindow: time.Duration(60) * time.Second,
				AttemptsTXTS:             -10,
				TimeoutTXTS:              time.Duration(50) * time.Millisecond,
				Timestamping:             HWTIMESTAMP,
				Servers: map[string]int{
					"192.168.0.10": 0,
				},
			},
			wantErr: true,
		},
		{
			name: "negative timeouttxts",
			in: Config{
				Iface:                    "eth0",
				Interval:                 time.Second,
				ExchangeTimeout:          100 * time.Millisecond,
				MetricsAggregationWindow: time.Duration(60) * time.Second,
				AttemptsTXTS:             10,
				TimeoutTXTS:              time.Duration(-50) * time.Millisecond,
				Timestamping:             HWTIMESTAMP,
				Servers: map[string]int{
					"192.168.0.10": 0,
				},
			},
			wantErr: true,
		},
		{
			name: "negative metricsaggregationwindow",
			in: Config{
				Iface:                    "eth0",
				Interval:                 time.Second,
				ExchangeTimeout:          100 * time.Millisecond,
				MetricsAggregationWindow: time.Duration(-60) * time.Second,
				AttemptsTXTS:             10,
				TimeoutTXTS:              time.Duration(50) * time.Millisecond,
				Timestamping:             HWTIMESTAMP,
				Servers: map[string]int{
					"192.168.0.10": 0,
				},
			},
			wantErr: true,
		},
		{
			name: "negative monitoringport",
			in: Config{
				Iface:                    "eth0",
				Interval:                 time.Second,
				ExchangeTimeout:          100 * time.Millisecond,
				MetricsAggregationWindow: time.Duration(60) * time.Second,
				AttemptsTXTS:             10,
				TimeoutTXTS:              time.Duration(50) * time.Millisecond,
				Timestamping:             HWTIMESTAMP,
				Servers: map[string]int{
					"192.168.0.10": 0,
				},
				MonitoringPort: -10,
			},
			wantErr: true,
		},
		{
			name: "negative DSCP",
			in: Config{
				Iface:                    "eth0",
				Interval:                 time.Second,
				ExchangeTimeout:          100 * time.Millisecond,
				MetricsAggregationWindow: time.Duration(60) * time.Second,
				AttemptsTXTS:             10,
				TimeoutTXTS:              time.Duration(50) * time.Millisecond,
				Timestamping:             HWTIMESTAMP,
				Servers: map[string]int{
					"192.168.0.10": 0,
				},
				DSCP: -1,
			},
			wantErr: true,
		},
		{
			name: "bad timestamping",
			in: Config{
				Iface:                    "eth0",
				Interval:                 time.Second,
				ExchangeTimeout:          100 * time.Millisecond,
				MetricsAggregationWindow: time.Duration(60) * time.Second,
				AttemptsTXTS:             10,
				TimeoutTXTS:              time.Duration(50) * time.Millisecond,
				Timestamping:             "blah",
				Servers: map[string]int{
					"192.168.0.10": 0,
				},
			},
			wantErr: true,
		},
		{
			name: "negative exchangetimeout",
			in: Config{
				Iface:                    "eth0",
				Interval:                 time.Second,
				ExchangeTimeout:          -100 * time.Millisecond,
				MetricsAggregationWindow: time.Duration(60) * time.Second,
				AttemptsTXTS:             10,
				TimeoutTXTS:              time.Duration(50) * time.Millisecond,
				Timestamping:             HWTIMESTAMP,
				Servers: map[string]int{
					"192.168.0.10": 0,
				},
			},
			wantErr: true,
		},
		{
			name: "wrong exchangetimeout",
			in: Config{
				Iface:                    "eth0",
				Interval:                 time.Second,
				ExchangeTimeout:          2 * time.Second,
				MetricsAggregationWindow: time.Duration(60) * time.Second,
				AttemptsTXTS:             10,
				TimeoutTXTS:              time.Duration(50) * time.Millisecond,
				Timestamping:             HWTIMESTAMP,
				Servers: map[string]int{
					"192.168.0.10": 0,
				},
			},
			wantErr: true,
		},
		{
			name: "bad measurement config",
			in: Config{
				Iface:                    "eth0",
				Interval:                 time.Second,
				ExchangeTimeout:          100 * time.Millisecond,
				MetricsAggregationWindow: time.Duration(60) * time.Second,
				AttemptsTXTS:             10,
				TimeoutTXTS:              time.Duration(50) * time.Millisecond,
				Timestamping:             HWTIMESTAMP,
				Servers: map[string]int{
					"192.168.0.10": 0,
				},
				Measurement: MeasurementConfig{
					PathDelayFilterLength: -1,
				},
			},
			wantErr: true,
		},
		{
			name: "bad backoff config",
			in: Config{
				Iface:                    "eth0",
				Interval:                 time.Second,
				ExchangeTimeout:          100 * time.Millisecond,
				MetricsAggregationWindow: time.Duration(60) * time.Second,
				AttemptsTXTS:             10,
				TimeoutTXTS:              time.Duration(50) * time.Millisecond,
				Timestamping:             HWTIMESTAMP,
				Servers: map[string]int{
					"192.168.0.10": 0,
				},
				Backoff: BackoffConfig{
					Mode: "fggl",
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.in.Validate()
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPrepareConfig(t *testing.T) {
	f, err := os.CreateTemp("", "sptp")
	require.NoError(t, err)
	defer os.Remove(f.Name()) // clean up
	_, err = f.Write([]byte(`iface: eth0
interval: 1s
exchangetimeout: 200ms
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
	cfg, err := PrepareConfig(f.Name(), nil, "eth1", 3456, 2*time.Second, 42)
	require.NoError(t, err)
	want := &Config{
		Iface:                    "eth1",
		Timestamping:             "hardware",
		MonitoringPort:           3456,
		Interval:                 2 * time.Second,
		ExchangeTimeout:          200 * time.Millisecond,
		DSCP:                     42,
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

func TestPrepareConfigDefaults(t *testing.T) {
	cfg, err := PrepareConfig("", []string{"192.168.0.10"}, "eth1", 3456, 2*time.Second, 42)
	require.NoError(t, err)
	want := &Config{
		Iface:                    "eth1",
		Timestamping:             "hardware",
		MonitoringPort:           3456,
		Interval:                 2 * time.Second,
		ExchangeTimeout:          100 * time.Millisecond,
		DSCP:                     42,
		FirstStepThreshold:       0,
		MetricsAggregationWindow: 60 * time.Second,
		Servers: map[string]int{
			"192.168.0.10": 0,
		},
		AttemptsTXTS: 10,
		TimeoutTXTS:  time.Duration(50) * time.Millisecond,
		Measurement: MeasurementConfig{
			PathDelayFilterLength:         0,
			PathDelayFilter:               "",
			PathDelayDiscardFilterEnabled: false,
			PathDelayDiscardBelow:         0,
		},
	}
	require.Equal(t, want, cfg)
}
