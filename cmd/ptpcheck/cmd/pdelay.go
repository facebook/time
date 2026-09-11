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
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/facebook/time/cmd/ptpcheck/checker"
	"github.com/facebook/time/ptp/pdelay"
	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/sptp/client"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// Default probe configuration
const (
	// DefaultProbeInterval is 5 minutes as per PTP in-rack linearizability requirements
	DefaultProbeInterval = 5 * time.Minute
	// DefaultProbeJitterMax is maximum random jitter to add to probe interval
	DefaultProbeJitterMax = 30 * time.Second
	// DefaultPingTimeout tracks sptp's collection window, which it must exceed
	DefaultPingTimeout = client.PingTimeout + time.Second
	// sptpClientFlagDesc replaces rootClientFlagDesc: this path speaks HTTP, so a
	// ptp4l unix socket would fail with an opaque URL error
	sptpClientFlagDesc = "HTTP endpoint of the local sptp monitoring server. Empty means detect automatically."
)

// flags for pdelay command
var (
	pdelayIntervalf time.Duration
	pdelayJitterf   time.Duration
	pdelayTimeoutf  time.Duration
	pdelayIPv4f     bool
	pdelayCountf    int
)

func init() {
	RootCmd.AddCommand(pdelayCmd)
	pdelayCmd.Flags().DurationVarP(&pdelayIntervalf, "interval", "I", DefaultProbeInterval, "probe interval (e.g., 5m)")
	pdelayCmd.Flags().DurationVarP(&pdelayJitterf, "jitter", "j", DefaultProbeJitterMax, "maximum random jitter to add to interval")
	pdelayCmd.Flags().DurationVarP(&pdelayTimeoutf, "timeout", "t", DefaultPingTimeout,
		fmt.Sprintf("timeout for the sptp request; must exceed the %s sptp spends collecting responses", client.PingTimeout))
	pdelayCmd.Flags().BoolVarP(&pdelayIPv4f, "ipv4", "4", false, "use IPv4 multicast (224.0.0.107) instead of IPv6 (ff02::6b)")
	pdelayCmd.Flags().IntVarP(&pdelayCountf, "count", "c", 0, "number of peer delay requests to send (0 = run continuously with interval)")
	pdelayCmd.Flags().StringVarP(&rootClientFlag, "client", "C", "", sptpClientFlagDesc)
}

// CalculateJitter returns a random duration between 0 and maxJitter
func CalculateJitter(maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(maxJitter)))
}

// ProbeConfig holds configuration for periodic probing
type ProbeConfig struct {
	// Server is the sptp monitoring address, empty means autodetect
	Server   string
	Interval time.Duration
	Jitter   time.Duration
	Timeout  time.Duration
	IPv4     bool
	// Count is the number of probe cycles to run. 0 means run continuously.
	Count int
}

// checkPingTimeout rejects a timeout that cannot outlive sptp's collection window,
// which would otherwise abort every request client-side
func checkPingTimeout(timeout time.Duration) error {
	if timeout <= client.PingTimeout {
		return fmt.Errorf("timeout %s must exceed the %s sptp spends collecting", timeout, client.PingTimeout)
	}
	return nil
}

// multicastGroup is the IEEE 1588 peer delay group this probe targets
func (c ProbeConfig) multicastGroup() string {
	if c.IPv4 {
		return ptp.PDelayMulticastIPv4
	}
	return ptp.PDelayMulticastIPv6
}

// ProbeResultCallback is called with all measurement results after each probe cycle
type ProbeResultCallback func(results []*pdelay.Result)

// OnProbeResult is an optional callback that gets called with all results from each probe cycle.
// Set this from external packages (e.g., internal) to add custom logging.
var OnProbeResult ProbeResultCallback

