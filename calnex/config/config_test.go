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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/facebook/time/calnex/api"
	"github.com/go-ini/ini"
	"github.com/stretchr/testify/require"
)

func TestChSet(t *testing.T) {
	testConfig := `[measure]
ch0\used=Yes
ch1\used=Yes
ch2\used=Yes
ch3\used=Yes
ch4\used=Yes
ch5\used=Yes
`

	expectedConfig := `[measure]
ch0\used=No
ch1\used=No
ch2\used=No
ch3\used=No
ch4\used=No
ch5\used=No
`
	c := config{}

	f, err := ini.Load([]byte(testConfig))
	require.NoError(t, err)

	s := f.Section("measure")
	c.chSet("leoleovich.com", s, api.ChannelA, api.ChannelF, "%s\\used", api.NO)
	require.True(t, c.changed)

	buf, err := api.ToBuffer(f)
	require.NoError(t, err)
	require.Equal(t, expectedConfig, buf.String())
}

func TestBaseConfig(t *testing.T) {
	testConfig := `[gnss]
antenna_delay=42 ns
[measure]
continuous=Off
reference=Auto
meas_time=10 minutes
tie_mode=TIE
ch6\used=Yes
ch8\used=No
`

	expectedConfig := `[gnss]
antenna_delay=42 ns
[measure]
continuous=On
reference=Internal
meas_time=1 days 1 hours
tie_mode=TIE + 1 PPS Alignment
ch6\used=Yes
ch8\used=Yes
device_name=leoleovich.com
ch6\synce_enabled=Off
ch7\synce_enabled=Off
ch6\ptp_synce\ptp\dscp=0
ch7\ptp_synce\ptp\dscp=0
ch6\ptp_synce\ethernet\dhcp_v6=DHCP
ch7\ptp_synce\ethernet\dhcp_v6=DHCP
ch6\ptp_synce\ethernet\dhcp_v4=Disabled
ch7\ptp_synce\ethernet\dhcp_v4=Disabled
ch6\ptp_synce\ethernet\qsfp_fec=RS-FEC
ch7\used=No
ch6\protocol_enabled=Off
ch7\protocol_enabled=Off
ch6\virtual_channels_enabled=On
`

	c := config{}

	f, err := ini.Load([]byte(testConfig))
	require.NoError(t, err)

	s := f.Section("measure")
	g := f.Section("gnss")

	c.baseConfig("leoleovich.com", s, g, 42)
	require.True(t, c.changed)

	buf, err := api.ToBuffer(f)
	require.NoError(t, err)
	require.Equal(t, expectedConfig, buf.String())
}

