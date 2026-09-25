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
	"context"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/facebook/time/cmd/ptpcheck/checker"
	"github.com/facebook/time/cmd/ptpcheck/render"
	"github.com/facebook/time/ptp/sptp/asymmetry"
	"github.com/facebook/time/ptp/sptp/stats"

	"github.com/olekukonko/tablewriter/tw"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var gmNoDNSFlag bool

func init() {
	RootCmd.AddCommand(gmCmd)
	gmCmd.Flags().StringVarP(&rootClientFlag, "client", "C", "", rootClientFlagDesc)
	gmCmd.Flags().BoolVarP(&gmNoDNSFlag, "no-resolving", "n", false, "disable resolving of IP addresses to hostnames")
}

var gmCmd = &cobra.Command{
	Use:   "gm",
	Short: "Report per-grandmaster offsets from the local sptp",
	Run: func(_ *cobra.Command, _ []string) {
		ConfigureVerbosity()

		if err := gmRun(rootClientFlag, gmNoDNSFlag); err != nil {
			log.Fatal(err)
		}
	},
}

func printGMStats(w io.Writer, gmStats stats.Stats, noDNS bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), render.Timeout)
	defer cancel()
	present := make(stats.Stats, 0, len(gmStats))
	for _, stat := range gmStats {
		if stat != nil {
			present = append(present, stat)
		}
	}
	sort.Sort(present)
	table := render.Table(w, []tw.Align{
		tw.AlignLeft,  // SELECTED
		tw.AlignLeft,  // ADDRESS
		tw.AlignRight, // OFFSET(NS)
		tw.AlignRight, // DELAY(NS)
		tw.AlignRight, // PORT MOVES
		tw.AlignLeft,  // SEARCH
		tw.AlignLeft,  // ERROR
	}, "SELECTED", "ADDRESS", "OFFSET(NS)", "DELAY(NS)", "PORT MOVES", "SEARCH", "ERROR")
	for _, stat := range present {
		err := table.Append([]string{
			fmt.Sprintf("%v", stat.Selected),
			render.Addr(ctx, stat.GMAddress, noDNS),
			fmt.Sprintf("%3.f", stat.Offset),
			fmt.Sprintf("%3.f", stat.MeanPathDelay),
			fmt.Sprintf("%d", stat.PortChangeCount),
			searchState(stat),
			stat.Error,
		})
		if err != nil {
			return err
		}
	}
	return table.Render()
}

// searchState names what the corrector is doing. An sptp predating the field omits
// it, and the move count cannot tell settled from asymmetric or exhausted.
func searchState(stat *stats.Stat) string {
	if stat.SearchState == nil {
		return asymmetry.SearchUnknown.String()
	}
	// the wire value is wider than the enum, so 257 must not alias a real state
	if s := *stat.SearchState; asymmetry.SearchStateNamed(s) {
		return asymmetry.SearchState(s).String() //nolint:gosec
	}
	return asymmetry.SearchUnknown.String()
}

func gmRun(server string, noDNS bool) error {
	gmStats, err := stats.FetchStats(checker.GetServerAddress(server, checker.FlavourSPTP))
	if err != nil {
		return fmt.Errorf("fetching sptp stats: %w", err)
	}
	return printGMStats(os.Stdout, gmStats, noDNS)
}
