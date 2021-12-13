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
	"encoding/json"
	"fmt"
	"math"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/facebookincubator/time/ntp/ntpcheck/checker"
)

func printStats(r *checker.NTPCheckResult, legacy bool) error {
	type ntpStatsLegacy struct {
		checker.NTPStats
		SystemNTPStat float64 `json:"system.ntp_stat"`
	}

	output, err := checker.NewNTPStats(r)
	if err != nil {
		return err
	}
	if legacy {
		extraOutput := ntpStatsLegacy{
			NTPStats:      *output,
			SystemNTPStat: math.Abs(output.PeerOffset),
		}
		toPrint, err := json.Marshal(extraOutput)
		if err != nil {
			return err
		}
		fmt.Println(string(toPrint))
		return nil
	}
	toPrint, err := json.Marshal(output)
	if err != nil {
		return err
	}
	fmt.Println(string(toPrint))
	return nil
}

var legacyOutput = false

func init() {
	RootCmd.AddCommand(statsCmd)
	statsCmd.Flags().StringVarP(&server, "server", "S", "", "server to connect to")
	statsCmd.Flags().BoolVarP(&legacyOutput, "legacy", "", false, "output system.ntp_stat value for backwards compatibility")
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Print NTP stats in JSON format",
	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()

		result, err := checker.RunCheck(server)
		if err != nil {
			log.Fatal(err)
		}
		err = printStats(result, legacyOutput)
		if err != nil {
			log.Fatal(err)
		}
	},
}
