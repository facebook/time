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
	"golang.org/x/exp/constraints"

	"github.com/facebook/time/cmd/ptpcheck/checker"
	"github.com/facebook/time/phc"
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

func fmtThreshold(warnThreshold any) string {
	return color.BlueString("%v", warnThreshold)
}

func checkAgainstThresholdPositive[T constraints.Signed](name string, value, warnThreshold, failThreshold T, explanation string) (status, string) {
	var zero T // can't use 0 as untyped const, so use zero value for each type
	if value <= zero {
		return FAIL, fmt.Sprintf(
			"%s is %s, we expect it to be positive and within %s%s",
			name,
			color.RedString("%v", value),
			fmtThreshold(warnThreshold),
			". "+explanation,
		)
	}
	return checkAgainstThreshold(name, value, warnThreshold, failThreshold, explanation)
}

func checkAgainstThresholdNonZero[T constraints.Ordered](name string, value, warnThreshold, failThreshold T, explanation string) (status, string) {
	var zero T // can't use 0 as untyped const, so use zero value for each type
	if value == zero {
		return FAIL, fmt.Sprintf(
			"%s is %s, we expect it to be non-zero and within %s%s",
			name,
			color.RedString("%v", value),
			fmtThreshold(warnThreshold),
			". "+explanation,
		)
	}
	return checkAgainstThreshold(name, value, warnThreshold, failThreshold, explanation)
}

// generic function to check value against some thresholds
func checkAgainstThreshold[T constraints.Ordered](name string, value, warnThreshold, failThreshold T, explanation string) (status, string) {
	msgTemplate := "%s is %s, we expect it to be within %s%s"
	thresholdStr := fmtThreshold(warnThreshold)

	if value > failThreshold {
		return FAIL, fmt.Sprintf(
			msgTemplate,
			name,
			color.RedString("%v", value),
			thresholdStr,
			". "+explanation,
		)
	}
	if value > warnThreshold {
		return WARN, fmt.Sprintf(
			msgTemplate,
			name,
			color.YellowString("%v", value),
			thresholdStr,
			". "+explanation,
		)
	}
	return OK, fmt.Sprintf(
		msgTemplate,
		name,
		color.GreenString("%v", value),
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
	const warnThreshold = time.Second
	const failThreshold = 5 * time.Second
	lastSync := time.Unix(0, r.IngressTimeNS)
	since := phcTime.Sub(lastSync)
	if since < 0 {
		return FAIL, fmt.Sprintf("Last synchronization (%v) happened in the future compared to now (%v)", lastSync, phcTime)
	}
	return checkAgainstThreshold(
		"Period since last ingress",
		since,
		warnThreshold,
		failThreshold,
		"We expect to receive SYNC messages from GM very often",
	)
}

func checkOffset(r *checker.PTPCheckResult) (status, string) {
	// We expect our clock difference from server to be no more than 250us.
	const warnThreshold = 250 * time.Microsecond
	// If offset is > 1ms something is very very wrong
	const failThreshold = time.Millisecond
	return checkAgainstThresholdNonZero(
		"GM offset",
		time.Duration(math.Abs(r.OffsetFromMasterNS)),
		warnThreshold,
		failThreshold,
		"Offset is the difference between our clock and remote server (time error).",
	)
}
func checkPathDelay(r *checker.PTPCheckResult) (status, string) {
	// We expect GM to be within same region, so path delay should be relatively small
	const warnThreshold = 100 * time.Millisecond
	// If path delay is > 250ms it's really weird
	const failThreshold = 250 * time.Millisecond
	return checkAgainstThresholdPositive(
		"GM mean path delay",
		time.Duration(r.MeanPathDelayNS),
		warnThreshold,
		failThreshold,
		"Mean path delay is measured network delay between us and GM",
	)
}

func portServiceStatsDiagnosers(r *checker.PTPCheckResult) []diagnoser {
	result := []diagnoser{}
	// counters are reset on ptp4l restart
	var maxPacketsLoss uint64 = 100

	type l struct {
		name        string
		value       uint64
		threshold   uint64
		explanation string
	}
	checks := []l{
		{
			name:        "Sync timeout count",
			value:       r.PortServiceStats.SyncTimeout,
			threshold:   maxPacketsLoss,
			explanation: "We expect to not skip sync packets",
		},
		{
			name:        "Announce timeout count",
			value:       r.PortServiceStats.AnnounceTimeout,
			threshold:   maxPacketsLoss,
			explanation: "We expect to not skip announce packets",
		},
		{
			name:        "Sync mismatch count",
			value:       r.PortServiceStats.SyncMismatch,
			threshold:   maxPacketsLoss,
			explanation: "We expect sync packets to arrive in correct order",
		},
		{
			name:        "FollowUp mismatch count",
			value:       r.PortServiceStats.FollowupMismatch,
			threshold:   maxPacketsLoss,
			explanation: "We expect FollowUp packets to arrive in correct order",
		},
	}
	for _, check := range checks {
		var f diagnoser
		check := check // capture loop variable
		f = func(_ *checker.PTPCheckResult) (status, string) {
			return checkAgainstThreshold(
				check.name,
				check.value,
				check.threshold,
				10*check.threshold,
				check.explanation,
			)
		}
		result = append(result, f)
	}
	return result
}

// expandDiagnosers returns extra diagnosers based on the checker.PTPCheckResult content.
// For example, if PORT_SERVICE_STATS_NP TLV is supported by ptp4l, we run tests against it.
func expandDiagnosers(r *checker.PTPCheckResult) []diagnoser {
	extra := []diagnoser{}
	if r.PortServiceStats == nil {
		return extra
	}
	return portServiceStatsDiagnosers(r)
}

var diagnosers = []diagnoser{
	checkGMPresent,
	checkSyncActive,
	checkOffset,
	checkPathDelay,
}

func runDiagnosers(r *checker.PTPCheckResult, toRun []diagnoser) int {
	failed := 0
	for _, check := range toRun {
		status, msg := check(r)
		if status != OK {
			failed++
		}
		switch status {
		case CRITICAL:
			fmt.Printf("%s %s\n", failString, msg)
			return 127
		default:
			fmt.Printf("%s %s\n", statusToColor[status], msg)
		}
	}
	return failed
}

func runAllDiagnosers(r *checker.PTPCheckResult) int {
	extra := expandDiagnosers(r)
	toRun := append(diagnosers, extra...)
	return runDiagnosers(r, toRun)
}

func init() {
	RootCmd.AddCommand(diagCmd)
	diagCmd.Flags().StringVarP(&rootClientFlag, "client", "C", "", rootClientFlagDesc)
	diagCmd.Flags().StringVarP(&diagIfaceFlag, "iface", "i", "eth0", "Network interface to get time from")
}

var diagCmd = &cobra.Command{
	Use:   "diag",
	Short: "Perform basic PTP diagnosis, report in human-readable form.",
	Long: `Perform basic PTP diagnosis, report in human-readable form.
Runs a set of checks against the PTP client, and prints the results.
Exit code will be equal to sum of failed check, or 127 in case of critical problem.
`,
	Run: func(_ *cobra.Command, _ []string) {
		ConfigureVerbosity()

		result, err := checker.RunCheck(rootClientFlag)
		if err != nil {
			log.Fatal(err)
		}
		exitCode := runAllDiagnosers(result)
		os.Exit(exitCode)
	},
}