func TestMeasureConfig(t *testing.T) {
	testConfig := `[measure]
ch0\used=No
ch1\used=Yes
ch2\used=No
ch3\used=Yes
ch4\used=No
ch5\used=Yes
ch6\used=No
ch7\used=Yes
ch8\used=No
ch9\used=No
ch10\used=No
ch11\used=No
ch12\used=No
ch13\used=No
ch14\used=No
ch15\used=No
ch16\used=No
ch17\used=No
ch18\used=No
ch19\used=No
ch20\used=No
ch21\used=No
ch22\used=No
ch23\used=No
ch24\used=No
ch25\used=No
ch26\used=No
ch27\used=No
ch28\used=No
ch29\used=No
ch30\used=No
ch31\used=No
ch32\used=No
ch33\used=No
ch34\used=No
ch35\used=No
ch36\used=No
ch37\used=No
ch38\used=No
ch39\used=No
ch40\used=No
ch6\protocol_enabled=Off
ch6\ptp_synce\mode\probe_type=Disabled
ch7\protocol_enabled=Off
ch7\ptp_synce\mode\probe_type=Disabled
ch9\protocol_enabled=Off
ch9\ptp_synce\mode\probe_type=Disabled
ch10\protocol_enabled=Off
ch10\ptp_synce\mode\probe_type=Disabled
ch11\protocol_enabled=Off
ch11\ptp_synce\mode\probe_type=Disabled
ch12\protocol_enabled=Off
ch12\ptp_synce\mode\probe_type=Disabled
ch13\protocol_enabled=Off
ch13\ptp_synce\mode\probe_type=Disabled
ch14\protocol_enabled=Off
ch14\ptp_synce\mode\probe_type=Disabled
ch15\protocol_enabled=Off
ch15\ptp_synce\mode\probe_type=Disabled
ch16\protocol_enabled=Off
ch16\ptp_synce\mode\probe_type=Disabled
ch17\protocol_enabled=Off
ch17\ptp_synce\mode\probe_type=Disabled
ch18\protocol_enabled=Off
ch18\ptp_synce\mode\probe_type=Disabled
ch19\protocol_enabled=Off
ch19\ptp_synce\mode\probe_type=Disabled
ch20\protocol_enabled=Off
ch20\ptp_synce\mode\probe_type=Disabled
ch21\protocol_enabled=Off
ch21\ptp_synce\mode\probe_type=Disabled
ch22\protocol_enabled=Off
ch22\ptp_synce\mode\probe_type=Disabled
ch23\protocol_enabled=Off
ch23\ptp_synce\mode\probe_type=Disabled
ch24\protocol_enabled=Off
ch24\ptp_synce\mode\probe_type=Disabled
ch25\protocol_enabled=Off
ch25\ptp_synce\mode\probe_type=Disabled
ch26\protocol_enabled=Off
ch26\ptp_synce\mode\probe_type=Disabled
ch27\protocol_enabled=Off
ch27\ptp_synce\mode\probe_type=Disabled
ch28\protocol_enabled=Off
ch28\ptp_synce\mode\probe_type=Disabled
ch29\protocol_enabled=Off
ch29\ptp_synce\mode\probe_type=Disabled
ch30\protocol_enabled=Off
ch30\ptp_synce\mode\probe_type=Disabled
ch31\protocol_enabled=Off
ch31\ptp_synce\mode\probe_type=Disabled
ch32\protocol_enabled=Off
ch32\ptp_synce\mode\probe_type=Disabled
ch33\protocol_enabled=Off
ch33\ptp_synce\mode\probe_type=Disabled
ch34\protocol_enabled=Off
ch34\ptp_synce\mode\probe_type=Disabled
ch35\protocol_enabled=Off
ch35\ptp_synce\mode\probe_type=Disabled
ch36\protocol_enabled=Off
ch36\ptp_synce\mode\probe_type=Disabled
ch37\protocol_enabled=Off
ch37\ptp_synce\mode\probe_type=Disabled
ch38\protocol_enabled=Off
ch38\ptp_synce\mode\probe_type=Disabled
ch39\protocol_enabled=Off
ch39\ptp_synce\mode\probe_type=Disabled
ch40\protocol_enabled=Off
ch40\ptp_synce\mode\probe_type=Disabled
ch0\server_ip=10.32.1.168
ch0\signal_type=1 PPS
ch0\trig_level=1 V
ch0\freq=1 Hz
ch0\suppress_steps=No
ch9\ptp_synce\mode\probe_type=NTP
ch9\ptp_synce\ntp\server_ip_ipv6=2000::000a
ch9\ptp_synce\physical_packet_channel=Channel 2
ch9\ptp_synce\ntp\normalize_delays=On
ch9\ptp_synce\ntp\protocol_level=UDP/IPv4
ch9\ptp_synce\ntp\poll_log_interval=1 packet/1 s
ch30\ptp_synce\mode\probe_type=PTP
ch30\ptp_synce\ptp\version=PTP_V2.1
ch30\ptp_synce\ptp\master_ip_ipv6=2000::000a
ch30\ptp_synce\physical_packet_channel=Channel 2
ch30\ptp_synce\ptp\protocol_level=UDP/IPv4
ch30\ptp_synce\ptp\log_announce_int=1 packet/1 s
ch30\ptp_synce\ptp\log_delay_req_int=1 packet/1 s
ch30\ptp_synce\ptp\log_sync_int=1 packet/1 s
ch30\ptp_synce\ptp\stack_mode=Multicast
ch30\ptp_synce\ptp\domain=0
`

	expectedConfig := `[measure]
ch0\used=Yes
ch1\used=No
ch2\used=No
ch3\used=No
ch4\used=No
ch5\used=No
ch6\used=No
ch7\used=Yes
ch8\used=No
ch9\used=Yes
ch10\used=No
ch11\used=No
ch12\used=No
ch13\used=No
ch14\used=No
ch15\used=No
ch16\used=No
ch17\used=No
ch18\used=No
ch19\used=No
ch20\used=No
ch21\used=No
ch22\used=No
ch23\used=No
ch24\used=No
ch25\used=No
ch26\used=No
ch27\used=No
ch28\used=No
ch29\used=No
ch30\used=Yes
ch31\used=No
ch32\used=No
ch33\used=No
ch34\used=No
ch35\used=No
ch36\used=No
ch37\used=No
ch38\used=No
ch39\used=No
ch40\used=No
ch6\protocol_enabled=Off
ch6\ptp_synce\mode\probe_type=Disabled
ch7\protocol_enabled=Off
ch7\ptp_synce\mode\probe_type=Disabled
ch9\protocol_enabled=On
ch9\ptp_synce\mode\probe_type=NTP
ch10\protocol_enabled=Off
ch10\ptp_synce\mode\probe_type=Disabled
ch11\protocol_enabled=Off
ch11\ptp_synce\mode\probe_type=Disabled
ch12\protocol_enabled=Off
ch12\ptp_synce\mode\probe_type=Disabled
ch13\protocol_enabled=Off
ch13\ptp_synce\mode\probe_type=Disabled
ch14\protocol_enabled=Off
ch14\ptp_synce\mode\probe_type=Disabled
ch15\protocol_enabled=Off
ch15\ptp_synce\mode\probe_type=Disabled
ch16\protocol_enabled=Off
ch16\ptp_synce\mode\probe_type=Disabled
ch17\protocol_enabled=Off
ch17\ptp_synce\mode\probe_type=Disabled
ch18\protocol_enabled=Off
ch18\ptp_synce\mode\probe_type=Disabled
ch19\protocol_enabled=Off
ch19\ptp_synce\mode\probe_type=Disabled
ch20\protocol_enabled=Off
ch20\ptp_synce\mode\probe_type=Disabled
ch21\protocol_enabled=Off
ch21\ptp_synce\mode\probe_type=Disabled
ch22\protocol_enabled=Off
ch22\ptp_synce\mode\probe_type=Disabled
ch23\protocol_enabled=Off
ch23\ptp_synce\mode\probe_type=Disabled
ch24\protocol_enabled=Off
ch24\ptp_synce\mode\probe_type=Disabled
ch25\protocol_enabled=Off
ch25\ptp_synce\mode\probe_type=Disabled
ch26\protocol_enabled=Off
ch26\ptp_synce\mode\probe_type=Disabled
ch27\protocol_enabled=Off
ch27\ptp_synce\mode\probe_type=Disabled
ch28\protocol_enabled=Off
ch28\ptp_synce\mode\probe_type=Disabled
ch29\protocol_enabled=Off
ch29\ptp_synce\mode\probe_type=Disabled
ch30\protocol_enabled=On
ch30\ptp_synce\mode\probe_type=PTP
ch31\protocol_enabled=Off
ch31\ptp_synce\mode\probe_type=Disabled
ch32\protocol_enabled=Off
ch32\ptp_synce\mode\probe_type=Disabled
ch33\protocol_enabled=Off
ch33\ptp_synce\mode\probe_type=Disabled
ch34\protocol_enabled=Off
ch34\ptp_synce\mode\probe_type=Disabled
ch35\protocol_enabled=Off
ch35\ptp_synce\mode\probe_type=Disabled
ch36\protocol_enabled=Off
ch36\ptp_synce\mode\probe_type=Disabled
ch37\protocol_enabled=Off
ch37\ptp_synce\mode\probe_type=Disabled
ch38\protocol_enabled=Off
ch38\ptp_synce\mode\probe_type=Disabled
ch39\protocol_enabled=Off
ch39\ptp_synce\mode\probe_type=Disabled
ch40\protocol_enabled=Off
ch40\ptp_synce\mode\probe_type=Disabled
ch0\server_ip=fd00:3226:301b::1f
ch0\signal_type=1 PPS
ch0\trig_level=500 mV
ch0\freq=1 Hz
ch0\suppress_steps=Yes
ch9\ptp_synce\ntp\server_ip_ipv6=fd00:3226:301b::3f
ch9\ptp_synce\physical_packet_channel=Channel 1
ch9\ptp_synce\ntp\normalize_delays=Off
ch9\ptp_synce\ntp\protocol_level=UDP/IPv6
ch9\ptp_synce\ntp\poll_log_interval=1 packet/16 s
ch30\ptp_synce\ptp\version=SPTP_V2.1
ch30\ptp_synce\ptp\master_ip_ipv6=fd00:3016:3109:face:0:1:0
ch30\ptp_synce\physical_packet_channel=Channel 1
ch30\ptp_synce\ptp\protocol_level=UDP/IPv6
ch30\ptp_synce\ptp\log_announce_int=1 packet/16 s
ch30\ptp_synce\ptp\log_delay_req_int=1 packet/16 s
ch30\ptp_synce\ptp\log_sync_int=1 packet/16 s
ch30\ptp_synce\ptp\stack_mode=Unicast
ch30\ptp_synce\ptp\domain=0
`

	c := config{}

	f, err := ini.Load([]byte(testConfig))
	require.NoError(t, err)

	s := f.Section("measure")

	cc := &CalnexConfig{
		AntennaDelayNS: 42,
		Measure: map[api.Channel]MeasureConfig{
			api.ChannelA: {
				Target: "fd00:3226:301b::1f",
				Probe:  api.ProbePPS,
			},
			api.ChannelVP1: {
				Target: "fd00:3226:301b::3f",
				Probe:  api.ProbeNTP,
			},
			api.ChannelVP22: {
				Target: "fd00:3016:3109:face:0:1:0",
				Probe:  api.ProbePTP,
			},
		},
	}

	c.measureConfig("leoleovich.com", s, cc.Measure)
	require.True(t, c.changed)

	buf, err := api.ToBuffer(f)
	require.NoError(t, err)
	require.Equal(t, expectedConfig, buf.String())
}

