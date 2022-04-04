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

package server

import (
	"errors"
	"net"
	"os"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	yaml "gopkg.in/yaml.v2"
)

var errInsaneUTCoffset = errors.New("UTC offset is outside of sane range")

// StaticConfig is a set of static options which require a server restart
type StaticConfig struct {
	ConfigFile     string
	DebugAddr      string
	DSCP           int
	Interface      string
	IP             net.IP
	LogLevel       string
	MonitoringPort int
	QueueSize      int
	RecvWorkers    int
	SendWorkers    int
	TimestampType  string
}

// DynamicConfig is a set of dynamic options which don't need a server restart
type DynamicConfig struct {
	// ClockCccuracy to report via announce messages. Time Accurate within 100ns
	ClockAccuracy uint8
	// ClockClass to report via announce messages. 6 - Locked with Primary Reference Clock
	ClockClass uint8
	// DrainInterval is an interval for drain checks
	DrainInterval time.Duration
	// MaxSubDuration is a maximum sync/announce/delay_resp subscription duration
	MaxSubDuration time.Duration
	// MetricInterval is an interval of resetting metrics
	MetricInterval time.Duration
	// MinSubInterval is a minimum interval of the sync/announce subscription messages
	MinSubInterval time.Duration
	// UTCOffset is a current UTC offset.
	UTCOffset time.Duration
}

// Config is a server config structure
type Config struct {
	StaticConfig
	DynamicConfig

	clockIdentity ptp.ClockIdentity
}

// UTCOffsetSanity checks if UTC offset value has an adequate value
func (dc *DynamicConfig) UTCOffsetSanity() error {
	if dc.UTCOffset < 30*time.Second || dc.UTCOffset > 50*time.Second {
		return errInsaneUTCoffset
	}
	return nil
}

func (c *Config) ReadDynamicConfig() error {
	cData, err := os.ReadFile(c.ConfigFile)
	if err != nil {
		return err
	}

	d := c.DynamicConfig

	err = yaml.Unmarshal(cData, &d)
	if err != nil {
		return err
	}

	if err := d.UTCOffsetSanity(); err != nil {
		return err
	}

	c.DynamicConfig = d
	return nil
}

// IfaceHasIP checks if selected IP is on interface
func (c *Config) IfaceHasIP() (bool, error) {
	ips, err := ifaceIPs(c.Interface)
	if err != nil {
		return false, err
	}

	for _, ip := range ips {
		if c.IP.Equal(ip) {
			return true, nil
		}
	}

	return false, nil
}

// ifaceIPs gets all IPs on the specified interface
func ifaceIPs(iface string) ([]net.IP, error) {
	i, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, err
	}

	addrs, err := i.Addrs()
	if err != nil {
		return nil, err
	}

	res := []net.IP{}
	for _, addr := range addrs {
		ip := addr.(*net.IPNet).IP
		res = append(res, ip)
	}
	res = append(res, net.IPv6zero)
	res = append(res, net.IPv4zero)

	return res, nil
}
