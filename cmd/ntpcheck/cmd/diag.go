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
	"strings"
	"time"

	"github.com/facebook/time/cmd/ntpcheck/checker"
	"github.com/fatih/color"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type status int

// possible check results
const (
	OK status = iota
	WARN
	FAIL
	CRITICAL
)

// diagnoser is function that does checks on NTPCheckResult
type diagnoser func(r *checker.NTPCheckResult) (status, string)

var okString = color.GreenString("[ OK ]")
var warnString = color.YellowString("[WARN]")
var failString = color.RedString("[FAIL]")

var statusToColor = []string{okString, warnString, failString}

func formatPeers(peers []string) string {
	return "\t" + strings.Join(peers, "\n\t")
}

// generic function to check value against some thresholds
func checkAgainstThreshold(name string, value, warnThreshold, failThreshold float64, explanation string) (status, string) {
	msgTemplate := "%s is %s, we expect it to be within %s%s"
	absValue := math.Abs(value)
	thresholdStr := color.BlueString("%.1fms", warnThreshold)
	if absValue > failThreshold {
		return FAIL, fmt.Sprintf(
			msgTemplate,
			name,
			color.RedString("%.3fms", value),
			thresholdStr,
			". "+explanation,
		)
	}
	if absValue > warnThreshold {
		return WARN, fmt.Sprintf(
			msgTemplate,
			name,
			color.YellowString("%.3fms", value),
			thresholdStr,
			". "+explanation,
		)
	}
	return OK, fmt.Sprintf(
		msgTemplate,
		name,
		color.GreenString("%.3fms", value),
		thresholdStr,
		"",
	)
}

// all checks logic comes from http://doc.ntp.org/current-stable/debug.html

func checkSync(r *checker.NTPCheckResult) (status, string) {
	syspeer, err := r.FindSysPeer()
	if err != nil {
		return CRITICAL, "No sys peer, clock is not syncing"
	}
	if r.LI == 3 {
		return FAIL, "Clock is not fully syncronized, leap indicator is set to 'alarm'"
	}
	return OK, fmt.Sprintf("Clock is syncing to %s", color.BlueString(syspeer.SRCAdr))
}

func checkLeap(r *checker.NTPCheckResult) (status, string) {
	if r.LI != 0 {
		return WARN, fmt.Sprintf("Leap indicator is set to '%s'", r.LIDesc)
	}
	return OK, "Leap indicator is set to 'none'"
}

func checkJitter(r *checker.NTPCheckResult) (status, string) {
	syspeer, err := r.FindSysPeer()
	if err != nil {
		return CRITICAL, "No sys peer, clock is not syncing"
	}
	// We expect jitter (mean deviation of multiple time samples) to be relatively low.
	// We don't have formal SLA, but 1ms is a reasonable expectation on healthy network.
	const warnThreshold = 1.0
	// 1s of jitter is a dead giveaway for network problems.
	const failThreshold = 1000.0
	return checkAgainstThreshold(
		"Sys Peer jitter",
		syspeer.Jitter,
		warnThreshold,
		failThreshold,
		"Jitter is the mean deviation of multiple time samples from remote server.",
	)
}

func checkCorrectionMetric(r *checker.NTPCheckResult) (status, string) {
	const warnThreshold time.Duration = time.Minute
	const failThreshold time.Duration = 10 * time.Minute
	var correctionInMilliseconds = r.Correction * 1000.0
	return checkAgainstThreshold(
		"Current correction",
		correctionInMilliseconds,
		float64(warnThreshold.Milliseconds()),
		float64(failThreshold.Milliseconds()),
		"Correction is the difference between system time and chronyd’s estimate of the current true time.",
	)
}

func checkOffset(r *checker.NTPCheckResult) (status, string) {
	syspeer, err := r.FindSysPeer()
	if err != nil {
		return CRITICAL, "No sys peer, clock is not syncing"
	}
	// We expect our clock difference from server to be no more than 1ms.
	// Currently there is no SLA, so it's just a warning.
	const warnThreshold = 1.0
	// If offset is > 1s something is very very wrong
	const failThreshold = 1000.0
	return checkAgainstThreshold(
		"Sys Peer offset",
		syspeer.Offset,
		warnThreshold,
		failThreshold,
		"Offset is the difference between our clock and remote server (time error).",
	)
}

func checkPeersFlash(r *checker.NTPCheckResult) (status, string) {
	badPeers := []string{}
	total := len(r.Peers)
	for _, peer := range r.Peers {
		if peer.Flash != 0 {
			badPeers = append(
				badPeers,
				fmt.Sprintf(
					"Peer %s has flashers [%s]",
					color.BlueString(peer.SRCAdr),
					color.YellowString(strings.Join(peer.Flashers, " ")),
				),
			)
		}
	}
	if len(badPeers) > 0 {
		badCount := len(badPeers)
		msg := fmt.Sprintf("%d peers are OK (have 'flash' indicator set to 0), %d peers have problems:\n", total-badCount, badCount)
		return WARN, msg + formatPeers(badPeers)
	}
	return OK, fmt.Sprintf("All %d peers are OK (have 'flash' indicator set to 0)", total)
}

func checkPeersReach(r *checker.NTPCheckResult) (status, string) {
	badPeers := []string{}
	total := len(r.Peers)
	for _, peer := range r.Peers {
		if peer.Reach != 255 {
			badPeers = append(
				badPeers,
				fmt.Sprintf(
					"Peer %s has reach %s",
					color.BlueString(peer.SRCAdr),
					color.YellowString("%08b", peer.Reach),
				),
			)
		}
	}
	if len(badPeers) > 0 {
		badCount := len(badPeers)
		msg := fmt.Sprintf("%d peers are fully reachable, %d peers had reachability problems:\n", total-badCount, badCount)
		return WARN, msg + formatPeers(badPeers)
	}
	return OK, fmt.Sprintf("All %d peers were reachable 8/8 last sync attempts", total)
}

var diagnosers = []diagnoser{
	checkSync,
	checkLeap,
	checkOffset,
	checkJitter,
	checkCorrectionMetric,
	checkPeersFlash,
	checkPeersReach,
}

func runDiagnosers(r *checker.NTPCheckResult) {
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
	diagCmd.Flags().StringVarP(&server, "server", "S", "", "server to connect to")
}

const desc = "Perform basic NTP diagnosis, report in human-readable form."

var diagCmd = &cobra.Command{
	Use:   "diag",
	Short: desc,
	Long:  desc + "\nIf you need more information, please refer to http://doc.ntp.org/current-stable/debug.html",
	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()

		result, err := checker.RunCheck(server)
		if err != nil {
			log.Fatal(err)
		}
		runDiagnosers(result)
	},
}
