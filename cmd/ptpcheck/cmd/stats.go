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

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/facebook/time/cmd/ptpcheck/checker"
)

func printStats(r *checker.PTPCheckResult) error {
	type stats struct {
		Offset            float64 `json:"ptp.offset_ns"`
		MeanPathDelay     float64 `json:"ptp.mean_path_delay_ns"`
		StepsRemoved      int     `json:"ptp.steps_removed"`
		GMPresent         int     `json:"ptp.gm_present"` // bool for ODS
		CorrectionFieldRX int64   `json:"ptp.cf_rx,omitempty"`
		CorrectionFieldTX int64   `json:"ptp.cf_tx,omitempty"`
	}

	output := stats{
		Offset:            r.OffsetFromMasterNS,
		MeanPathDelay:     r.MeanPathDelayNS,
		StepsRemoved:      r.StepsRemoved,
		GMPresent:         0,
		CorrectionFieldRX: r.CorrectionFieldRxNS,
		CorrectionFieldTX: r.CorrectionFieldTxNS,
	}
	if r.GrandmasterPresent {
		output.GMPresent = 1
	}

	toPrint, err := json.Marshal(output)
	if err != nil {
		return err
	}
	fmt.Println(string(toPrint))
	return nil
}

func init() {
	RootCmd.AddCommand(statsCmd)
	statsCmd.Flags().StringVarP(&rootClientFlag, "client", "C", "", rootClientFlagDesc)
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Print PTP stats in JSON format",
	Run: func(_ *cobra.Command, _ []string) {
		ConfigureVerbosity()

		result, err := checker.RunCheck(rootClientFlag)
		if err != nil {
			log.Fatal(err)
		}
		err = printStats(result)
		if err != nil {
			log.Fatal(err)
		}
	},
}
