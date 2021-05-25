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
	"math"
	"os"
	"time"

	"github.com/fatih/color"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/facebookincubator/ptp/phc"
	"github.com/facebookincubator/ptp/ptpcheck/checker"
)

// flag
var diagIfaceFlag string

type status int

// possible check results
const (
	OK status = iota
	WARN
	FAIL
	CRITICAL
)

// diagnoser is function that does checks on PTPCheckResult
type diagnoser func(r *checker.PTPCheckResult) (status, string)

var okString = color.GreenString("[ OK ]")
var warnString = color.YellowString("[WARN]")
var failString = color.RedString("[FAIL]")

var statusToColor = []string{okString, warnString, failString}

// generic function to check value against some thresholds
func checkAgainstThreshold(name string, value, warnThreshold, failThreshold float64, explanation string) (status, string) {
	msgTemplate := "%s is %s, we expect it to be within %s%s"
	absValue := math.Abs(value)
	thresholdStr := color.BlueString("%v", time.Duration(warnThreshold))
	if absValue > failThreshold {
		return FAIL, fmt.Sprintf(
			msgTemplate,
			name,
			color.RedString("%v", time.Duration(value)),
			thresholdStr,
			". "+explanation,
		)
	}
	if absValue > warnThreshold {
		return WARN, fmt.Sprintf(
			msgTemplate,
			name,
			color.YellowString("%v", time.Duration(value)),
			thresholdStr,
			". "+explanation,
		)
	}
	return OK, fmt.Sprintf(
		msgTemplate,
		name,
		color.GreenString("%v", time.Duration(value)),
		thresholdStr,
		"",
	)
}

func checkGMPresent(r *checker.PTPCheckResult) (status, string) {
	if !r.GrandmasterPresent {
		return FAIL, "GM is not present"
	}
	return OK, "GM is present"
}

func checkSyncActive(r *checker.PTPCheckResult) (status, string) {
	if r.IngressTimeNS == 0 {
		return WARN, "No ingress time data available"
	}

	phcTime, err := phc.Time(diagIfaceFlag, phc.MethodIoctlSysOffsetExtended)
	if err != nil {
		return WARN, fmt.Sprintf("No PHC time data available: %v", err)
	}
	// We expect to get sync messages at least every second
	const warnThreshold = float64(time.Second)
	const failThreshold = float64(5 * time.Second)
	lastSync := time.Unix(0, r.IngressTimeNS)
	since := phcTime.Sub(lastSync)
	return checkAgainstThreshold(
		"Period since last ingress",
		float64(since),
		warnThreshold,
		failThreshold,
		"We expect to receive SYNC messages from GM very often",
	)
}

func checkOffset(r *checker.PTPCheckResult) (status, string) {
	// We expect our clock difference from server to be no more than 250us.
	const warnThreshold = float64(250 * time.Microsecond)
	// If offset is > 1ms something is very very wrong
	const failThreshold = float64(time.Millisecond)
	return checkAgainstThreshold(
		"GM offset",
		r.OffsetFromMasterNS,
		warnThreshold,
		failThreshold,
		"Offset is the difference between our clock and remote server (time error).",
	)
}
func checkPathDelay(r *checker.PTPCheckResult) (status, string) {
	// We expect GM to be withing same region, so path delay should be relatively small
	const warnThreshold = float64(100 * time.Millisecond)
	// If path delay is > 250ms it's really weird
	const failThreshold = float64(250 * time.Millisecond)
	return checkAgainstThreshold(
		"GM mean path delay",
		r.MeanPathDelayNS,
		warnThreshold,
		failThreshold,
		"Mean path delay is measured network delay between us and GM",
	)
}

var diagnosers = []diagnoser{
	checkGMPresent,
	checkSyncActive,
	checkOffset,
	checkPathDelay,
}

func runDiagnosers(r *checker.PTPCheckResult) {
	for _, check := range diagnosers {
		status, msg := check(r)
		switch status {
		case CRITICAL:
			fmt.Printf("%s %s\n", failString, msg)
			os.Exit(1)
		default:
			fmt.Printf("%s %s\n", statusToColor[status], msg)
		}
	}
}

func init() {
	RootCmd.AddCommand(diagCmd)
	diagCmd.Flags().StringVarP(&rootServerFlag, "server", "S", "/var/run/ptp4l", "server to connect to")
	diagCmd.Flags().StringVarP(&diagIfaceFlag, "iface", "i", "eth0", "Network interface to get time from")
}

var diagCmd = &cobra.Command{
	Use:   "diag",
	Short: "Perform basic PTP diagnosis, report in human-readable form.",
	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()

		result, err := checker.RunCheck(rootServerFlag)
		if err != nil {
			log.Fatal(err)
		}
		runDiagnosers(result)
	},
}
