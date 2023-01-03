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
	"time"

	"github.com/facebook/time/ptp/c4u"
	"github.com/facebook/time/ptp/c4u/clock"
	"github.com/facebook/time/ptp/c4u/stats"
	ptp "github.com/facebook/time/ptp/protocol"
	log "github.com/sirupsen/logrus"
)

func main() {
	var (
		calibratingBaseLine time.Duration
		holdoverBaseLine    time.Duration
		interval            time.Duration
		lockBaseLine        time.Duration
		logLevel            string
		monitoringPort      int
		once                bool
		sample              int
	)
	c := &c4u.Config{}

	flag.BoolVar(&c.Apply, "apply", false, "Save the ptp4u config to the path and send the SIGHUP to ptp4u")
	flag.BoolVar(&once, "once", false, "Run once and exit")
	flag.StringVar(&c.Path, "path", "/etc/ptp4u.yaml", "Path to a config file")
	flag.StringVar(&c.Pid, "ptp4u", "/var/run/ptp4u.pid", "Path to a ptp4u pid file")
	flag.StringVar(&c.AccuracyExpr, "accuracyExpr", "abs(mean(phcoffset)) + 3 * stddev(phcoffset) + abs(mean(oscillatoroffset)) + 3 * stddev(oscillatoroffset)", "Math to calculate clock accuracy")
	flag.StringVar(&c.ClassExpr, "classExpr", "p99(oscillatorclass)", "Math to calculate clock class")
	flag.IntVar(&sample, "sample", 600, "Sliding window size (samples) for clock data calculations")
	flag.DurationVar(&interval, "interval", time.Second, "Data cata collection interval")
	flag.StringVar(&logLevel, "loglevel", "info", "Set a log level. Can be: debug, info, warning, error")
	flag.IntVar(&monitoringPort, "monitoringport", 8889, "Port to run monitoring server on")
	flag.DurationVar(&lockBaseLine, "lockBaseLine", 250*time.Nanosecond, "Minimum value for ClockClass in LOCK state")
	flag.DurationVar(&holdoverBaseLine, "holdoverBaseLine", time.Microsecond, "Minimum value for ClockClass in HOLDOVER state")
	flag.DurationVar(&calibratingBaseLine, "calibratingBaseLine", 250*time.Nanosecond, "Minimum value for ClockClass in CALIBRATING state")
	flag.Parse()

	switch logLevel {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warning":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	default:
		log.Fatalf("Unrecognized log level: %v", logLevel)
	}

	c.LockBaseLine = ptp.ClockAccuracyFromOffset(lockBaseLine)
	c.HoldoverBaseLine = ptp.ClockAccuracyFromOffset(holdoverBaseLine)
	c.CalibratingBaseLine = ptp.ClockAccuracyFromOffset(calibratingBaseLine)

	if once {
		sample = 1
	}

	st := stats.NewJSONStats()
	go st.Start(monitoringPort)

	rb := clock.NewRingBuffer(sample)
	if err := c4u.Run(c, rb, st); err != nil {
		log.Fatal(err)
	}

	for it := time.NewTicker(interval); !once; <-it.C {
		if err := c4u.Run(c, rb, st); err != nil {
			log.Fatal(err)
		}
	}
}
