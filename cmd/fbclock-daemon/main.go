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
	"io"
	"os"
	"time"

	"github.com/facebook/time/fbclock/daemon"
	log "github.com/sirupsen/logrus"
)

func main() {
	var (
		cfg            = &daemon.Config{}
		err            error
		cfgPath        string
		manageDevice   bool
		csvLog         bool
		csvPath        string
		verbose        bool
		monitoringPort int
	)

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "fbclock daemon\n")
		fmt.Fprintf(flag.CommandLine.Output(), "%s\n\nFlags:\n", daemon.MathHelp)
		flag.PrintDefaults()
	}

	flag.StringVar(&cfg.Device, "ptpdevice", "/dev/ptp0", "Path to original PTP device we need to copy if -manage is true")
	flag.StringVar(&cfg.PTP4Lsock, "ptp4lsock", "/var/run/ptp4l", "Path to ptp4l unix socket")
	flag.IntVar(&monitoringPort, "monitoringport", 21039, "Port to run monitoring server on")
	flag.IntVar(&cfg.RingSize, "buffer", 100, "Size of ring buffers, must be at least size of largets num of samples used in M and W formulas")
	flag.StringVar(&cfg.Math.M, "m", "mean(clockaccuracy, 100) + abs(mean(offset, 100)) + 1.0 * stddev(offset, 100)", "Math expression for M")
	flag.StringVar(&cfg.Math.W, "w", "mean(m, 100) + 4.0 * stddev(m, 100)", "Math expression for W")
	flag.StringVar(&cfg.Math.Drift, "drift", "mean(freqchangeabs, 99)", "Math expression for Drift PPB")
	flag.DurationVar(&cfg.Interval, "i", time.Second, "Interval at which we talk to ptp4l and update data in shm")
	flag.DurationVar(&cfg.LinearizabilityTestInterval, "I", time.Minute, "Interval at which we run linearizability tests. 0 means disabled.")

	flag.StringVar(&cfgPath, "cfg", "", "Path to config")
	flag.BoolVar(&manageDevice, "manage", true, "Manage devices")
	flag.BoolVar(&csvLog, "csvlog", true, "Log all the metrics as CSV to log")
	flag.StringVar(&csvPath, "csvpath", "", "write CSV log into this file")
	flag.BoolVar(&verbose, "verbose", false, "Verbose logging")

	flag.Parse()

	log.SetReportCaller(true)
	if verbose {
		log.SetLevel(log.DebugLevel)
	}
	if csvPath != "" && !csvLog {
		log.Fatalf("'csvpath' flag requires 'csvlog' flag")
	}
	if cfgPath != "" {
		log.Warningf("using config from %s, flag values are ignored", cfgPath)
		cfg, err = daemon.ReadConfig(cfgPath)
		if err != nil {
			log.Fatal(err)
		}
	}
	if err := cfg.EvalAndValidate(); err != nil {
		log.Fatal(err)
	}
	if manageDevice {
		if err := daemon.SetupDeviceDir(cfg.Device); err != nil {
			log.Fatal(err)
		}
	}
	log.Debugf("Config: %+v", *cfg)

	// set up sample logging
	w := log.StandardLogger().Writer()
	defer w.Close()
	var l daemon.Logger = daemon.NewDummyLogger(w)
	if csvLog {
		csvW := io.Writer(w)
		// set up logging of CSV samples to file
		if csvPath != "" {
			f, err := os.Create(csvPath)
			if err != nil {
				log.Fatal(err)
			}
			defer f.Close()
			// write both to stderr and file
			csvW = io.MultiWriter(w, f)
		}
		l = daemon.NewCSVLogger(csvW)
	}
	stats := daemon.NewJSONStats()
	go stats.Start(monitoringPort)
	s, err := daemon.New(cfg, stats, l)
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
