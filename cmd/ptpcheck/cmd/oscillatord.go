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
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/facebook/time/oscillatord"
)

var (
	oscillatordPortFlag      int
	oscillatordAddressFlag   string
	oscillatorJSONFlag       bool
	oscillatorJSONPrefixFlag string
)

func init() {
	RootCmd.AddCommand(oscillatordCmd)
	oscillatordCmd.Flags().StringVarP(&oscillatordAddressFlag, "address", "a", "127.0.0.1", "address to connect to")
	oscillatordCmd.Flags().IntVarP(&oscillatordPortFlag, "port", "p", oscillatord.MonitoringPort, "port to connect to")
	oscillatordCmd.Flags().BoolVarP(&oscillatorJSONFlag, "json", "j", false, "JSON output")
	oscillatordCmd.Flags().StringVarP(&oscillatorJSONPrefixFlag, "prefix", "r", "ptp.timecard", "JSON prefix")
}

func printOscillatord(status *oscillatord.Status) {
	fmt.Println("Oscillator:")
	fmt.Printf("\tmodel: %s\n", status.Oscillator.Model)
	fmt.Printf("\tfine_ctrl: %d\n", status.Oscillator.FineCtrl)
	fmt.Printf("\tcoarse_ctrl: %d\n", status.Oscillator.CoarseCtrl)
	fmt.Printf("\tlock: %v\n", status.Oscillator.Lock)
	fmt.Printf("\ttemperature: %.2fC\n", status.Oscillator.Temperature)

	fmt.Println("GNSS:")
	fmt.Printf("\tfix: %s (%d)\n", status.GNSS.Fix, status.GNSS.Fix)
	fmt.Printf("\tfixOk: %v\n", status.GNSS.FixOK)
	fmt.Printf("\tantenna_power: %s (%d)\n", status.GNSS.AntennaPower, status.GNSS.AntennaPower)
	fmt.Printf("\tantenna_status: %s (%d)\n", status.GNSS.AntennaStatus, status.GNSS.AntennaStatus)
	fmt.Printf("\tleap_second_change: %s (%d)\n", status.GNSS.LSChange, status.GNSS.LSChange)
	fmt.Printf("\tleap_seconds: %d\n", status.GNSS.LeapSeconds)
	fmt.Printf("\tsatellites_count: %d\n", status.GNSS.SatellitesCount)
	fmt.Printf("\ttime_accuracy: %d\n", status.GNSS.TimeAccuracy)

	fmt.Println("Clock:")
	fmt.Printf("\tclass: %s (%d)\n", status.Clock.Class, status.Clock.Class)
	fmt.Printf("\toffset: %d\n", status.Clock.Offset)
}

func oscillatordRun(address string, jsonOut bool) error {
	timeout := 1 * time.Second
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return fmt.Errorf("connecting to oscillatord: %w", err)
	}
	defer conn.Close()
	deadline := time.Now().Add(timeout)
	if err = conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("setting connection deadline: %w", err)
	}

	status, err := oscillatord.ReadStatus(conn)
	if err != nil {
		return err
	}

	if jsonOut {
		toPrint, err := status.MonitoringJSON(oscillatorJSONPrefixFlag)
		fmt.Println(string(toPrint))
		return err
	}

	printOscillatord(status)

	return nil
}

var oscillatordCmd = &cobra.Command{
	Use:   "oscillatord",
	Short: "Print Time Card stats reported by oscillatord",
	Run: func(_ *cobra.Command, _ []string) {
		ConfigureVerbosity()
		address := net.JoinHostPort(oscillatordAddressFlag, fmt.Sprint(oscillatordPortFlag))
		if err := oscillatordRun(address, oscillatorJSONFlag); err != nil {
			log.Fatal(err)
		}
	},
}
