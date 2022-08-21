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

/*
Package server implements simple Unicast PTP UDP server.
*/
package server

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	"golang.org/x/sys/unix"
	yaml "gopkg.in/yaml.v2"
)

var errInsaneUTCoffset = errors.New("UTC offset is outside of sane range")

// dcMux is a dynamic config mutex
var dcMux = sync.Mutex{}

// StaticConfig is a set of static options which require a server restart
type StaticConfig struct {
	ConfigFile     string
	DebugAddr      string
	DSCP           int
	Interface      string
	IP             net.IP
	LogLevel       string
	MonitoringPort int
	PidFile        string
	QueueSize      int
	RecvWorkers    int
	SendWorkers    int
	TimestampType  string
}

// DynamicConfig is a set of dynamic options which don't need a server restart
type DynamicConfig struct {
	// ClockCccuracy to report via announce messages. Time Accurate within 100ns
	ClockAccuracy ptp.ClockAccuracy
	// ClockClass to report via announce messages. 6 - Locked with Primary Reference Clock
	ClockClass ptp.ClockClass
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
// As of Apr 2022 TAI UTC offset is 37 seconds
func (dc *DynamicConfig) UTCOffsetSanity() error {
	if dc.UTCOffset < 30*time.Second || dc.UTCOffset > 50*time.Second {
		return errInsaneUTCoffset
	}
	return nil
}

func ReadDynamicConfig(path string) (*DynamicConfig, error) {
	dc := &DynamicConfig{}
	cData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(cData, &dc)
	if err != nil {
		return nil, err
	}

	if err := dc.UTCOffsetSanity(); err != nil {
		return nil, err
	}

	return dc, nil
}

func (dc *DynamicConfig) Write(path string) error {
	d, err := yaml.Marshal(&dc)
	if err != nil {
		return err
	}

	return os.WriteFile(path, d, 0644)
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

// CreatePidFile creates a pid file in a defined location
func (c *Config) CreatePidFile() error {
	return os.WriteFile(c.PidFile, []byte(fmt.Sprintf("%d\n", unix.Getpid())), 0644)
}

// DeletePidFile deletes a pid file from a defined location
func (c *Config) DeletePidFile() error {
	return os.Remove(c.PidFile)
}

// ReadPidFile read a pid file from a path location and returns a pid
func ReadPidFile(path string) (int, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(strings.Replace(string(content), "\n", "", -1))
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
