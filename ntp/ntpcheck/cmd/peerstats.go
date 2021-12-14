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

	"github.com/facebook/time/ntp/ntpcheck/checker"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func printPeerStats(r *checker.NTPCheckResult) error {
	output, err := checker.NewNTPPeerStats(r)
	if err != nil {
		return err
	}
	toPrint, err := json.Marshal(output)
	if err != nil {
		return err
	}
	fmt.Println(string(toPrint))
	return nil
}

func init() {
	RootCmd.AddCommand(peerstatsCmd)
	peerstatsCmd.Flags().StringVarP(&server, "server", "S", "", "server to connect to")
}

var peerstatsCmd = &cobra.Command{
	Use:   "peerstats",
	Short: "Print all NTP peers stats in JSON format",
	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()

		result, err := checker.RunCheck(server)
		if err != nil {
			log.Fatal(err)
		}
		err = printPeerStats(result)
		if err != nil {
			log.Fatal(err)
		}
	},
}
