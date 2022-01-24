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

package config

import (
	"fmt"
	"net"

	"github.com/facebook/time/calnex/api"
	"github.com/go-ini/ini"
	log "github.com/sirupsen/logrus"
)

// CalnexConfig is a wrapper around map[channel]MeasureConfig
type CalnexConfig map[api.Channel]MeasureConfig

// MeasureConfig is a Calnex channel config
type MeasureConfig struct {
	Target string    `json:"target"`
	Probe  api.Probe `json:"probe"`
}

// NetworkConfig represents network config of a Calnex device
type NetworkConfig struct {
	Eth1 net.IP
	Gw1  net.IP
	Eth2 net.IP
	Gw2  net.IP
}

type config struct {
	changed bool
}

// chSet modifies a config on several channels
func (c *config) chSet(s *ini.Section, start, end api.Channel, keyf, value string) {
	for i := start; i <= end; i++ {
		name := fmt.Sprintf(keyf, i.CalnexAPI())
		c.set(s, name, value)
	}
}

// set modifies a single config value
func (c *config) set(s *ini.Section, name, value string) {
	k := s.Key(name)
	if k.Value() != value {
		k.SetValue(value)
		log.Infof("setting %s to %s", name, value)
		c.changed = true
	}
}

func (c *config) measureConfig(s *ini.Section, cc CalnexConfig) {
	channelEnabled := make(map[api.Channel]bool)

	for ch, m := range cc {
		channelEnabled[ch] = true

		probe := fmt.Sprintf("%s\\ptp_synce\\mode\\probe_type", ch.CalnexAPI())
		c.set(s, probe, m.Probe.CalnexName())

		switch m.Probe {
		case api.ProbeNTP:
			server := fmt.Sprintf("%s\\ptp_synce\\ntp\\server_ip", ch.CalnexAPI())
			c.set(s, server, m.Target)

			serverv6 := fmt.Sprintf("%s\\ptp_synce\\ntp\\server_ip_ipv6", ch.CalnexAPI())
			c.set(s, serverv6, m.Target)
		case api.ProbePTP:
			server := fmt.Sprintf("%s\\ptp_synce\\ptp\\master_ip", ch.CalnexAPI())
			c.set(s, server, m.Target)

			serverv6 := fmt.Sprintf("%s\\ptp_synce\\ptp\\master_ip_ipv6", ch.CalnexAPI())
			c.set(s, serverv6, m.Target)
		}
	}

	// Disable unused channels and enable used
	for ch := range api.ChannelCalnexToString {
		used := api.NO
		enabled := api.OFF
		if channelEnabled[ch] {
			// enable PTP/NTP channels
			used = api.YES
			enabled = api.ON
		}
		c.set(s, fmt.Sprintf("%s\\used", ch.CalnexAPI()), used)
		c.set(s, fmt.Sprintf("%s\\protocol_enabled", ch.CalnexAPI()), enabled)
	}
}

func (c *config) nicConfig(s *ini.Section, n *NetworkConfig) {
	c.set(s, fmt.Sprintf("%s\\ptp_synce\\ethernet\\gateway", api.ChannelONE.CalnexAPI()), n.Gw1.String())
	c.set(s, fmt.Sprintf("%s\\ptp_synce\\ethernet\\gateway_ipv6", api.ChannelONE.CalnexAPI()), n.Gw1.String())
	c.set(s, fmt.Sprintf("%s\\ptp_synce\\ethernet\\ip_address", api.ChannelONE.CalnexAPI()), n.Eth1.String())
	c.set(s, fmt.Sprintf("%s\\ptp_synce\\ethernet\\ip_address_ipv6", api.ChannelONE.CalnexAPI()), n.Eth1.String())
	c.set(s, fmt.Sprintf("%s\\ptp_synce\\ethernet\\mask", api.ChannelONE.CalnexAPI()), "64")
	c.set(s, fmt.Sprintf("%s\\ptp_synce\\ethernet\\gateway", api.ChannelTWO.CalnexAPI()), n.Gw2.String())
	c.set(s, fmt.Sprintf("%s\\ptp_synce\\ethernet\\gateway_ipv6", api.ChannelTWO.CalnexAPI()), n.Gw2.String())
	c.set(s, fmt.Sprintf("%s\\ptp_synce\\ethernet\\ip_address", api.ChannelTWO.CalnexAPI()), n.Eth2.String())
	c.set(s, fmt.Sprintf("%s\\ptp_synce\\ethernet\\ip_address_ipv6", api.ChannelTWO.CalnexAPI()), n.Eth2.String())
	c.set(s, fmt.Sprintf("%s\\ptp_synce\\ethernet\\mask", api.ChannelTWO.CalnexAPI()), "64")
}