func TestConfig(t *testing.T) {
	expectedConfig := `[gnss]
antenna_delay=4.2 us
[measure]
device_name=%s
continuous=On
reference=Internal
meas_time=1 days 1 hours
tie_mode=TIE + 1 PPS Alignment
ch0\used=Yes
ch1\used=No
ch2\used=No
ch3\used=No
ch4\used=No
ch5\used=No
ch6\used=Yes
ch7\used=No
ch8\used=Yes
ch9\used=Yes
ch10\used=No
ch11\used=No
ch12\used=No
ch13\used=No
ch14\used=No
ch15\used=No
ch16\used=No
ch17\used=No
ch18\used=No
ch19\used=No
ch20\used=No
ch21\used=No
ch22\used=No
ch23\used=No
ch24\used=No
ch25\used=No
ch26\used=No
ch27\used=No
ch28\used=No
ch29\used=No
ch30\used=Yes
ch31\used=No
ch32\used=No
ch33\used=No
ch34\used=No
ch35\used=No
ch36\used=No
ch37\used=No
ch38\used=No
ch39\used=No
ch40\used=No
ch6\protocol_enabled=Off
ch7\protocol_enabled=Off
ch6\virtual_channels_enabled=On
ch9\protocol_enabled=On
ch9\ptp_synce\mode\probe_type=NTP
ch10\protocol_enabled=Off
ch10\ptp_synce\mode\probe_type=Disabled
ch11\protocol_enabled=Off
ch11\ptp_synce\mode\probe_type=Disabled
ch12\protocol_enabled=Off
ch12\ptp_synce\mode\probe_type=Disabled
ch13\protocol_enabled=Off
ch13\ptp_synce\mode\probe_type=Disabled
ch14\protocol_enabled=Off
ch14\ptp_synce\mode\probe_type=Disabled
ch15\protocol_enabled=Off
ch15\ptp_synce\mode\probe_type=Disabled
ch16\protocol_enabled=Off
ch16\ptp_synce\mode\probe_type=Disabled
ch17\protocol_enabled=Off
ch17\ptp_synce\mode\probe_type=Disabled
ch18\protocol_enabled=Off
ch18\ptp_synce\mode\probe_type=Disabled
ch19\protocol_enabled=Off
ch19\ptp_synce\mode\probe_type=Disabled
ch20\protocol_enabled=Off
ch20\ptp_synce\mode\probe_type=Disabled
ch21\protocol_enabled=Off
ch21\ptp_synce\mode\probe_type=Disabled
ch22\protocol_enabled=Off
ch22\ptp_synce\mode\probe_type=Disabled
ch23\protocol_enabled=Off
ch23\ptp_synce\mode\probe_type=Disabled
ch24\protocol_enabled=Off
ch24\ptp_synce\mode\probe_type=Disabled
ch25\protocol_enabled=Off
ch25\ptp_synce\mode\probe_type=Disabled
ch26\protocol_enabled=Off
ch26\ptp_synce\mode\probe_type=Disabled
ch27\protocol_enabled=Off
ch27\ptp_synce\mode\probe_type=Disabled
ch28\protocol_enabled=Off
ch28\ptp_synce\mode\probe_type=Disabled
ch29\protocol_enabled=Off
ch29\ptp_synce\mode\probe_type=Disabled
ch30\protocol_enabled=On
ch30\ptp_synce\mode\probe_type=PTP
ch31\protocol_enabled=Off
ch31\ptp_synce\mode\probe_type=Disabled
ch32\protocol_enabled=Off
ch32\ptp_synce\mode\probe_type=Disabled
ch33\protocol_enabled=Off
ch33\ptp_synce\mode\probe_type=Disabled
ch34\protocol_enabled=Off
ch34\ptp_synce\mode\probe_type=Disabled
ch35\protocol_enabled=Off
ch35\ptp_synce\mode\probe_type=Disabled
ch36\protocol_enabled=Off
ch36\ptp_synce\mode\probe_type=Disabled
ch37\protocol_enabled=Off
ch37\ptp_synce\mode\probe_type=Disabled
ch38\protocol_enabled=Off
ch38\ptp_synce\mode\probe_type=Disabled
ch39\protocol_enabled=Off
ch39\ptp_synce\mode\probe_type=Disabled
ch40\protocol_enabled=Off
ch40\ptp_synce\mode\probe_type=Disabled
ch0\server_ip=fd00:3226:301b::1f
ch0\signal_type=1 PPS
ch0\trig_level=500 mV
ch0\freq=1 Hz
ch0\suppress_steps=Yes
ch6\synce_enabled=Off
ch6\ptp_synce\ptp\dscp=0
ch6\ptp_synce\ethernet\dhcp_v4=Disabled
ch6\ptp_synce\ethernet\dhcp_v6=DHCP
ch6\ptp_synce\ethernet\qsfp_fec=RS-FEC
ch7\synce_enabled=Off
ch7\ptp_synce\ptp\dscp=0
ch7\ptp_synce\ethernet\dhcp_v4=Disabled
ch7\ptp_synce\ethernet\dhcp_v6=DHCP
ch9\ptp_synce\ntp\server_ip_ipv6=fd00:3226:301b::3f
ch9\ptp_synce\physical_packet_channel=Channel 1
ch9\ptp_synce\ntp\normalize_delays=Off
ch9\ptp_synce\ntp\protocol_level=UDP/IPv6
ch9\ptp_synce\ntp\poll_log_interval=1 packet/16 s
ch30\ptp_synce\ptp\version=SPTP_V2.1
ch30\ptp_synce\ptp\master_ip_ipv6=fd00:3016:3109:face:0:1:0
ch30\ptp_synce\physical_packet_channel=Channel 1
ch30\ptp_synce\ptp\protocol_level=UDP/IPv6
ch30\ptp_synce\ptp\log_announce_int=1 packet/16 s
ch30\ptp_synce\ptp\log_delay_req_int=1 packet/16 s
ch30\ptp_synce\ptp\log_sync_int=1 packet/16 s
ch30\ptp_synce\ptp\stack_mode=Unicast
ch30\ptp_synce\ptp\domain=0
`
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		if strings.Contains(r.URL.Path, "getsettings") {
			// FetchSettings
			fmt.Fprintln(w, "[measure]\nch0\\used=No\nch6\\used=Yes\nch9\\used=Yes\nch22\\used=Yes")
		} else if strings.Contains(r.URL.Path, "getstatus") {
			// FetchStatus
			fmt.Fprintln(w, "{\n\"referenceReady\": true,\n\"modulesReady\": true,\n\"measurementActive\": true\n}")
		} else if strings.Contains(r.URL.Path, "stopmeasurement") {
			// StopMeasure
			fmt.Fprintln(w, "{\n\"result\": true\n}")
		} else if strings.Contains(r.URL.Path, "setsettings") {
			b, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			// Config comes back shuffled every time
			require.ElementsMatch(t, strings.Split(expectedConfig, "\n"), strings.Split(string(b), "\n"))
			// PushSettings
			fmt.Fprintln(w, "{\n\"result\": true\n}")
		} else if strings.Contains(r.URL.Path, "startmeasurement") {
			// StartMeasure
			fmt.Fprintln(w, "{\n\"result\": true\n}")
		}
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := api.NewAPI(parsed.Host, true, time.Second)
	calnexAPI.Client = ts.Client()
	expectedConfig = fmt.Sprintf(expectedConfig, parsed.Host)

	cc := &CalnexConfig{
		AntennaDelayNS: 4200,
		Measure: map[api.Channel]MeasureConfig{
			api.ChannelA: {
				Target: "fd00:3226:301b::1f",
				Probe:  api.ProbePPS,
			},
			api.ChannelVP1: {
				Target: "fd00:3226:301b::3f",
				Probe:  api.ProbeNTP,
			},
			api.ChannelVP22: {
				Target: "fd00:3016:3109:face:0:1:0",
				Probe:  api.ProbePTP,
			},
		},
	}

	err := Config(parsed.Host, true, cc, true)
	require.NoError(t, err)
}

