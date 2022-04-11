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
)

func main() {
	var (
		once     bool
		interval time.Duration
		sample   int
	)
	c := &c4u.Config{}

	flag.BoolVar(&c.Apply, "apply", false, "Save the ptp4u config to the path and send the SIGHUP to ptp4u")
	flag.BoolVar(&once, "once", false, "Run once and exit")
	flag.StringVar(&c.Path, "path", "/etc/ptp4u.yaml", "Path to a config file")
	flag.StringVar(&c.Pid, "ptp4u", "/var/run/ptp4u.pid", "Path to a ptp4u pid file")
	flag.IntVar(&sample, "sample", 600, "Sliding window size (samples) for clock data calculations")
	flag.DurationVar(&interval, "interval", time.Second, "Data cata collection interval")
	flag.Parse()

	if once {
		sample = 1
	}

	rb := clock.NewRingBuffer(sample)
	c4u.Run(c, rb)

	for it := time.NewTicker(interval); !once; <-it.C {
		c4u.Run(c, rb)
	}
}