func (c *config) baseConfig(s *ini.Section) {
	// disable synce
	c.chSet(s, api.ChannelONE, api.ChannelTWO, "%s\\synce_enabled", api.OFF)

	// DHCP off (not working properly anyway)
	c.chSet(s, api.ChannelONE, api.ChannelTWO, "%s\\ptp_synce\\ethernet\\dhcp", api.OFF)

	// show raw metrics
	c.chSet(s, api.ChannelONE, api.ChannelTWO, "%s\\ptp_synce\\ntp\\normalize_delays", api.OFF)

	// use ipv6
	c.chSet(s, api.ChannelONE, api.ChannelTWO, "%s\\ptp_synce\\ntp\\protocol_level", "UDP/IPv6")
	c.chSet(s, api.ChannelONE, api.ChannelTWO, "%s\\ptp_synce\\ptp\\protocol_level", "UDP/IPv6")

	// ntp 1 packet per 64 second
	c.chSet(s, api.ChannelONE, api.ChannelTWO, "%s\\ptp_synce\\ntp\\poll_log_interval", "1 packet/16 s")

	// ptp 1 packet per 1 second
	c.chSet(s, api.ChannelONE, api.ChannelTWO, "%s\\ptp_synce\\ptp\\log_announce_int", "1 packet/16 s")
	c.chSet(s, api.ChannelONE, api.ChannelTWO, "%s\\ptp_synce\\ptp\\log_delay_req_int", "1 packet/16 s")
	c.chSet(s, api.ChannelONE, api.ChannelTWO, "%s\\ptp_synce\\ptp\\log_sync_int", "1 packet/16 s")

	// ptp unicast mode
	c.chSet(s, api.ChannelONE, api.ChannelTWO, "%s\\ptp_synce\\ptp\\stack_mode", "Unicast")

	// ptp domain
	c.chSet(s, api.ChannelONE, api.ChannelTWO, "%s\\ptp_synce\\ptp\\domain", "0")

	// ptp dscp
	c.chSet(s, api.ChannelONE, api.ChannelTWO, "%s\\ptp_synce\\ptp\\dscp", "0")

	// continuous measurement
	c.set(s, "continuous", api.ON)

	// 25h measurement
	c.set(s, "meas_time", "1 days 1 hours")

	// tie_mode=TIE + 1 PPS TE
	c.set(s, "tie_mode", "TIE + 1 PPS TE")
}

// Config configures target Calnex with Network/Calnex configs if apply is specified
func Config(target string, insecureTLS bool, n *NetworkConfig, cc CalnexConfig, apply bool) error {
	var c config
	api := api.NewAPI(target, insecureTLS)

	f, err := api.FetchSettings()
	if err != nil {
		return err
	}

	s := f.Section("measure")

	// set static config
	c.baseConfig(s)

	// set IP/Gateway/Mask
	c.nicConfig(s, n)

	// set measure config
	c.measureConfig(s, cc)

	if !apply {
		log.Infof("dry run. Exiting")
		return nil
	}

	// check measurement status
	status, err := api.FetchStatus()
	if err != nil {
		return err
	}

	if c.changed {
		if status.MeasurementActive {
			log.Infof("stopping measurement")
			// stop measurement
			if err = api.StopMeasure(); err != nil {
				return err
			}
		}

		log.Infof("pushing the config")
		// set the modified config
		if err = api.PushSettings(f); err != nil {
			return err
		}
	} else {
		log.Infof("no change needs to be applied")
	}

	if c.changed || !status.MeasurementActive {
		log.Infof("starting measurement")
		// start measurement
		if err = api.StartMeasure(); err != nil {
			return err
		}
	}

	return nil
}
