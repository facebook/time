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
	"context"
	"fmt"
	"math"
	"net/netip"
	"time"

	"github.com/facebook/time/cmd/ptpcheck/checker"
	"github.com/facebook/time/ptp/pdelay"
	"github.com/facebook/time/ptp/sptp/client"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// flags
var (
	countf    int
	timeoutf  time.Duration
	intervalf time.Duration
)

func init() {
	RootCmd.AddCommand(ptpingCmd)
	ptpingCmd.Flags().StringVarP(&rootClientFlag, "client", "C", "", sptpClientFlagDesc)
	ptpingCmd.Flags().IntVarP(&countf, "count", "c", 5, "number of probes to send")
	ptpingCmd.Flags().DurationVarP(&timeoutf, "timeout", "t", DefaultPingTimeout, "request timeout")
	ptpingCmd.Flags().DurationVarP(&intervalf, "interval", "i", time.Second, "interval between probes")
}

type timestamps struct {
	t1 time.Time
	t2 time.Time
	t3 time.Time
	t4 time.Time
}

func ptpingOutput(count int, server string, totalRTT time.Duration, ts timestamps) {
	fw := ts.t4.Sub(ts.t3)
	bk := ts.t2.Sub(ts.t1)
	netRTT := fw + bk
	// if we didn't get a valid T1 or T2, set the reverse latency to 0 (
	if ts.t1.IsZero() || ts.t2.IsZero() {
		bk = 0
	}
	if bk == 0 {
		fmt.Printf("%s: seq=%d net=%f (->%s + <-%f)\trtt=%s\n", server, count, math.NaN(), fw, math.NaN(), totalRTT)
	} else {
		fmt.Printf("%s: seq=%d net=%s (->%s + <-%s)\trtt=%s\n", server, count, netRTT, fw, bk, totalRTT)
	}
}

// resultToTimestamps maps the canonical T1..T4 of a ping onto ptping's view, where
// t3/t4 is the outbound leg and t1/t2 the return leg.
func resultToTimestamps(r *pdelay.Result) timestamps {
	return timestamps{
		t3: r.T1,
		t4: r.T2,
		t1: r.T3,
		t2: r.T4,
	}
}

func ptpingRun(ctx context.Context, sptpAddress string, server string, count int, timeout, interval time.Duration) error {
	if count <= 0 {
		return nil
	}
	if err := checkPingTimeout(timeout); err != nil {
		return err
	}
	// resolve here so the probe and the printed label name the same host
	target, err := client.LookupNetIP(server)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", server, err)
	}
	address := checker.GetServerAddress(sptpAddress, checker.FlavourSPTP)

	var succeeded int
	for c := 1; c <= count; c++ {
		if c > 1 {
			select {
			case <-ctx.Done():
			case <-time.After(interval):
			}
		}
		if err := ctx.Err(); err != nil {
			if succeeded > 0 {
				return nil
			}
			return err
		}
		wire, err := pdelay.FetchPing(ctx, address, target.String(), timeout)
		if err != nil {
			if ctx.Err() != nil && succeeded > 0 {
				return nil
			}
			log.Errorf("failed to send request: %s", err)
			continue
		}
		res := pickResponder(wire, target)
		if res == nil {
			log.Errorf("failed to read sync response: no result for %s", target)
			continue
		}
		if res.Error != nil {
			log.Errorf("failed to read sync response: %v", res.Error)
			continue
		}
		// zero timestamps would render as bogus fw/bk values
		if !res.Valid() {
			log.Errorf("incomplete response from %s", target)
			continue
		}
		// SWRTT is the only round trip source here, so a zero means missing data
		if res.SWRTT <= 0 {
			log.Errorf("no round trip time in response from %s", target)
			continue
		}
		succeeded++
		ptpingOutput(c, server, res.SWRTT, resultToTimestamps(res))
	}
	if succeeded == 0 {
		return fmt.Errorf("no successful probes to %s out of %d", server, count)
	}
	return nil
}

// pickResponder returns the result belonging to target. Canonical forms are
// compared: LookupNetIP can return ::ffff:a.b.c.d while sptp Unmap()s.
func pickResponder(wire pdelay.Results, target netip.Addr) *pdelay.Result {
	want := target.Unmap().WithZone("")
	for _, r := range wire {
		if r.Responder.Unmap().WithZone("") == want {
			return r
		}
	}
	// a lone result with no responder is still ours: we named the target
	if len(wire) == 1 && !wire[0].Responder.IsValid() {
		return wire[0]
	}
	return nil
}

var ptpingCmd = &cobra.Command{
	Use:        "ptping {server}",
	Short:      "sptp-based ping",
	Long:       "measure real network latency between 2 sptp-enabled hosts",
	Args:       cobra.ExactArgs(1),
	ArgAliases: []string{"server"},
	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()

		if err := ptpingRun(cmd.Context(), rootClientFlag, args[0], countf, timeoutf, intervalf); err != nil {
			log.Fatal(err)
		}
	},
}
