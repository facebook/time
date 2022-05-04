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
		probe := ""

		switch m.Probe {
		case api.ProbeNTP:
			probe = fmt.Sprintf("%s\\ptp_synce\\mode\\probe_type", ch.CalnexAPI())

			c.set(s, fmt.Sprintf("%s\\protocol_enabled", ch.CalnexAPI()), api.ON)
			// Set Virtual Port to use Physical channel 1
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\physical_packet_channel", ch.CalnexAPI()), "Channel 1")

			// Set target we measure
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ntp\\server_ip", ch.CalnexAPI()), m.Target)
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ntp\\server_ip_ipv6", ch.CalnexAPI()), m.Target)

			// show raw metrics
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ntp\\normalize_delays", ch.CalnexAPI()), api.OFF)
			// use ipv6
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ntp\\protocol_level", ch.CalnexAPI()), "UDP/IPv6")
			// ntp 1 packet per 64 second
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ntp\\poll_log_interval", ch.CalnexAPI()), "1 packet/16 s")
		case api.ProbePTP:
			probe = fmt.Sprintf("%s\\ptp_synce\\mode\\probe_type", ch.CalnexAPI())

			c.set(s, fmt.Sprintf("%s\\protocol_enabled", ch.CalnexAPI()), api.ON)
			// Set Virtual Port to use Physical channel 1
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\physical_packet_channel", ch.CalnexAPI()), "Channel 1")

			// Set target we measure
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ptp\\master_ip", ch.CalnexAPI()), m.Target)
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ptp\\master_ip_ipv6", ch.CalnexAPI()), m.Target)

			// use ipv6
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ptp\\protocol_level", ch.CalnexAPI()), "UDP/IPv6")

			// ptp 1 packet per 1 second
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ptp\\log_announce_int", ch.CalnexAPI()), "1 packet/16 s")
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ptp\\log_delay_req_int", ch.CalnexAPI()), "1 packet/16 s")
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ptp\\log_sync_int", ch.CalnexAPI()), "1 packet/16 s")

			// ptp unicast mode
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ptp\\stack_mode", ch.CalnexAPI()), "Unicast")

			// ptp domain
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ptp\\domain", ch.CalnexAPI()), "0")
		case api.ProbePPS:
			probe = fmt.Sprintf("%s\\signal_type", ch.CalnexAPI())

			c.set(s, fmt.Sprintf("%s\\server_ip", ch.CalnexAPI()), m.Target)
			c.set(s, fmt.Sprintf("%s\\trig_level", ch.CalnexAPI()), "500 mV")
			c.set(s, fmt.Sprintf("%s\\freq", ch.CalnexAPI()), "1 Hz")
			c.set(s, fmt.Sprintf("%s\\suppress_steps", ch.CalnexAPI()), api.YES)
		}

		// enable PTP/NTP channels
		c.set(s, fmt.Sprintf("%s\\used", ch.CalnexAPI()), api.YES)
		c.set(s, probe, m.Probe.CalnexName())
	}

	// Disable unused channels and enable used
	for ch, datatype := range api.MeasureChannelDatatypeMap {
		if !channelEnabled[ch] {
			c.set(s, fmt.Sprintf("%s\\used", ch.CalnexAPI()), api.NO)
			if datatype == api.TWOWAYTE {
				c.set(s, fmt.Sprintf("%s\\protocol_enabled", ch.CalnexAPI()), api.OFF)
				c.set(s, fmt.Sprintf("%s\\ptp_synce\\mode\\probe_type", ch.CalnexAPI()), api.DISABLED)
			}
		}
	}
}

func (c *config) baseConfig(s *ini.Section) {
	// continuous measurement
	c.set(s, "continuous", api.ON)

	// 25h measurement
	c.set(s, "meas_time", "1 days 1 hours")

	// tie_mode=TIE + 1 PPS TE
	c.set(s, "tie_mode", "TIE + 1 PPS TE")

	// ch8 is a ref channel. Always ON
	c.set(s, "ch8\\used", api.YES)

	// disable synce
	c.chSet(s, api.ChannelONE, api.ChannelTWO, "%s\\synce_enabled", api.OFF)

	// ptp dscp
	c.chSet(s, api.ChannelONE, api.ChannelTWO, "%s\\ptp_synce\\ptp\\dscp", "0")

	// DHCP
	c.chSet(s, api.ChannelONE, api.ChannelTWO, "%s\\ptp_synce\\ethernet\\dhcp_v6", api.DHCP)
	c.chSet(s, api.ChannelONE, api.ChannelTWO, "%s\\ptp_synce\\ethernet\\dhcp_v4", api.DISABLED)

	// Disable 2nd Physical channel
	c.chSet(s, api.ChannelONE, api.ChannelTWO, "%s\\used", api.NO)
	c.chSet(s, api.ChannelONE, api.ChannelTWO, "%s\\protocol_enabled", api.OFF)
}

// Config configures target Calnex with Network/Calnex configs if apply is specified
func Config(target string, insecureTLS bool, cc CalnexConfig, apply bool) error {
	var c config
	api := api.NewAPI(target, insecureTLS)

	f, err := api.FetchSettings()
	if err != nil {
		return err
	}

	s := f.Section("measure")

	// set static config
	c.baseConfig(s)

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
