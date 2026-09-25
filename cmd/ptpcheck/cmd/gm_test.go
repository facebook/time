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
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/facebook/time/cmd/ptpcheck/render"
	"github.com/facebook/time/ptp/sptp/asymmetry"
	"github.com/facebook/time/ptp/sptp/stats"
	"github.com/stretchr/testify/require"
)

func TestPrintGMStatsTable(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, printGMStats(&buf, stats.Stats{
		nil,
		{GMAddress: "2401:db00::2e1:0", Selected: true, Offset: 39, MeanPathDelay: -67094920, PortChangeCount: 15,
			SearchState: searchStatePtr(asymmetry.SearchSearching), Priority3: 1},
		{GMAddress: "2401:db00::3fb:0", Offset: 2746, MeanPathDelay: -67102623,
			SearchState: searchStatePtr(asymmetry.SearchSettled), Priority3: 2},
	}, true))
	out := buf.String()

	require.Contains(t, out, "SELECTED")
	require.Contains(t, out, "PORT MOVES")
	require.Contains(t, out, "SEARCH")
	require.NotContains(t, out, "port_changes=", "the equals-sign format is replaced by the table")
	require.Contains(t, out, "searching", "a GM mid-search says so")
	require.Contains(t, out, "settled", "an idle GM says so")
	require.Contains(t, out, "|", "rendered as an ASCII table")
}

// A GM the corrector gave up on reports its spent budget, not a fresh zero.
func TestPrintGMStatsExhausted(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, printGMStats(&buf, stats.Stats{
		{GMAddress: "2401:db00::1", Selected: true, Offset: 2364, PortChangeCount: 64,
			SearchState: searchStatePtr(asymmetry.SearchExhausted)},
	}, true))
	require.Contains(t, buf.String(), "exhausted")
	require.NotContains(t, buf.String(), "settled")
}

// A host running no correction sends the field and calls it unknown, which the
// move count must not talk it out of.
func TestPrintGMStatsExplicitUnknownStaysUnknown(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, printGMStats(&buf, stats.Stats{
		{GMAddress: "2401:db00::1", Selected: true, PortChangeCount: 0,
			SearchState: searchStatePtr(asymmetry.SearchUnknown)},
	}, true))
	require.Contains(t, buf.String(), "unknown")
	require.NotContains(t, buf.String(), "settled")
}

// A value outside the enum names no state, and must not wrap into one.
func TestPrintGMStatsOutOfRangeStateIsUnknown(t *testing.T) {
	var buf bytes.Buffer
	outOfRange := int(asymmetry.SearchSettled) + 256
	require.NoError(t, printGMStats(&buf, stats.Stats{
		{GMAddress: "2401:db00::1", Selected: true, SearchState: &outOfRange},
	}, true))
	require.Contains(t, buf.String(), "unknown")
	require.NotContains(t, buf.String(), "settled")
}

func searchStatePtr(s asymmetry.SearchState) *int {
	v := int(s)
	return &v
}

// An sptp predating search_state omits it. The move count cannot tell an unspent
// asymmetric path from a settled one, so an omitted field names no state at all
// rather than publishing a plausible wrong one during independent deployment.
func TestPrintGMStatsOmittedStateIsUnknown(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, printGMStats(&buf, stats.Stats{
		{GMAddress: "2401:db00::1", Selected: true, PortChangeCount: 15, Priority3: 1},
		{GMAddress: "2401:db00::2", PortChangeCount: 0, Priority3: 2},
	}, true))
	out := buf.String()
	require.NotContains(t, out, "settled", "zero moves is not evidence the path is fine")
	require.NotContains(t, out, "searching", "and a spent count is not evidence a search is still running")
	require.Equal(t, 2, strings.Count(out, "unknown"), "both rows report no state")
	require.Contains(t, out, "15", "the move count itself is still shown")
}

func TestPrintGMStatsEmpty(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, printGMStats(&buf, stats.Stats{}, true))
	require.Contains(t, buf.String(), "ADDRESS")
}

// Resolution is on by default, so an address with no PTR still has to render.
func TestPrintGMStatsResolvingKeepsUnresolvableAddress(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, printGMStats(&buf, stats.Stats{
		{GMAddress: "2001:db8::1", Selected: true, Offset: 39},
	}, false))
	require.Contains(t, buf.String(), "2001:db8::1")
}

func TestGMNoResolvingFlag(t *testing.T) {
	f := gmCmd.Flags().Lookup("no-resolving")
	require.NotNil(t, f, "gm takes the same -n as sources")
	require.Equal(t, "n", f.Shorthand)
	require.Equal(t, "false", f.DefValue, "hostnames are shown unless asked otherwise")
}

// Every state the enum names must render as itself, so appending one cannot
// silently fall through the range check and read as unknown.
func TestPrintGMStatsRendersEveryNamedState(t *testing.T) {
	for _, s := range []asymmetry.SearchState{
		asymmetry.SearchUnknown, asymmetry.SearchAsymmetric,
		asymmetry.SearchSearching, asymmetry.SearchSettled, asymmetry.SearchExhausted,
	} {
		var buf bytes.Buffer
		require.NoError(t, printGMStats(&buf, stats.Stats{
			{GMAddress: "2401:db00::1", SearchState: searchStatePtr(s)},
		}, true))
		require.Contains(t, buf.String(), s.String())
	}
}

// A resolver that never answers must not hold the command open.
func TestPrintGMStatsDNSIsBounded(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		var buf bytes.Buffer
		done <- printGMStats(&buf, stats.Stats{{GMAddress: "2001:db8::1", Selected: true}}, false)
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(render.Timeout + 10*time.Second):
		t.Fatal("printGMStats did not return, so the lookup is unbounded")
	}
}
