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
	"flag"
	"net/http"
	"time"

	_ "net/http/pprof"

	"github.com/facebook/time/ptp/sptp/stats"
	log "github.com/sirupsen/logrus"
)

func main() {
	var (
		verboseFlag            bool
		exporterPortFlag       int
		sptpMonitoringPortFlag int
		intervalFlag           time.Duration
		pprofFlag              string
	)

	flag.BoolVar(&verboseFlag, "verbose", false, "verbose output")
	flag.IntVar(&sptpMonitoringPortFlag, "monitoringport", 4269, "port sptp metrics http server is listening on")
	flag.IntVar(&exporterPortFlag, "exporterport", 6942, "port prometheus metrics exporter is listening on")

	flag.DurationVar(&intervalFlag, "interval", time.Second, "how often to fetch metrics from  sptp")
	flag.StringVar(&pprofFlag, "pprof", "", "Address to have the profiler listen on, disabled if empty.")

	flag.Parse()

	log.SetLevel(log.InfoLevel)
	if verboseFlag {
		log.SetLevel(log.DebugLevel)
	}
	if pprofFlag != "" {
		go func() {
			err := http.ListenAndServe(pprofFlag, nil)
			if err != nil {
				log.Errorf("Failed to start pprof. Err: %v", err)
			}
		}()
	}
	exporter := stats.NewPrometheusExporter(exporterPortFlag, sptpMonitoringPortFlag, intervalFlag)
	exporter.Start()
}
