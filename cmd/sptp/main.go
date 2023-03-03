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
	"fmt"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/facebook/time/ptp/sptp/client"

	_ "net/http/pprof"
)

func updateSysStats(sysstats *client.SysStats, statsserver client.StatsServer, interval time.Duration) {
	stats, err := sysstats.CollectRuntimeStats(interval)
	if err != nil {
		log.Warningf("failed to get system metrics %v", err)
	}

	for k, v := range stats {
		statsserver.SetCounter(fmt.Sprintf("sptp.%s", k), int64(v))
	}
}

func updateSysStatsForever(sysstats *client.SysStats, statsserver client.StatsServer, interval time.Duration) {
	// update stats on goroutine start
	updateSysStats(sysstats, statsserver, interval)
	for range time.Tick(interval) {
		// update stats on every tick
		updateSysStats(sysstats, statsserver, interval)
	}
}

func doWork(cfg *client.Config) error {
	stats := client.NewJSONStats()
	sysstats := &client.SysStats{}
	go updateSysStatsForever(sysstats, stats, cfg.MetricsAggregationWindow)
	go stats.Start(cfg.MonitoringPort)
	p, err := client.NewSPTP(cfg, stats)
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

	flag.BoolVar(&verboseFlag, "verbose", false, "verbose output")
	flag.StringVar(&ifaceFlag, "iface", "eth0", "network interface to use")
	flag.StringVar(&configFlag, "config", "", "path to the config")
	flag.IntVar(&monitoringPortFlag, "monitoringport", 4269, "port to start monitoring http server on")
	flag.IntVar(&dscpFlag, "dscp", 0, "DSCP for PTP packets, valid values are between 0-63 (used by send workers)")
	flag.DurationVar(&intervalFlag, "interval", time.Second, "how often to send DelayReq to each GM")
	flag.StringVar(&pprofFlag, "pprof", "", "Address to have the profiler listen on, disabled if empty.")

	flag.Parse()

	log.SetLevel(log.InfoLevel)
	if verboseFlag {
		log.SetLevel(log.DebugLevel)
	}
	cfg, err := client.PrepareConfig(configFlag, flag.Args(), ifaceFlag, monitoringPortFlag, intervalFlag, dscpFlag)
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
