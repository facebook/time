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

package export

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEntryFromCSV(t *testing.T) {
	timestamp := "1599158325.368869"
	value := "-000.000006966500"
	csvLine := []string{timestamp, value}
	channel := "1"
	source := "calnex01"
	target := "ntp01"
	protocol := "ptp"

	expectedEntry := &Entry{
		Int:    &IntData{Time: int(1599158325)},
		Float:  &FloatData{Value: float64(-000.000006966500)},
		Normal: &NormalData{Channel: channel, Target: target, Protocol: protocol, Source: source},
	}
	entry, err := entryFromCSV(csvLine, channel, target, protocol, source)
	require.Nil(t, err)
	require.Equal(t, expectedEntry, entry)
}

func TestEntryFromCSVFieldCount(t *testing.T) {
	tests := []struct {
		name      string
		csvLine   []string
		wantShort bool
		wantErr   bool
	}{
		{name: "timestamp only", csvLine: []string{"1599158325.368869"}, wantShort: true, wantErr: true},
		{name: "no fields", csvLine: []string{}, wantShort: true, wantErr: true},
		// Tolerated: a firmware that appends a column must not stop the export.
		{name: "extra fields", csvLine: []string{"1599158325.368869", "-000.000006966500", "x"}},
		{name: "empty fields", csvLine: []string{"", ""}, wantErr: true},
		{name: "unparseable value", csvLine: []string{"1599158325.368869", "not-a-float"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := entryFromCSV(tt.csvLine, "1", "ntp01", "ptp", "calnex01")
			if !tt.wantErr {
				require.NoError(t, err)
				require.NotNil(t, entry)
				return
			}
			require.Error(t, err)
			require.Nil(t, entry)
			if tt.wantShort {
				require.ErrorContains(t, err, "at least 2 fields")
			} else {
				require.NotContains(t, err.Error(), "at least 2 fields")
			}
		})
	}
}
