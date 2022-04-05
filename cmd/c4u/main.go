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
	"os"
	"time"

	"github.com/facebook/time/ptp/c4u/clock"
	"github.com/facebook/time/ptp/c4u/utcoffset"
	"github.com/facebook/time/ptp/ptp4u/server"
	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

var (
	save bool
	path string
)

func main() {
	flag.BoolVar(&save, "save", false, "Save config to the path instead of reading it")
	flag.StringVar(&path, "path", "/etc/ptp4u.yaml", "Path to a config file")
	flag.Parse()

	current := &server.DynamicConfig{}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}

	err = yaml.Unmarshal(data, &current)
	if err != nil {
		log.Fatal(err)
	}

	// Generate
	config := &server.DynamicConfig{
		DrainInterval:  30 * time.Second,
		MaxSubDuration: 1 * time.Hour,
		MetricInterval: 1 * time.Minute,
		MinSubInterval: 1 * time.Second,
	}

	c, err := clock.Run()
	if err != nil {
		log.Fatal(err)
	}
	config.ClockClass = c.ClockClass
	config.ClockAccuracy = c.ClockAccuracy

	u, err := utcoffset.Run()
	if err != nil {
		log.Fatal(err)
	}
	config.UTCOffset = u

	if save {
		d, err := yaml.Marshal(&config)
		if err != nil {
			log.Fatal(err)
		}

		err = os.WriteFile(path, d, 0644)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		log.Printf("Current: %+v", current)
		log.Printf("Pending: %+v", config)
	}
}
