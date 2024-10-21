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

	"github.com/stretchr/testify/require"
)

var txcapsTestcases = []struct {
	in  TxTypeCaps
	out string
}{
	{in: 0, out: "-"},
	{in: (1 << 0), out: "off (0)"},
	{in: (1 << 1), out: "on (1)"},
	{in: (1 << 2), out: "stepsync (2)"},
	{in: ((1 << 0) | (1 << 1)), out: "off (0), on (1)"},
	{in: ((1 << 0) | (1 << 2)), out: "off (0), stepsync (2)"},
	{in: ((1 << 1) | (1 << 2)), out: "on (1), stepsync (2)"},
	{in: ((1 << 0) | (1 << 1) | (1 << 2)), out: "off (0), on (1), stepsync (2)"},
}

func TestTxcapsString(t *testing.T) {
	for _, tc := range txcapsTestcases {
		out := tc.in.String()
		require.Equal(t, tc.out, out)
	}
}

var rxcapsTestcases = []struct {
	in  RxFilterCaps
	out string
}{
	{in: 0, out: "-"},
	{in: (1 << 0), out: "none (0)"},
	{in: (1 << 1), out: "all (1)"},
	{in: (1 << 3), out: "ptpv1-l4-event (3)"},
	{in: (1 << 6), out: "ptpv2-l4-event (6)"},
	{in: (1 << 9), out: "ptpv2-l2-event (9)"},
	{in: ((1 << 0) | (1 << 1)), out: "none (0), all (1)"},
	{
		in:  ((1 << 0) | (1 << 1) | (1 << 6) | (1 << 9)),
		out: "none (0), all (1), ptpv2-l4-event (6), ptpv2-l2-event (9)",
	},
}

func TestRxcapsString(t *testing.T) {
	for _, tc := range rxcapsTestcases {
		out := tc.in.String()
		require.Equal(t, tc.out, out)
	}
}
