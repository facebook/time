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
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/facebook/time/fbclock"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	fbclockRequestsFlag int64
	fbclockDurationFlag time.Duration
	fbclockUTCFlag      bool
)

func init() {
	RootCmd.AddCommand(fbclockCmd)
	fbclockCmd.Flags().Int64VarP(&fbclockRequestsFlag, "requests", "r", 1, "number of requests to fbclock")
	fbclockCmd.Flags().DurationVarP(&fbclockDurationFlag, "duration", "t", 1*time.Second, "spread the requests over this duration")
	fbclockCmd.Flags().BoolVarP(&fbclockUTCFlag, "utc", "", false, "get UTC time (TAI is default)")
}

// formatErrorCauses orders by count, then by message so runs are comparable.
func formatErrorCauses(causes map[string]int64) string {
	msgs := slices.SortedFunc(maps.Keys(causes), func(a, b string) int {
		return cmp.Or(cmp.Compare(causes[b], causes[a]), cmp.Compare(a, b))
	})
	var b strings.Builder
	for _, msg := range msgs {
		fmt.Fprintf(&b, "  %d\t%s\n", causes[msg], msg)
	}
	return b.String()
}

func fbclockSample(tt *fbclock.TrueTime, prefix string, out map[string]int64) {
	out[prefix+"wou_ns"] = tt.Latest.Sub(tt.Earliest).Nanoseconds()
	out[prefix+"latest_ns"] = tt.Latest.UnixNano()
	out[prefix+"earliest_ns"] = tt.Earliest.UnixNano()
}

// Every key here is one ODS timeseries on every PTP client host. Fixed set,
// pinned by TestFbclockMetricKeys.
func fbclockMetrics(s fbclock.Stats, prefix, suffix string, out map[string]int64) {
	out[prefix+"wou_ns.avg"+suffix] = s.WOUAvg
	out[prefix+"wou_ns.max"+suffix] = s.WOUMax
	out[prefix+"wou_lt_10us.sum"+suffix] = s.WOUlt10us
	out[prefix+"wou_lt_100us.sum"+suffix] = s.WOUlt100us
	out[prefix+"wou_lt_1000us.sum"+suffix] = s.WOUlt1000us
	out[prefix+"wou_ge_1000us.sum"+suffix] = s.WOUge1000us
	out[prefix+"errors.sum"+suffix] = s.Errors
	out[prefix+"requests.sum"+suffix] = s.Requests
}

func fbclockRun(requests int64, duration time.Duration, utc bool) error {
	prefix := "ptp.fbclock_synthetic.api."
	suffix := fmt.Sprintf(".%d", int(duration.Seconds()))

	clock, err := fbclock.NewFBClockV2()
	if err != nil {
		return err
	}
	defer clock.Close()
	type res struct {
		tt  *fbclock.TrueTime
		err error
	}

	sleepTime := duration / time.Duration(requests)
	c := make(chan *res, requests)
	for i := range int(requests) {
		// we want to spread all 'requests' over 'duration'
		if i != 0 && sleepTime != 0 {
			time.Sleep(sleepTime)
		}
		go func() {
			var tt *fbclock.TrueTime
			var err error
			if utc {
				tt, err = clock.GetTimeUTC()
			} else {
				tt, err = clock.GetTime()
			}
			c <- &res{
				tt,
				err,
			}
		}()
	}

	sc := &fbclock.StatsCollector{}
	out := map[string]int64{}
	for range int(requests) {
		r := <-c
		sc.Update(r.tt, r.err)
		if r.err != nil {
			continue
		}
		// what we got (latest sample)
		fbclockSample(r.tt, prefix, out)
	}

	s := sc.Stats()
	fbclockMetrics(s, prefix, suffix, out)

	toPrint, err := json.Marshal(out)
	if err != nil {
		return err
	}
	fmt.Println(string(toPrint))

	if requests > 1 {
		fmt.Fprintf(os.Stderr, "Running clock.GetTime %d times over %v. Average WOU size is: %d\n", requests, duration, s.WOUAvg)
	}
	// stderr: the fbagent collector parses stdout as JSON.
	if s.Errors > 0 {
		fmt.Fprintf(os.Stderr, "%d of %d clock.GetTime calls failed:\n%s",
			s.Errors, s.Requests, formatErrorCauses(sc.ErrorCauses()))
	}
	return nil
}

var fbclockCmd = &cobra.Command{
	Use:   "fbclock",
	Short: "Print fbclock TrueTime",
	Run: func(_ *cobra.Command, _ []string) {
		ConfigureVerbosity()

		if fbclockRequestsFlag < 1 {
			log.Fatal("requests must be greater than 0")
		}

		if fbclockDurationFlag < 0 {
			log.Fatal("duration must be 0 or positive")
		}

		if err := fbclockRun(fbclockRequestsFlag, fbclockDurationFlag, fbclockUTCFlag); err != nil {
			log.Fatal(err)
		}

	},
}
