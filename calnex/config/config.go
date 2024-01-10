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
	"os"
	"time"

	"github.com/facebook/time/calnex/api"
	"github.com/go-ini/ini"
	log "github.com/sirupsen/logrus"
)

// Calnexes is a map of devices to CalnexConfig
type Calnexes map[string]*CalnexConfig

// CalnexConfig is a config representation of calnex
type CalnexConfig struct {
	Measure        map[api.Channel]MeasureConfig `json:"measure"`
	AntennaDelayNS int                           `json:"antennaDelayNS"`
}

// MeasureConfig is a Calnex channel config
type MeasureConfig struct {
	Target string    `json:"target"`
	Probe  api.Probe `json:"probe"`
	Name   string    `json:"name"`
}

type config struct {
	changed bool
}

// chSet modifies a config on several channels
func (c *config) chSet(s *ini.Section, start, end api.Channel, keyf, value string) {
	for i := start.Calnex(); i <= end.Calnex(); i++ {
		ch, _ := api.ChannelFromInt(i)
		name := fmt.Sprintf(keyf, ch.CalnexAPI())
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

func (c *config) measureConfig(s *ini.Section, mc map[api.Channel]MeasureConfig) {
	channelEnabled := make(map[api.Channel]bool)

	for ch, m := range mc {
		channelEnabled[ch] = true
		probe := ""

		switch m.Probe {
		case api.ProbeNTP:
			probe = fmt.Sprintf("%s\\ptp_synce\\mode\\probe_type", ch.CalnexAPI())

			c.set(s, fmt.Sprintf("%s\\protocol_enabled", ch.CalnexAPI()), api.ON)
			// Set Virtual Port to use Physical channel 1
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\physical_packet_channel", ch.CalnexAPI()), api.CHANNEL1)

			// Set target we measure
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ntp\\server_ip", ch.CalnexAPI()), m.Target)
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ntp\\server_ip_ipv6", ch.CalnexAPI()), m.Target)

			// show raw metrics
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ntp\\normalize_delays", ch.CalnexAPI()), api.OFF)
			// use ipv6
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ntp\\protocol_level", ch.CalnexAPI()), api.IPV6)
			// ntp 1 packet per 16 seconds
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ntp\\poll_log_interval", ch.CalnexAPI()), api.INTERVAL)
		case api.ProbePTP:
			probe = fmt.Sprintf("%s\\ptp_synce\\mode\\probe_type", ch.CalnexAPI())

			c.set(s, fmt.Sprintf("%s\\protocol_enabled", ch.CalnexAPI()), api.ON)
			// Set Virtual Port to use Physical channel 1
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\physical_packet_channel", ch.CalnexAPI()), api.CHANNEL1)

			// Use SPTP instead of PTPv2
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ptp\\version", ch.CalnexAPI()), api.SPTP)

			// Set target we measure
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ptp\\master_ip", ch.CalnexAPI()), m.Target)
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ptp\\master_ip_ipv6", ch.CalnexAPI()), m.Target)

			// use ipv6
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ptp\\protocol_level", ch.CalnexAPI()), api.IPV6)

			// ptp 1 packet per 16 seconds
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ptp\\log_announce_int", ch.CalnexAPI()), api.INTERVAL)
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ptp\\log_delay_req_int", ch.CalnexAPI()), api.INTERVAL)
			c.set(s, fmt.Sprintf("%s\\ptp_synce\\ptp\\log_sync_int", ch.CalnexAPI()), api.INTERVAL)

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

		// enable NTP/PTP/PPS channels
		c.set(s, fmt.Sprintf("%s\\used", ch.CalnexAPI()), api.YES)
		c.set(s, probe, m.Probe.CalnexName())
	}

	// Disable unused channels
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

func (c *config) baseConfig(measure *ini.Section, gnss *ini.Section, target string, antennaDelayNS int) {
	// device hostname
	c.set(measure, "device_name", target)

	// gnss antenna compensation
	c.set(gnss, "antenna_delay", fmt.Sprintf("%d ns", antennaDelayNS))

	// continuous measurement
	c.set(measure, "continuous", api.ON)

	// continuous measurement
	c.set(measure, "reference", api.INTERNAL)

	// 25h measurement
	c.set(measure, "meas_time", "1 days 1 hours")

	// tie_mode=TIE + 1 PPS Alignment
	c.set(measure, "tie_mode", "TIE + 1 PPS Alignment")

	// ch8 is a ref channel. Always ON
	c.set(measure, "ch8\\used", api.YES)

	// disable synce
	c.chSet(measure, api.ChannelONE, api.ChannelTWO, "%s\\synce_enabled", api.OFF)

	// ptp dscp
	c.chSet(measure, api.ChannelONE, api.ChannelTWO, "%s\\ptp_synce\\ptp\\dscp", "0")

	// DHCP
	c.chSet(measure, api.ChannelONE, api.ChannelTWO, "%s\\ptp_synce\\ethernet\\dhcp_v6", api.DHCP)
	c.chSet(measure, api.ChannelONE, api.ChannelTWO, "%s\\ptp_synce\\ethernet\\dhcp_v4", api.DISABLED)

	// enable QSFP FEC for 100G links (first channel only)
	c.chSet(measure, api.ChannelONE, api.ChannelONE, "%s\\ptp_synce\\ethernet\\qsfp_fec", api.RSFEC)

	// Enable 1st Physical channel
	c.set(measure, fmt.Sprintf("%s\\used", api.ChannelONE.CalnexAPI()), api.YES)
	// Disable 2nd Physical channel
	c.set(measure, fmt.Sprintf("%s\\used", api.ChannelTWO.CalnexAPI()), api.NO)

	// Disable packet measurement on the two physical channel measurements
	c.chSet(measure, api.ChannelONE, api.ChannelTWO, "%s\\protocol_enabled", api.OFF)

	// Enable virtual channel measurements for channel 1
	c.set(measure, fmt.Sprintf("%s\\virtual_channels_enabled", api.ChannelONE.CalnexAPI()), api.ON)
}

// Config configures target Calnex with Network/Calnex configs if apply is specified
func Config(target string, insecureTLS bool, cc *CalnexConfig, apply bool) error {
	var c config
	api := api.NewAPI(target, insecureTLS, 4*time.Minute)

	f, err := prepare(&c, api, target, cc)
	if err != nil {
		return err
	}

	if !apply {
		log.Info("dry run. Exiting")
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

// Save saves the Network/Calnex configs to file
func Save(target string, insecureTLS bool, cc *CalnexConfig, saveConfig string) error {
	var c config
	calnexAPI := api.NewAPI(target, insecureTLS, time.Minute)

	f, err := prepare(&c, calnexAPI, target, cc)
	if err != nil {
		return err
	}

	buf, err := api.ToBuffer(f)
	if err != nil {
		return err
	}

	if err = os.WriteFile(saveConfig, buf.Bytes(), 0644); err != nil {
		return err
	}

	return nil
}

func prepare(c *config, api *api.API, target string, cc *CalnexConfig) (*ini.File, error) {
	f, err := api.FetchSettings()
	if err != nil {
		return nil, err
	}

	m := f.Section("measure")
	g := f.Section("gnss")

	// set base config
	c.baseConfig(m, g, target, cc.AntennaDelayNS)

	// set measure config
	c.measureConfig(m, cc.Measure)

	return f, nil
}