// RunPeriodicProbe runs periodic peer delay measurements using multicast
// Asks the local sptp to send PDelay_Req to ff02::6b as per IEEE 1588 specification.
// All SPTP clients in the rack that joined the multicast group will respond.
// If cfg.Count > 0, runs exactly that many probe cycles back-to-back without
// waiting for the interval between them, then returns. Otherwise, runs
// continuously using cfg.Interval (plus jitter) between cycles.
func RunPeriodicProbe(ctx context.Context, cfg ProbeConfig) error {
	// check once so periodic mode fails fast instead of looping on opaque failures
	if err := checkPingTimeout(cfg.Timeout); err != nil {
		return err
	}
	periodic := false
	if cfg.Count > 0 {
		log.Infof("[pdelay] starting multicast probe (count: %d)", cfg.Count)
	} else {
		log.Infof("[pdelay] starting periodic multicast probe (interval: %s, jitter: %s)", cfg.Interval, cfg.Jitter)
		periodic = true
	}
	log.Infof("[pdelay] asking sptp to send PDelay_Req to multicast address %s", cfg.multicastGroup())
	var nextProbe time.Duration

	var attempted, succeeded int
	for periodic || cfg.Count > 0 {
		// Calculate next probe time with jitter
		if periodic {
			jitter := CalculateJitter(cfg.Jitter)
			nextProbe = cfg.Interval + jitter
		} else {
			cfg.Count--
		}

		if err := ctx.Err(); err != nil {
			log.Infof("[pdelay] stopping probes")
			return err
		}
		if nextProbe > 0 {
			log.Debugf("[pdelay] waiting %s until next probe cycle", nextProbe)
			select {
			case <-ctx.Done():
				log.Infof("[pdelay] stopping probes")
				return ctx.Err()
			case <-time.After(nextProbe):
			}
		}

		attempted++
		results, err := runMulticastProbe(ctx, cfg)
		if err != nil {
			log.Warningf("[pdelay] multicast probe failed: %v", err)
			continue
		}
		succeeded++

		log.Infof("[pdelay] received %d responses from rack servers", len(results))

		// Call callback with all results from this cycle
		if OnProbeResult != nil {
			OnProbeResult(results)
		}
	}
	// exiting 0 with no rows would leave the ptp_pdelay dataset quiet and every
	// dashboard reading healthy
	if attempted > 0 && succeeded == 0 {
		return fmt.Errorf("all %d multicast probes failed", attempted)
	}
	return nil
}

// runMulticastProbe asks sptp to probe the rack and returns the measurements
func runMulticastProbe(ctx context.Context, cfg ProbeConfig) ([]*pdelay.Result, error) {
	address := checker.GetServerAddress(cfg.Server, checker.FlavourSPTP)
	results, err := pdelay.FetchPing(ctx, address, cfg.multicastGroup(), cfg.Timeout)
	if err != nil {
		return nil, fmt.Errorf("asking sptp at %s to probe %s: %w", address, cfg.multicastGroup(), err)
	}

	var addrW, offW int
	for _, result := range results {
		addrW = max(addrW, len(result.Responder.String()))
		offW = max(offW, len(result.Offset().String()))
	}
	for _, result := range results {
		// the scuba logger branches on Error, so an incomplete measurement must
		// carry one rather than be recorded as a real zero offset
		if result.Error == nil && !result.Valid() {
			result.Error = errors.New("incomplete response")
		}
		if result.Error != nil {
			fmt.Printf("%-*s %v\n", addrW, result.Responder, result.Error)
			continue
		}
		fmt.Printf("%-*s offset=%-*s path_delay=%s\n", addrW, result.Responder,
			offW, result.Offset(), result.PathDelay())
	}
	return results, nil
}

var pdelayCmd = &cobra.Command{
	Use:   "pdelay",
	Short: "Periodic in-rack peer delay measurement using multicast",
	Long: `Run periodic peer delay measurements against all hosts in the same rack.

This command asks the local sptp daemon to send PDelay_Req messages to the PTP
peer delay multicast address (ff02::6b for IPv6, 224.0.0.107 for IPv4) as per
IEEE 1588. All SPTP clients in the same L2 domain (rack) that have joined the
multicast group will receive the request and respond.

sptp sends the probe from the PTP event port it already owns, so NICs that can
only hardware timestamp packets on port 319 are supported.

The probe runs every 5 minutes (configurable) with random jitter to avoid
synchronized probing across the fleet. This implements in-rack linearizability
checks for detecting time offset asymmetries between servers.`,
	Args: cobra.NoArgs,
	Run: func(_ *cobra.Command, _ []string) {
		ConfigureVerbosity()

		cfg := ProbeConfig{
			Server:   rootClientFlag,
			Interval: pdelayIntervalf,
			Jitter:   pdelayJitterf,
			Timeout:  pdelayTimeoutf,
			IPv4:     pdelayIPv4f,
			Count:    pdelayCountf,
		}

		// Set up signal handling for graceful shutdown
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-sigChan
			log.Infof("[pdelay] received shutdown signal")
			cancel()
		}()

		if err := RunPeriodicProbe(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
	},
}
