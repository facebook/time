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

	"github.com/facebook/time/cmd/ptpcheck/checker"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(serviceStatsCmd)
	serviceStatsCmd.Flags().StringVarP(&rootServerFlag, "server", "S", "/var/run/ptp4l", "server to connect to")
}

func serviceStatsRun(server string) error {
	c, cleanup, err := checker.PrepareClient(server)
	defer cleanup()
	if err != nil {
		return fmt.Errorf("preparing connection: %w", err)
	}
	tlv, err := c.PortServiceStatsNP()
	if err != nil {
		return fmt.Errorf("talking to ptp4l: %w", err)
	}
	str, err := json.Marshal(tlv.PortServiceStats)
	if err != nil {
		return fmt.Errorf("marshaling json: %w", err)
	}
	fmt.Printf("%s\n", string(str))
	return nil
}

var serviceStatsCmd = &cobra.Command{
	Use:   "servicestats",
	Short: "Print PTP port service stats in JSON format",
	Run: func(c *cobra.Command, args []string) {
		ConfigureVerbosity()

		if err := serviceStatsRun(rootServerFlag); err != nil {
			log.Fatal(err)
		}

	},
}
