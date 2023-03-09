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

	yaml "gopkg.in/yaml.v2"
)

// Config represents configuration we expect to read from file
type Config struct {
	PTPClientAddress            string        // where should fbclock connect to
	RingSize                    int           // must be at least the size of N samples we use in expressions
	Math                        Math          // configuration for calculation we'll be doing
	Interval                    time.Duration // how often do we poll ptp4l and update data in shm
	Iface                       string        // network interface to use
	LinearizabilityTestInterval time.Duration // perform the linearizability test every so often
	SPTP                        bool          // wherever we run in sptp or ptp4l mode
}

// EvalAndValidate makes sure config is valid and evaluates expressions for further use.
func (c *Config) EvalAndValidate() error {
	if c.PTPClientAddress == "" {
		return fmt.Errorf("bad config: 'ptpclientaddress'")
	}
	if c.RingSize <= 0 {
		return fmt.Errorf("bad config: 'ringsize' must be >0")
	}
	if c.Interval > time.Minute {
		return fmt.Errorf("bad config: 'interval' is over a minute")
	}
	if err := c.Math.Prepare(); err != nil {
		return err
	}
	return nil
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
