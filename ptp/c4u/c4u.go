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

package c4u

import (
	"time"

	"github.com/facebook/time/ptp/c4u/clock"
	"github.com/facebook/time/ptp/c4u/utcoffset"
	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/ptp4u/server"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

type Config struct {
	Apply bool
	Path  string
	Pid   string
	TAU   int
}

type ringBuffer struct {
	data  []ptp.ClockQuality
	index int
	size  int
}

func NewRingBuffer(size int) *ringBuffer {
	return &ringBuffer{size: size, data: make([]ptp.ClockQuality, size)}
}

func (rb *ringBuffer) Write(c ptp.ClockQuality) {
	if rb.index >= rb.size {
		rb.index = 0
	}
	rb.data[rb.index] = c
	rb.index++
}

func (rb *ringBuffer) Data() []ptp.ClockQuality {
	return rb.data
}

var defaultConfig = &server.DynamicConfig{
	DrainInterval:  30 * time.Second,
	MaxSubDuration: 1 * time.Hour,
	MetricInterval: 1 * time.Minute,
	MinSubInterval: 1 * time.Second,
}

func Run(config *Config) {
	rb := NewRingBuffer(config.TAU)

	for it := time.NewTicker(time.Second); true; <-it.C {
		// Clock data
		c, err := clock.Run()
		if err != nil {
			log.Errorf("Failed to collect clock data: %v", err)
			continue
		}
		rb.Write(*c)
		w := clock.Worst(rb.Data())

		// UTC data
		u, err := utcoffset.Run()
		if err != nil {
			log.Errorf("Failed to collect UTC offset data: %v", err)
			continue
		}

		current, err := server.ReadDynamicConfig(config.Path)
		if err != nil {
			log.Errorf("Failed read current ptp4u config: %v. Using defaults", err)
			current = defaultConfig
		}
		pending := &server.DynamicConfig{}
		*pending = *current

		pending.ClockClass = w.ClockClass
		pending.ClockAccuracy = w.ClockAccuracy
		pending.UTCOffset = u

		if *current != *pending {
			log.Infof("Current: %+v", current)
			log.Infof("Pending: %+v", pending)

			if config.Apply {
				log.Infof("Saving a pending config to %s", config.Path)
				err := pending.Write(config.Path)
				if err != nil {
					log.Errorf("Failed save the ptp4u config: %v", err)
					continue
				}

				pid, err := server.ReadPidFile(config.Pid)
				if err != nil {
					log.Errorf("Failed to read ptp4u pid: %v", err)
					continue
				}

				err = unix.Kill(pid, unix.SIGHUP)
				if err != nil {
					log.Errorf("Failed to send SIGHUP to ptp4u %d: %v", pid, err)
					continue
				}
				log.Infof("SIGHUP is sent to ptp4u pid: %d", pid)
			}
		}
	}
}
