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
	"net"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/facebook/time/ptp/oscillatord"
)

var (
	oscillatordPortFlag    int
	oscillatordAddressFlag string
	oscillatorJSONFlag     bool
)

func init() {
	RootCmd.AddCommand(oscillatordCmd)
	oscillatordCmd.Flags().StringVarP(&oscillatordAddressFlag, "address", "a", "127.0.0.1", "address to connect to")
	oscillatordCmd.Flags().IntVarP(&oscillatordPortFlag, "port", "p", 2958, "port to connect to")
	oscillatordCmd.Flags().BoolVarP(&oscillatorJSONFlag, "json", "j", false, "JSON output")
}

func bool2int(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func printOscillatordJSON(status *oscillatord.Status) error {
	output := struct {
		Temperature       int64 `json:"ptp.timecard.temperature"`
		Lock              int64 `json:"ptp.timecard.lock"`
		GNSSFixNum        int64 `json:"ptp.timecard.gnss.fix_num"`
		GNSSFixOk         int64 `json:"ptp.timecard.gnss.fix_ok"`
		GNSSAntennaPower  int64 `json:"ptp.timecard.gnss.antenna_power"`
		GNSSAntennaStatus int64 `json:"ptp.timecard.gnss.antenna_status"`
		GNSSLSChange      int64 `json:"ptp.timecard.gnss.leap_second_change"`
		GNSSLeapSeconds   int64 `json:"ptp.timecard.gnss.leap_seconds"`
	}{
		Temperature:       int64(status.Oscillator.Temperature),
		Lock:              bool2int(status.Oscillator.Lock),
		GNSSFixNum:        int64(status.GNSS.Fix),
		GNSSFixOk:         bool2int(status.GNSS.FixOK),
		GNSSAntennaPower:  int64(status.GNSS.AntennaPower),
		GNSSAntennaStatus: int64(status.GNSS.AntennaStatus),
		GNSSLSChange:      int64(status.GNSS.LSChange),
		GNSSLeapSeconds:   int64(status.GNSS.LeapSeconds),
	}
	toPrint, err := json.Marshal(output)
	if err != nil {
		return err
	}
	fmt.Println(string(toPrint))
	return nil
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
}

func oscillatordRun(address string, jsonOut bool) error {
	timeout := 1 * time.Second
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return fmt.Errorf("connecting to oscillatord: %w", err)
	}
	defer conn.Close()
	deadline := time.Now().Add(timeout)
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("setting connection deadline: %w", err)
	}

	status, err := oscillatord.ReadStatus(conn)
	if err != nil {
		return err
	}

	if jsonOut {
		return printOscillatordJSON(status)
	}

	printOscillatord(status)

	return nil
}

var oscillatordCmd = &cobra.Command{
	Use:   "oscillatord",
	Short: "Print Time Card stats reported by oscillatord",
	Run: func(c *cobra.Command, args []string) {
		ConfigureVerbosity()
		address := net.JoinHostPort(oscillatordAddressFlag, fmt.Sprint(oscillatordPortFlag))
		if err := oscillatordRun(address, oscillatorJSONFlag); err != nil {
			log.Fatal(err)
		}
	},
}
