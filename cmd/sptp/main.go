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

package main

import (
	"context"
	"flag"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/facebook/time/ptp/sptp/client"

	_ "net/http/pprof"
)

func doWork(cfg *client.Config) error {
	stats, err := client.NewJSONStats()
	if err != nil {
		return err
	}
	go stats.Start(cfg.MonitoringPort, cfg.MetricsAggregationWindow)
	p, err := client.NewSPTP(cfg, *stats)
	if err != nil {
		return err
	}
	ctx := context.Background()
	return p.Run(ctx)
}

func main() {
	var (
		verboseFlag        bool
		ifaceFlag          string
		monitoringPortFlag int
		intervalFlag       time.Duration
		dscpFlag           int
		configFlag         string
		pprofFlag          string
	)
	defaults := client.DefaultConfig()

	flag.BoolVar(&verboseFlag, "verbose", false, "verbose output")
	flag.StringVar(&ifaceFlag, "iface", defaults.Iface, "network interface to use")
	flag.StringVar(&configFlag, "config", "", "path to the config")
	flag.IntVar(&monitoringPortFlag, "monitoringport", defaults.MonitoringPort, "port to start monitoring http server on")
	flag.IntVar(&dscpFlag, "dscp", defaults.DSCP, "DSCP for PTP packets, valid values are between 0-63 (used by send workers)")
	flag.DurationVar(&intervalFlag, "interval", defaults.Interval, "how often to send DelayReq to each GM")
	flag.StringVar(&pprofFlag, "pprof", "", "Address to have the profiler listen on, disabled if empty.")

	flag.Parse()
	setFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		setFlags[f.Name] = true
	})

	log.SetLevel(log.InfoLevel)
	if verboseFlag {
		log.SetLevel(log.DebugLevel)
	}
	cfg, err := client.PrepareConfig(configFlag, flag.Args(), ifaceFlag, monitoringPortFlag, intervalFlag, dscpFlag, setFlags)
	if err != nil {
		log.Fatal(err)
	}
	if pprofFlag != "" {
		go func() {
			err = http.ListenAndServe(pprofFlag, nil)
			if err != nil {
				log.Errorf("Failed to start pprof. Err: %v", err)
			}
		}()
	}
	if err := doWork(cfg); err != nil {
		log.Fatal(err)
	}
}
