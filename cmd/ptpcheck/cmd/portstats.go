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
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/facebook/time/cmd/ptpcheck/checker"
)

func printPortStats(r *checker.PTPCheckResult) error {
	output := map[string]uint64{}
	for k, v := range r.PortStatsTX {
		kk := strings.ToLower(fmt.Sprintf("ptp.portstats.tx.%s", k))
		output[kk] = v
	}
	for k, v := range r.PortStatsRX {
		kk := strings.ToLower(fmt.Sprintf("ptp.portstats.rx.%s", k))
		output[kk] = v
	}

	toPrint, err := json.Marshal(output)
	if err != nil {
		return err
	}
	fmt.Println(string(toPrint))
	return nil
}

func init() {
	RootCmd.AddCommand(portStatsCmd)
	portStatsCmd.Flags().StringVarP(&rootClientFlag, "client", "C", "", rootClientFlagDesc)
}

var portStatsCmd = &cobra.Command{
	Use:   "portstats",
	Short: "Print PTP port stats in JSON format",
	Run: func(_ *cobra.Command, _ []string) {
		ConfigureVerbosity()

		domain, err := RootCmd.PersistentFlags().GetUint8("domain")
		if err != nil {
			log.Fatal(err)
		}
		result, err := checker.RunCheck(rootClientFlag, domain)
		if err != nil {
			log.Fatal(err)
		}
		err = printPortStats(result)
		if err != nil {
			log.Fatal(err)
		}
	},
}
