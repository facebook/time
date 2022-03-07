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
	"fmt"
	"net"
	"os"
	"time"

	"github.com/facebook/time/cmd/ptpcheck/checker"
	"github.com/facebook/time/phc"
	ptp "github.com/facebook/time/ptp/protocol"

	"github.com/olekukonko/tablewriter"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	sourcesNoDNSFlag bool
)

func init() {
	RootCmd.AddCommand(sourcesCmd)
	sourcesCmd.Flags().StringVarP(&rootServerFlag, "server", "S", "/var/run/ptp4l", "server to connect to")
	sourcesCmd.Flags().BoolVarP(&sourcesNoDNSFlag, "no-resolving", "n", false, "disable resolving of IP addresses to hostnames")
}

func sourcesRun(server string, noDNS bool) error {
	c, cleanup, err := checker.PrepareClient(server)
	defer cleanup()
	if err != nil {
		return fmt.Errorf("preparing connection: %w", err)
	}
	ppn, err := c.PortPropertiesNP()
	if err != nil {
		return fmt.Errorf("getting PORT_PROPERTIES_NP from ptp4l: %w", err)
	}
	cds, err := c.CurrentDataSet()
	if err != nil {
		return fmt.Errorf("getting CURRENT_DATA_SET from ptp4l: %w", err)
	}
	tsn, err := c.TimeStatusNP()
	if err != nil {
		return fmt.Errorf("getting TIME_STATUS_NP from ptp4l: %w", err)
	}
	tlv, err := c.UnicastMasterTableNP()
	if err != nil {
		return fmt.Errorf("getting UNICAST_MASTER_TABLE_NP from ptp4l: %w", err)
	}
	// obtain time from clock that ptp4l uses
	var currentTime time.Time
	if ppn.Timestamping == ptp.TimestampingHardware {
		currentTime, err = phc.Time(string(ppn.Interface), phc.MethodIoctlSysOffsetExtended)
		if err != nil {
			log.Errorf("No PHC time data available: %v", err)
		}
	} else {
		currentTime = time.Now()
	}
	table := tablewriter.NewWriter(os.Stdout)
	table.SetColWidth(20)
	table.SetHeader([]string{
		"selected", "identity", "address", "state", "clock", "variance", "p1:p2", "offset(ns)", "delay(ns)", "last sync",
	})
	for _, entry := range tlv.UnicastMasterTable.UnicastMasters {
		address := entry.Address.String()
		if !noDNS {
			names, err := net.LookupAddr(address)
			if err == nil && len(names) > 0 {
				address = names[0]
			}
		}

		val := []string{
			fmt.Sprintf("%v", entry.Selected),
			entry.PortIdentity.String(),
			address,
			entry.PortState.String(),
		}
		if entry.PortState != ptp.UnicastMasterStateWait {
			val = append(val, []string{
				fmt.Sprintf("%d:0x%x", entry.ClockQuality.ClockClass, entry.ClockQuality.ClockAccuracy),
				fmt.Sprintf("0x%x", entry.ClockQuality.OffsetScaledLogVariance),
				fmt.Sprintf("%d:%d", entry.Priority1, entry.Priority2),
			}...)
		} else {
			val = append(val, []string{"", "", ""}...)
		}
		if entry.Selected {
			lastSync := "unknown"
			if tsn.IngressTimeNS == 0 {
				lastSync = "not syncing"
			} else if !currentTime.IsZero() {
				since := currentTime.Sub(time.Unix(0, tsn.IngressTimeNS))
				lastSync = fmt.Sprintf("%v", since)
			}
			val = append(val, []string{
				fmt.Sprintf("%3.f", cds.OffsetFromMaster.Nanoseconds()),
				fmt.Sprintf("%3.f", cds.MeanPathDelay.Nanoseconds()),
				lastSync,
			}...)
		} else {
			val = append(val, []string{"", "", ""}...)
		}
		table.Append(val)
	}
	table.Render()
	return nil
}

var sourcesCmd = &cobra.Command{
	Use:   "sources",
	Short: "Print PTP client unicast master table",
	Long:  "Print PTP client unicast master table. Like `chronyc sources`, but for PTP.",
	Run: func(c *cobra.Command, args []string) {
		ConfigureVerbosity()

		if err := sourcesRun(rootServerFlag, sourcesNoDNSFlag); err != nil {
			log.Fatal(err)
		}

	},
}