func TestConfigFail(t *testing.T) {
	cc := &CalnexConfig{Measure: map[api.Channel]MeasureConfig{}}

	err := Config("localhost", true, cc, true)
	require.Error(t, err)
}

func TestJSONExport(t *testing.T) {
	expected := `{"measure":{"A":{"target":"1 PPS","probe":"PPS","name":""},"VP1":{"target":"fd00:3226:301b::3f","probe":"NTP","name":""},"VP22":{"target":"fd00:3016:3109:face:0:1:0","probe":"PTP","name":""}},"antennaDelayNS":42}`
	cc := CalnexConfig{
		AntennaDelayNS: 42,
		Measure: map[api.Channel]MeasureConfig{
			api.ChannelA: {
				Target: "1 PPS",
				Probe:  api.ProbePPS,
			},
			api.ChannelVP1: {
				Target: "fd00:3226:301b::3f",
				Probe:  api.ProbeNTP,
			},
			api.ChannelVP22: {
				Target: "fd00:3016:3109:face:0:1:0",
				Probe:  api.ProbePTP,
			},
		},
	}

	jsonData, err := json.Marshal(cc)
	require.NoError(t, err)
	require.Equal(t, expected, string(jsonData))
}

func TestSaveConfig(t *testing.T) {
	expectedConfig := `[measure]
ch0\used=Yes
ch6\used=Yes
ch9\used=Yes
ch22\used=No
device_name=%s
continuous=On
reference=Internal
meas_time=1 days 1 hours
tie_mode=TIE + 1 PPS Alignment
ch8\used=Yes
ch6\synce_enabled=Off
ch7\synce_enabled=Off
ch6\ptp_synce\ptp\dscp=0
ch7\ptp_synce\ptp\dscp=0
ch6\ptp_synce\ethernet\dhcp_v6=DHCP
ch7\ptp_synce\ethernet\dhcp_v6=DHCP
ch6\ptp_synce\ethernet\dhcp_v4=Disabled
ch7\ptp_synce\ethernet\dhcp_v4=Disabled
ch6\ptp_synce\ethernet\qsfp_fec=RS-FEC
ch7\used=No
ch6\protocol_enabled=Off
ch7\protocol_enabled=Off
ch6\virtual_channels_enabled=On
ch0\server_ip=fd00:3226:301b::1f
ch0\trig_level=500 mV
ch0\freq=1 Hz
ch0\suppress_steps=Yes
ch0\signal_type=1 PPS
ch9\protocol_enabled=On
ch9\ptp_synce\physical_packet_channel=Channel 1
ch9\ptp_synce\ntp\server_ip_ipv6=fd00:3226:301b::3f
ch9\ptp_synce\ntp\normalize_delays=Off
ch9\ptp_synce\ntp\protocol_level=UDP/IPv6
ch9\ptp_synce\ntp\poll_log_interval=1 packet/16 s
ch9\ptp_synce\mode\probe_type=NTP
ch30\protocol_enabled=On
ch30\ptp_synce\physical_packet_channel=Channel 1
ch30\ptp_synce\ptp\version=SPTP_V2.1
ch30\ptp_synce\ptp\master_ip_ipv6=fd00:3016:3109:face:0:1:0
ch30\ptp_synce\ptp\protocol_level=UDP/IPv6
ch30\ptp_synce\ptp\log_announce_int=1 packet/16 s
ch30\ptp_synce\ptp\log_delay_req_int=1 packet/16 s
ch30\ptp_synce\ptp\log_sync_int=1 packet/16 s
ch30\ptp_synce\ptp\stack_mode=Unicast
ch30\ptp_synce\ptp\domain=0
ch30\used=Yes
ch30\ptp_synce\mode\probe_type=PTP
ch33\used=No
ch33\protocol_enabled=Off
ch33\ptp_synce\mode\probe_type=Disabled
ch35\used=No
ch35\protocol_enabled=Off
ch35\ptp_synce\mode\probe_type=Disabled
ch21\used=No
ch21\protocol_enabled=Off
ch21\ptp_synce\mode\probe_type=Disabled
ch31\used=No
ch31\protocol_enabled=Off
ch31\ptp_synce\mode\probe_type=Disabled
ch20\used=No
ch20\protocol_enabled=Off
ch20\ptp_synce\mode\probe_type=Disabled
ch22\protocol_enabled=Off
ch22\ptp_synce\mode\probe_type=Disabled
ch23\used=No
ch23\protocol_enabled=Off
ch23\ptp_synce\mode\probe_type=Disabled
ch32\used=No
ch32\protocol_enabled=Off
ch32\ptp_synce\mode\probe_type=Disabled
ch38\used=No
ch38\protocol_enabled=Off
ch38\ptp_synce\mode\probe_type=Disabled
ch40\used=No
ch40\protocol_enabled=Off
ch40\ptp_synce\mode\probe_type=Disabled
ch10\used=No
ch10\protocol_enabled=Off
ch10\ptp_synce\mode\probe_type=Disabled
ch16\used=No
ch16\protocol_enabled=Off
ch16\ptp_synce\mode\probe_type=Disabled
ch11\used=No
ch11\protocol_enabled=Off
ch11\ptp_synce\mode\probe_type=Disabled
ch12\used=No
ch12\protocol_enabled=Off
ch12\ptp_synce\mode\probe_type=Disabled
ch3\used=No
ch19\used=No
ch19\protocol_enabled=Off
ch19\ptp_synce\mode\probe_type=Disabled
ch26\used=No
ch26\protocol_enabled=Off
ch26\ptp_synce\mode\probe_type=Disabled
ch34\used=No
ch34\protocol_enabled=Off
ch34\ptp_synce\mode\probe_type=Disabled
ch4\used=No
ch18\used=No
ch18\protocol_enabled=Off
ch18\ptp_synce\mode\probe_type=Disabled
ch13\used=No
ch13\protocol_enabled=Off
ch13\ptp_synce\mode\probe_type=Disabled
ch17\used=No
ch17\protocol_enabled=Off
ch17\ptp_synce\mode\probe_type=Disabled
ch36\used=No
ch36\protocol_enabled=Off
ch36\ptp_synce\mode\probe_type=Disabled
ch37\used=No
ch37\protocol_enabled=Off
ch37\ptp_synce\mode\probe_type=Disabled
ch15\used=No
ch15\protocol_enabled=Off
ch15\ptp_synce\mode\probe_type=Disabled
ch29\used=No
ch29\protocol_enabled=Off
ch29\ptp_synce\mode\probe_type=Disabled
ch14\used=No
ch14\protocol_enabled=Off
ch14\ptp_synce\mode\probe_type=Disabled
ch25\used=No
ch25\protocol_enabled=Off
ch25\ptp_synce\mode\probe_type=Disabled
ch27\used=No
ch27\protocol_enabled=Off
ch27\ptp_synce\mode\probe_type=Disabled
ch28\used=No
ch28\protocol_enabled=Off
ch28\ptp_synce\mode\probe_type=Disabled
ch39\used=No
ch39\protocol_enabled=Off
ch39\ptp_synce\mode\probe_type=Disabled
ch1\used=No
ch2\used=No
ch24\used=No
ch24\protocol_enabled=Off
ch24\ptp_synce\mode\probe_type=Disabled
ch5\used=No
[gnss]
antenna_delay=42 ns
`

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, "[measure]\nch0\\used=No\nch6\\used=Yes\nch9\\used=Yes\nch22\\used=Yes")
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := api.NewAPI(parsed.Host, true, time.Second)
	calnexAPI.Client = ts.Client()
	expectedConfig = fmt.Sprintf(expectedConfig, parsed.Host)
	cc := &CalnexConfig{
		AntennaDelayNS: 42,
		Measure: map[api.Channel]MeasureConfig{
			api.ChannelA: {
				Target: "fd00:3226:301b::1f",
				Probe:  api.ProbePPS,
			},
			api.ChannelVP1: {
				Target: "fd00:3226:301b::3f",
				Probe:  api.ProbeNTP,
			},
			api.ChannelVP22: {
				Target: "fd00:3016:3109:face:0:1:0",
				Probe:  api.ProbePTP,
			},
		},
	}

	// Prepare tmp config file location
	f, err := os.CreateTemp("/tmp", "calnex")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	defer f.Close()

	err = Save(parsed.Host, true, cc, f.Name())
	require.NoError(t, err)

	savedConfig, err := os.ReadFile(f.Name())
	require.NoError(t, err)

	require.ElementsMatch(t, strings.Split(expectedConfig, "\n"), strings.Split(string(savedConfig), "\n"))
}

