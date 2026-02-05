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
	"github.com/facebook/time/ptp/sptp/stats"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(serviceStatsCmd)
	serviceStatsCmd.Flags().StringVarP(&rootClientFlag, "client", "C", "", rootClientFlagDesc)
}

func serviceStatsRunPTP4l(address string, domainNumber uint8) error {
	c, cleanup, err := checker.PrepareMgmtClient(address)
	defer cleanup()
	if err != nil {
		return fmt.Errorf("preparing connection: %w", err)
	}
	c.SetDomainNumber(domainNumber)
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

func serviceStatsRunSPTP(address string) error {
	sysStats, err := stats.FetchSysStats(address)
	if err != nil {
		return err
	}
	str, err := json.Marshal(sysStats)
	if err != nil {
		return fmt.Errorf("marshaling json: %w", err)
	}
	fmt.Printf("%s\n", string(str))
	return nil
}

func serviceStatsRun(address string, domainNumber uint8) error {
	f := checker.GetFlavour()
	address = checker.GetServerAddress(address, f)
	switch f {
	case checker.FlavourPTP4L:
		return serviceStatsRunPTP4l(address, domainNumber)
	case checker.FlavourSPTP:
		return serviceStatsRunSPTP(address)
	}
	return fmt.Errorf("uknown PTP client flavour %v", f)
}

var serviceStatsCmd = &cobra.Command{
	Use:   "servicestats",
	Short: "Print PTP port service stats in JSON format",
	Run: func(_ *cobra.Command, _ []string) {
		ConfigureVerbosity()
		domain, err := RootCmd.PersistentFlags().GetUint8("domain")
		if err != nil {
			log.Fatal(err)
		}
		if err := serviceStatsRun(rootClientFlag, domain); err != nil {
			log.Fatal(err)
		}

	},
}
