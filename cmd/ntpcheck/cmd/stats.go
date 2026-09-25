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
	"errors"
	"fmt"
	"io/fs"
	"math"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/facebook/time/cmd/ntpcheck/checker"
	"github.com/facebook/time/leapsectz"
	"github.com/facebook/time/phc"
)

// fbclock's copy of the PHC sptp disciplines, not whichever NIC comes first
var ptpDevice = "/dev/fbclock/ptp"

// readPHC measures the system clock (TAI) against the PHC in ms, nil without a reading.
func readPHC() *float64 {
	// EXTENDED is what the fbclock library reads the PHC with, so every PTP host supports it
	sysoff, err := phc.TimeAndOffsetFromDevice(ptpDevice, phc.MethodIoctlSysOffsetExtendedRealTimeClock)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		log.Warningf("reading %s: %v", ptpDevice, err)
		return nil
	}
	// the PHC keeps TAI; without the table every reading is 37s out
	leaps, err := leapsectz.Parse("")
	if err != nil {
		log.Warningf("reading leap second table: %v", err)
		return nil
	}
	offset := sysoff.Offset + time.Duration(leapsectz.UTCOffsetS(leaps, sysoff.SysTime))*time.Second
	ms := float64(offset) / float64(time.Millisecond)
	return &ms
}

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
			SystemNTPStat: math.Abs(output.Offset),
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
	Run: func(_ *cobra.Command, _ []string) {
		ConfigureVerbosity()
		// before the check, so it lands within ms of the tracking read
		var phcOffset *float64
		if server == "" {
			phcOffset = readPHC()
		}
		result, err := checker.RunCheck(server)
		if err != nil {
			log.Fatal(err)
		}
		result.PHCOffsetMS = phcOffset
		err = printStats(result, legacyOutput)
		if err != nil {
			log.Fatal(err)
		}
	},
}
