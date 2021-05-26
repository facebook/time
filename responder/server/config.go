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
	"fmt"
	"net"
	"time"

	"github.com/facebookincubator/ntp/protocol/ntpshm"
	"github.com/facebookincubator/ptp/phc"
	ptp "github.com/facebookincubator/ptp/protocol"
)

// Config is a server config structure
type Config struct {
	Interface      string
	IP             net.IP
	LogLevel       string
	MaxSubDuration time.Duration
	MetricInterval time.Duration
	MinSubInterval time.Duration
	MonitoringPort int
	SHM            bool
	TimestampType  string
	UTCOffset      time.Duration
	Workers        int

	clockIdentity ptp.ClockIdentity
}

// SetUTCOffsetFromSHM reads SHM and if valid sets UTC offset
func (c *Config) SetUTCOffsetFromSHM() error {
	phcTime, err := phc.Time(c.Interface, phc.MethodSyscallClockGettime)
	if err != nil {
		return err
	}

	shmTime, err := ntpshm.Time()
	if err != nil {
		return err
	}

	// Safety check (http://leapsecond.com/java/gpsclock.htm):
	// SHM (NTP) time is always behind of the PHC (PTP) (as of 2021 by 37 seconds)
	// PHC (PTP) time is always ahead of the SHM (NTP)
	// SHM+30 <PHC< SHM+50
	uo := phcTime.Sub(shmTime).Round(time.Second)
	if uo.Seconds() < 30 || uo.Seconds() > 50 {
		return fmt.Errorf("shm (%s) and phc (%s) times are too far away: %s", shmTime, phcTime, uo)
	}

	c.UTCOffset = uo
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
