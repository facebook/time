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

package cmd

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestRefIDIPv4(t *testing.T) {
	// refID prints to stdout; just check it doesn't error on valid IPv4
	err := refID("192.168.1.1")
	require.NoError(t, err)
}

func TestRefIDIPv6(t *testing.T) {
	err := refID("2001:db8::1")
	require.NoError(t, err)
}

func TestRefIDEmpty(t *testing.T) {
	err := refID("")
	require.Error(t, err)
}

func TestRefIDInvalid(t *testing.T) {
	err := refID("not-an-ip")
	require.Error(t, err)
}

func TestStripZeroes(t *testing.T) {
	testCases := []struct {
		input float64
		want  string
	}{
		{input: 1.50, want: "1.5"},
		{input: 2.00, want: "2"},
		{input: 3.14, want: "3.14"},
		{input: 0.10, want: "0.1"},
		{input: 0.00, want: "0"},
		{input: 100.00, want: "100"},
		{input: 42.01, want: "42.01"},
		{input: -1.50, want: "-1.5"},
		{input: -0.10, want: "-0.1"},
	}
	for _, tc := range testCases {
		t.Run(tc.want, func(t *testing.T) {
			got := stripZeroes(tc.input)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestConfigureVerbosity(t *testing.T) {
	verbose = false
	ConfigureVerbosity()
	require.Equal(t, log.InfoLevel, log.GetLevel())

	verbose = true
	ConfigureVerbosity()
	require.Equal(t, log.DebugLevel, log.GetLevel())
}
