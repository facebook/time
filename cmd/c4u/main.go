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

	"github.com/facebook/time/ptp/c4u"
)

func main() {
	c := &c4u.Config{}

	flag.BoolVar(&c.Save, "save", false, "Save config to the path instead of reading it")
	flag.StringVar(&c.Path, "path", "/etc/ptp4u.yaml", "Path to a config file")
	flag.StringVar(&c.Pid, "ptp4u", "/var/run/ptp4u.pid", "Path to a ptp4u pid file")
	flag.IntVar(&c.TAU, "tau", 60, "Sliding window size (seconds) for clock data calculations")
	flag.Parse()

	c4u.Run(c)
}
