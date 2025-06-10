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

package daemon

import (
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	yaml "gopkg.in/yaml.v2"
)

// Config represents configuration we expect to read from file
type Config struct {
	PTPClientAddress               string        // where should fbclock connect to
	RingSize                       int           // must be at least the size of N samples we use in expressions
	Math                           Math          // configuration for calculation we'll be doing
	Interval                       time.Duration // how often do we poll ptp4l and update data in shm
	Iface                          string        // network interface to use
	LinearizabilityTestInterval    time.Duration // perform the linearizability test every so often
	SPTP                           bool          // denotes whether we are running in sptp or ptp4l mode
	LinearizabilityTestMaxGMOffset time.Duration // max offset between GMs before linearizability test considered failed
	BootDelay                      time.Duration // postpone startup by this time after boot
	EnableDataV2                   bool          // enable fbclock data v2
}

// EvalAndValidate makes sure config is valid and evaluates expressions for further use.
func (c *Config) EvalAndValidate() error {
	if c.PTPClientAddress == "" {
		return fmt.Errorf("bad config: 'ptpclientaddress'")
	}
	if c.RingSize <= 0 {
		return fmt.Errorf("bad config: 'ringsize' must be >0")
	}
	if c.Interval <= 0 || c.Interval > time.Minute {
		return fmt.Errorf("bad config: 'interval' must be between 0 and 1 minute")
	}

	if c.LinearizabilityTestInterval < 0 {
		return fmt.Errorf("bad config: 'test interval' must be positive")
	}

	if c.LinearizabilityTestMaxGMOffset < 0 {
		return fmt.Errorf("bad config: 'offset' must be positive")
	}
	return c.Math.Prepare()
}

// PostponeStart postpones startup by BootDelay
func (c *Config) PostponeStart() error {
	uptime, err := uptime()
	if err != nil {
		return err
	}
	log.Debugf("system uptime: %s", uptime)

	if uptime < c.BootDelay {
		log.Infof("postponing startup by %s", c.BootDelay-uptime)
		time.Sleep(c.BootDelay - uptime)
	}

	return nil
}

// uptime returns system boot time
func uptime() (time.Duration, error) {
	var ts unix.Timespec

	if err := unix.ClockGettime(unix.CLOCK_BOOTTIME, &ts); err != nil {
		return 0, err
	}

	return time.Duration(ts.Nano()), nil
}

// ReadConfig reads config and unmarshals it from yaml into Config
func ReadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := Config{}
	err = yaml.UnmarshalStrict(data, &c)
	return &c, err
}