func TestSaveConfigFail(t *testing.T) {
	cc := &CalnexConfig{Measure: map[api.Channel]MeasureConfig{}}

	// Prepare tmp config file location
	f, err := os.CreateTemp("/tmp", "calnex")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	defer f.Close()

	err = Save("localhost", true, cc, f.Name())
	require.Error(t, err)
}

func TestPrepare(t *testing.T) {
	testConfig := `[gnss]
antenna_delay=650 ns
[measure]
continuous=Off
reference=Auto
meas_time=10 minutes
tie_mode=TIE
ch6\used=No
ch8\used=Yes
ch0\used=Yes
ch1\used=Yes
ch2\used=No
ch3\used=Yes
ch4\used=No
ch5\used=Yes
ch7\used=Yes
ch9\used=No
ch10\used=No
ch11\used=Yes
ch12\used=No
ch13\used=No
ch14\used=No
ch15\used=No
ch16\used=No
ch17\used=No
ch18\used=Yes
ch19\used=No
ch20\used=No
ch21\used=No
ch22\used=Yes
ch23\used=No
ch24\used=No
ch25\used=Yes
ch26\used=No
ch27\used=No
ch28\used=No
ch29\used=No
ch30\used=Yes
ch31\used=No
ch32\used=Yes
ch33\used=No
ch34\used=No
ch35\used=No
ch36\used=No
ch37\used=No
ch38\used=Yes
ch39\used=No
ch40\used=No
ch6\protocol_enabled=Yes
ch6\ptp_synce\mode\probe_type=Disabled
ch7\protocol_enabled=Off
ch7\ptp_synce\mode\probe_type=Disabled
ch9\protocol_enabled=Off
ch9\ptp_synce\mode\probe_type=Disabled
ch10\protocol_enabled=Off
ch10\ptp_synce\mode\probe_type=Disabled
ch11\protocol_enabled=Off
ch11\ptp_synce\mode\probe_type=Disabled
ch12\protocol_enabled=Off
ch12\ptp_synce\mode\probe_type=Disabled
ch13\protocol_enabled=Off
ch13\ptp_synce\mode\probe_type=Disabled
ch14\protocol_enabled=Off
ch14\ptp_synce\mode\probe_type=Disabled
ch15\protocol_enabled=On
ch15\ptp_synce\mode\probe_type=Disabled
ch16\protocol_enabled=Off
ch16\ptp_synce\mode\probe_type=Disabled
ch17\protocol_enabled=Off
ch17\ptp_synce\mode\probe_type=Disabled
ch18\protocol_enabled=Off
ch18\ptp_synce\mode\probe_type=Disabled
ch19\protocol_enabled=Off
ch19\ptp_synce\mode\probe_type=Disabled
ch20\protocol_enabled=Off
ch20\ptp_synce\mode\probe_type=Disabled
ch21\protocol_enabled=Off
ch21\ptp_synce\mode\probe_type=Disabled
ch22\protocol_enabled=Off
ch22\ptp_synce\mode\probe_type=Disabled
ch23\protocol_enabled=Off
ch23\ptp_synce\mode\probe_type=Disabled
ch24\protocol_enabled=Off
ch24\ptp_synce\mode\probe_type=Disabled
ch25\protocol_enabled=On
ch25\ptp_synce\mode\probe_type=Disabled
ch26\protocol_enabled=Off
ch26\ptp_synce\mode\probe_type=NTP
ch27\protocol_enabled=Off
ch27\ptp_synce\mode\probe_type=Disabled
ch28\protocol_enabled=Off
ch28\ptp_synce\mode\probe_type=Disabled
ch29\protocol_enabled=Off
ch29\ptp_synce\mode\probe_type=PTP
ch30\protocol_enabled=On
ch30\ptp_synce\mode\probe_type=Disabled
ch31\protocol_enabled=Off
ch31\ptp_synce\mode\probe_type=Disabled
ch32\protocol_enabled=Off
ch32\ptp_synce\mode\probe_type=Disabled
ch33\protocol_enabled=Off
ch33\ptp_synce\mode\probe_type=Disabled
ch34\protocol_enabled=Off
ch34\ptp_synce\mode\probe_type=Disabled
ch35\protocol_enabled=Off
ch35\ptp_synce\mode\probe_type=Disabled
ch36\protocol_enabled=Off
ch36\ptp_synce\mode\probe_type=Disabled
ch37\protocol_enabled=Off
ch37\ptp_synce\mode\probe_type=Disabled
ch38\protocol_enabled=Off
ch38\ptp_synce\mode\probe_type=Disabled
ch39\protocol_enabled=Off
ch39\ptp_synce\mode\probe_type=Disabled
ch40\protocol_enabled=Off
ch40\ptp_synce\mode\probe_type=Disabled
ch0\server_ip=10.32.1.168
ch0\signal_type=1 PPS
ch0\trig_level=1 V
ch0\freq=1 Hz
ch0\suppress_steps=No
ch9\ptp_synce\mode\probe_type=NTP
ch9\ptp_synce\ntp\server_ip_ipv6=2000::000a
ch9\ptp_synce\physical_packet_channel=Channel 2
ch9\ptp_synce\ntp\normalize_delays=On
ch9\ptp_synce\ntp\protocol_level=UDP/IPv4
ch9\ptp_synce\ntp\poll_log_interval=1 packet/1 s
ch30\ptp_synce\mode\probe_type=PTP
ch30\ptp_synce\ptp\version=PTP_V2.1
ch30\ptp_synce\ptp\master_ip_ipv6=2000::000a
ch30\ptp_synce\physical_packet_channel=Channel 2
ch30\ptp_synce\ptp\protocol_level=UDP/IPv4
ch30\ptp_synce\ptp\log_announce_int=1 packet/1 s
ch30\ptp_synce\ptp\log_delay_req_int=1 packet/1 s
ch30\ptp_synce\ptp\log_sync_int=1 packet/1 s
ch30\ptp_synce\ptp\stack_mode=Multicast
ch30\ptp_synce\ptp\domain=1
`

	expectedConfig := `[gnss]
antenna_delay=42 ns
[measure]
continuous=On
reference=Internal
meas_time=1 days 1 hours
tie_mode=TIE + 1 PPS Alignment
ch6\used=Yes
ch8\used=Yes
ch0\used=Yes
ch1\used=No
ch2\used=No
ch3\used=No
ch4\used=No
ch5\used=No
ch7\used=No
ch9\used=Yes
ch10\used=No
ch11\used=No
ch12\used=No
ch13\used=No
ch14\used=No
ch15\used=No
ch16\used=No
ch17\used=No
ch18\used=No
ch19\used=No
ch20\used=No
ch21\used=No
ch22\used=No
ch23\used=No
ch24\used=No
ch25\used=No
ch26\used=No
ch27\used=No
ch28\used=No
ch29\used=No
ch30\used=Yes
ch31\used=No
ch32\used=No
ch33\used=No
ch34\used=No
ch35\used=No
ch36\used=No
ch37\used=No
ch38\used=No
ch39\used=No
ch40\used=No
ch6\protocol_enabled=Off
ch6\ptp_synce\mode\probe_type=Disabled
ch7\protocol_enabled=Off
ch7\ptp_synce\mode\probe_type=Disabled
ch9\protocol_enabled=On
ch9\ptp_synce\mode\probe_type=NTP
ch10\protocol_enabled=Off
ch10\ptp_synce\mode\probe_type=Disabled
ch11\protocol_enabled=Off
ch11\ptp_synce\mode\probe_type=Disabled
ch12\protocol_enabled=Off
ch12\ptp_synce\mode\probe_type=Disabled
ch13\protocol_enabled=Off
ch13\ptp_synce\mode\probe_type=Disabled
ch14\protocol_enabled=Off
ch14\ptp_synce\mode\probe_type=Disabled
ch15\protocol_enabled=Off
ch15\ptp_synce\mode\probe_type=Disabled
ch16\protocol_enabled=Off
ch16\ptp_synce\mode\probe_type=Disabled
ch17\protocol_enabled=Off
ch17\ptp_synce\mode\probe_type=Disabled
ch18\protocol_enabled=Off
ch18\ptp_synce\mode\probe_type=Disabled
ch19\protocol_enabled=Off
ch19\ptp_synce\mode\probe_type=Disabled
ch20\protocol_enabled=Off
ch20\ptp_synce\mode\probe_type=Disabled
ch21\protocol_enabled=Off
ch21\ptp_synce\mode\probe_type=Disabled
ch22\protocol_enabled=Off
ch22\ptp_synce\mode\probe_type=Disabled
ch23\protocol_enabled=Off
ch23\ptp_synce\mode\probe_type=Disabled
ch24\protocol_enabled=Off
ch24\ptp_synce\mode\probe_type=Disabled
ch25\protocol_enabled=Off
ch25\ptp_synce\mode\probe_type=Disabled
ch26\protocol_enabled=Off
ch26\ptp_synce\mode\probe_type=Disabled
ch27\protocol_enabled=Off
ch27\ptp_synce\mode\probe_type=Disabled
ch28\protocol_enabled=Off
ch28\ptp_synce\mode\probe_type=Disabled
ch29\protocol_enabled=Off
ch29\ptp_synce\mode\probe_type=Disabled
ch30\protocol_enabled=On
ch30\ptp_synce\mode\probe_type=PTP
ch31\protocol_enabled=Off
ch31\ptp_synce\mode\probe_type=Disabled
ch32\protocol_enabled=Off
ch32\ptp_synce\mode\probe_type=Disabled
ch33\protocol_enabled=Off
ch33\ptp_synce\mode\probe_type=Disabled
ch34\protocol_enabled=Off
ch34\ptp_synce\mode\probe_type=Disabled
ch35\protocol_enabled=Off
ch35\ptp_synce\mode\probe_type=Disabled
ch36\protocol_enabled=Off
ch36\ptp_synce\mode\probe_type=Disabled
ch37\protocol_enabled=Off
ch37\ptp_synce\mode\probe_type=Disabled
ch38\protocol_enabled=Off
ch38\ptp_synce\mode\probe_type=Disabled
ch39\protocol_enabled=Off
ch39\ptp_synce\mode\probe_type=Disabled
ch40\protocol_enabled=Off
ch40\ptp_synce\mode\probe_type=Disabled
ch0\server_ip=fd00:3226:301b::1f
ch0\signal_type=1 PPS
ch0\trig_level=500 mV
ch0\freq=1 Hz
ch0\suppress_steps=Yes
ch9\ptp_synce\ntp\server_ip_ipv6=fd00:3226:301b::3f
ch9\ptp_synce\physical_packet_channel=Channel 1
ch9\ptp_synce\ntp\normalize_delays=Off
ch9\ptp_synce\ntp\protocol_level=UDP/IPv6
ch9\ptp_synce\ntp\poll_log_interval=1 packet/16 s
ch30\ptp_synce\ptp\version=SPTP_V2.1
ch30\ptp_synce\ptp\master_ip_ipv6=fd00:3016:3109:face:0:1:0
ch30\ptp_synce\physical_packet_channel=Channel 1
ch30\ptp_synce\ptp\protocol_level=UDP/IPv6
ch30\ptp_synce\ptp\log_announce_int=1 packet/16 s
ch30\ptp_synce\ptp\log_delay_req_int=1 packet/16 s
ch30\ptp_synce\ptp\log_sync_int=1 packet/16 s
ch30\ptp_synce\ptp\stack_mode=Unicast
ch30\ptp_synce\ptp\domain=0
device_name=leoleovich.com
ch6\synce_enabled=Off
ch7\synce_enabled=Off
ch6\ptp_synce\ptp\dscp=0
ch7\ptp_synce\ptp\dscp=0
ch6\ptp_synce\ethernet\dhcp_v6=DHCP
ch7\ptp_synce\ethernet\dhcp_v6=DHCP
ch6\ptp_synce\ethernet\dhcp_v4=Disabled
ch7\ptp_synce\ethernet\dhcp_v4=Disabled
ch6\ptp_synce\ethernet\qsfp_fec=RS-FEC
ch6\virtual_channels_enabled=On
`

	var c config
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		fmt.Fprintln(w, testConfig)
	}))
	defer ts.Close()

	parsed, _ := url.Parse(ts.URL)
	calnexAPI := api.NewAPI(parsed.Host, true, time.Second)
	calnexAPI.Client = ts.Client()
	cc := &CalnexConfig{
		AntennaDelayNS: 42,
		Measure: map[api.Channel]MeasureConfig{
			api.ChannelA: {
				Target: "fd00:3226:301b::1f",
				Probe:  api.ProbePPS,
			},
			api.ChannelVP1: {
				Target: "fd00:3226:301b::3f",
				Probe:  api.ProbeNTP,
			},
			api.ChannelVP22: {
				Target: "fd00:3016:3109:face:0:1:0",
				Probe:  api.ProbePTP,
			},
		},
	}

	f, err := prepare(&c, calnexAPI, "leoleovich.com", cc)
	require.NoError(t, err)
	require.True(t, c.changed)

	buf, err := api.ToBuffer(f)
	require.NoError(t, err)
	require.Equal(t, expectedConfig, buf.String())
}
