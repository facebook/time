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
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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
	c.chSet(s, api.ChannelA, api.ChannelF, "%s\\used", api.NO)
	require.True(t, c.changed)

	buf, err := api.ToBuffer(f)
	require.NoError(t, err)
	require.Equal(t, expectedConfig, buf.String())
}

func TestBaseConfig(t *testing.T) {
	testConfig := `[measure]
ch6\synce_enabled=Off
ch7\synce_enabled=Off
ch0\used=No
ch1\used=No
ch2\used=No
ch3\used=No
ch4\used=No
ch5\used=No
ch6\used=Yes
ch7\used=Yes
ch0\protocol_enabled=Off
ch1\protocol_enabled=Off
ch2\protocol_enabled=Off
ch3\protocol_enabled=Off
ch4\protocol_enabled=Off
ch5\protocol_enabled=Off
ch6\protocol_enabled=Off
ch7\protocol_enabled=Off
ch6\ptp_synce\ethernet\dhcp=On
ch7\ptp_synce\ethernet\dhcp=On
ch6\ptp_synce\ntp\normalize_delays=On
ch7\ptp_synce\ntp\normalize_delays=On
ch6\ptp_synce\ntp\protocol_level=UDP/IPv4
ch7\ptp_synce\ntp\protocol_level=UDP/IPv4
ch6\ptp_synce\ntp\poll_log_interval=1 packet/42 s
ch7\ptp_synce\ntp\poll_log_interval=1 packet/42 s
ch6\ptp_synce\ptp\log_announce_int=1 packet/42 s
ch7\ptp_synce\ptp\log_announce_int=1 packet/42 s
ch6\ptp_synce\ptp\log_delay_req_int=1 packet/42 s
ch7\ptp_synce\ptp\log_delay_req_int=1 packet/42 s
ch6\ptp_synce\ptp\log_sync_int=1 packet/42 s
ch7\ptp_synce\ptp\log_sync_int=1 packet/42 s
ch6\ptp_synce\ptp\protocol_level=Ethernet
ch7\ptp_synce\ptp\protocol_level=Ethernet
ch6\ptp_synce\ptp\stack_mode=Multicast
ch7\ptp_synce\ptp\stack_mode=Multicast
ch6\ptp_synce\ptp\domain=24
ch7\ptp_synce\ptp\domain=24
ch6\ptp_synce\ptp\dscp=46
ch7\ptp_synce\ptp\dscp=46
continuous=Off
meas_time=10 minutes
tie_mode=TIE
`

	expectedConfig := `[measure]
ch6\synce_enabled=Off
ch7\synce_enabled=Off
ch0\used=No
ch1\used=No
ch2\used=No
ch3\used=No
ch4\used=No
ch5\used=No
ch6\used=Yes
ch7\used=Yes
ch0\protocol_enabled=Off
ch1\protocol_enabled=Off
ch2\protocol_enabled=Off
ch3\protocol_enabled=Off
ch4\protocol_enabled=Off
ch5\protocol_enabled=Off
ch6\protocol_enabled=Off
ch7\protocol_enabled=Off
ch6\ptp_synce\ethernet\dhcp=Off
ch7\ptp_synce\ethernet\dhcp=Off
ch6\ptp_synce\ntp\normalize_delays=Off
ch7\ptp_synce\ntp\normalize_delays=Off
ch6\ptp_synce\ntp\protocol_level=UDP/IPv6
ch7\ptp_synce\ntp\protocol_level=UDP/IPv6
ch6\ptp_synce\ntp\poll_log_interval=1 packet/64 s
ch7\ptp_synce\ntp\poll_log_interval=1 packet/64 s
ch6\ptp_synce\ptp\log_announce_int=1 packet/s
ch7\ptp_synce\ptp\log_announce_int=1 packet/s
ch6\ptp_synce\ptp\log_delay_req_int=1 packet/s
ch7\ptp_synce\ptp\log_delay_req_int=1 packet/s
ch6\ptp_synce\ptp\log_sync_int=1 packet/s
ch7\ptp_synce\ptp\log_sync_int=1 packet/s
ch6\ptp_synce\ptp\protocol_level=UDP/IPv6
ch7\ptp_synce\ptp\protocol_level=UDP/IPv6
ch6\ptp_synce\ptp\stack_mode=Unicast
ch7\ptp_synce\ptp\stack_mode=Unicast
ch6\ptp_synce\ptp\domain=0
ch7\ptp_synce\ptp\domain=0
ch6\ptp_synce\ptp\dscp=0
ch7\ptp_synce\ptp\dscp=0
continuous=On
meas_time=1 days 1 hours
tie_mode=TIE + 1 PPS TE
`

	c := config{}

	f, err := ini.Load([]byte(testConfig))
	require.NoError(t, err)

	s := f.Section("measure")

	c.baseConfig(s)
	require.True(t, c.changed)

	buf, err := api.ToBuffer(f)
	require.NoError(t, err)
	require.Equal(t, expectedConfig, buf.String())
}

func TestNicConfig(t *testing.T) {
	testConfig := `[measure]
ch6\ptp_synce\ethernet\gateway=192.168.4.1
ch6\ptp_synce\ethernet\gateway_ipv6=2000::000a
ch6\ptp_synce\ethernet\ip_address=192.168.4.200
ch6\ptp_synce\ethernet\ip_address_ipv6=2000::000b
ch6\ptp_synce\ethernet\mask=255.255.255.0
ch7\ptp_synce\ethernet\gateway=192.168.5.1
ch7\ptp_synce\ethernet\gateway_ipv6=2000::000a
ch7\ptp_synce\ethernet\ip_address=192.168.5.200
ch7\ptp_synce\ethernet\ip_address_ipv6=2000::000b
ch7\ptp_synce\ethernet\mask=255.255.255.0
`

	expectedConfig := `[measure]
ch6\ptp_synce\ethernet\gateway=fd00:3226:310a::a
ch6\ptp_synce\ethernet\gateway_ipv6=fd00:3226:310a::a
ch6\ptp_synce\ethernet\ip_address=fd00:3226:310a::1
ch6\ptp_synce\ethernet\ip_address_ipv6=fd00:3226:310a::1
ch6\ptp_synce\ethernet\mask=64
ch7\ptp_synce\ethernet\gateway=fd00:3226:310a::a
ch7\ptp_synce\ethernet\gateway_ipv6=fd00:3226:310a::a
ch7\ptp_synce\ethernet\ip_address=fd00:3226:310a::2
ch7\ptp_synce\ethernet\ip_address_ipv6=fd00:3226:310a::2
ch7\ptp_synce\ethernet\mask=64
`

	c := config{}

	f, err := ini.Load([]byte(testConfig))
	require.NoError(t, err)

	s := f.Section("measure")

	n := &NetworkConfig{
		Eth1: net.ParseIP("fd00:3226:310a::1"),
		Gw1:  net.ParseIP("fd00:3226:310a::a"),
		Eth2: net.ParseIP("fd00:3226:310a::2"),
		Gw2:  net.ParseIP("fd00:3226:310a::a"),
	}

	c.nicConfig(s, n)
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
ch0\protocol_enabled=Off
ch1\protocol_enabled=Off
ch1\protocol_enabled=Off
ch2\protocol_enabled=Off
ch3\protocol_enabled=Off
ch4\protocol_enabled=Off
ch5\protocol_enabled=Off
ch6\protocol_enabled=Off
ch7\protocol_enabled=Off
ch6\ptp_synce\mode\probe_type=PTP slave
ch6\ptp_synce\ntp\server_ip=10.32.1.168
ch6\ptp_synce\ntp\server_ip_ipv6=2000::000a
ch7\ptp_synce\mode\probe_type=PTP slave
ch7\ptp_synce\ptp\master_ip=10.32.1.168
ch7\ptp_synce\ptp\master_ip_ipv6=2000::000a
`

	expectedConfig := `[measure]
ch0\used=No
ch1\used=No
ch2\used=No
ch3\used=No
ch4\used=No
ch5\used=No
ch6\used=Yes
ch7\used=Yes
ch0\protocol_enabled=Off
ch1\protocol_enabled=Off
ch2\protocol_enabled=Off
ch3\protocol_enabled=Off
ch4\protocol_enabled=Off
ch5\protocol_enabled=Off
ch6\protocol_enabled=On
ch7\protocol_enabled=On
ch6\ptp_synce\mode\probe_type=NTP client
ch6\ptp_synce\ntp\server_ip=fd00:3226:301b::3f
ch6\ptp_synce\ntp\server_ip_ipv6=fd00:3226:301b::3f
ch7\ptp_synce\mode\probe_type=PTP slave
ch7\ptp_synce\ptp\master_ip=fd00:3016:3109:face:0:1:0
ch7\ptp_synce\ptp\master_ip_ipv6=fd00:3016:3109:face:0:1:0
`

	c := config{}

	f, err := ini.Load([]byte(testConfig))
	require.NoError(t, err)

	s := f.Section("measure")

	mc := map[api.Channel]MeasureConfig{
		api.ChannelONE: {
			Target: "fd00:3226:301b::3f",
			Probe:  api.ProbeNTP,
		},
		api.ChannelTWO: {
			Target: "fd00:3016:3109:face:0:1:0",
			Probe:  api.ProbePTP,
		},
	}

	c.measureConfig(s, CalnexConfig(mc))
	require.True(t, c.changed)

	buf, err := api.ToBuffer(f)
	require.NoError(t, err)
	require.Equal(t, expectedConfig, buf.String())
}

func TestConfig(t *testing.T) {
	expectedConfig := `[measure]
ch0\protocol_enabled=Off
ch0\used=No
ch1\protocol_enabled=Off
ch1\used=No
ch2\protocol_enabled=Off
ch2\used=No
ch3\protocol_enabled=Off
ch3\used=No
ch4\protocol_enabled=Off
ch4\used=No
ch5\protocol_enabled=Off
ch5\used=No
ch6\protocol_enabled=On
ch6\used=Yes
ch7\protocol_enabled=On
ch7\used=Yes
ch6\synce_enabled=Off
ch7\synce_enabled=Off
ch6\ptp_synce\ethernet\dhcp=Off
ch7\ptp_synce\ethernet\dhcp=Off
ch6\ptp_synce\ntp\normalize_delays=Off
ch7\ptp_synce\ntp\normalize_delays=Off
ch6\ptp_synce\ntp\protocol_level=UDP/IPv6
ch7\ptp_synce\ntp\protocol_level=UDP/IPv6
ch6\ptp_synce\ptp\protocol_level=UDP/IPv6
ch7\ptp_synce\ptp\protocol_level=UDP/IPv6
ch6\ptp_synce\ntp\poll_log_interval=1 packet/64 s
ch7\ptp_synce\ntp\poll_log_interval=1 packet/64 s
ch6\ptp_synce\ptp\log_announce_int=1 packet/s
ch7\ptp_synce\ptp\log_announce_int=1 packet/s
ch6\ptp_synce\ptp\log_delay_req_int=1 packet/s
ch7\ptp_synce\ptp\log_delay_req_int=1 packet/s
ch6\ptp_synce\ptp\log_sync_int=1 packet/s
ch7\ptp_synce\ptp\log_sync_int=1 packet/s
ch6\ptp_synce\ptp\stack_mode=Unicast
ch7\ptp_synce\ptp\stack_mode=Unicast
ch6\ptp_synce\ptp\domain=0
ch7\ptp_synce\ptp\domain=0
ch6\ptp_synce\ptp\dscp=0
ch7\ptp_synce\ptp\dscp=0
continuous=On
meas_time=1 days 1 hours
tie_mode=TIE + 1 PPS TE
ch6\ptp_synce\ethernet\gateway=fd00:3226:310a::a
ch6\ptp_synce\ethernet\gateway_ipv6=fd00:3226:310a::a
ch6\ptp_synce\ethernet\ip_address=fd00:3226:310a::1
ch6\ptp_synce\ethernet\ip_address_ipv6=fd00:3226:310a::1
ch6\ptp_synce\ethernet\mask=64
ch7\ptp_synce\ethernet\gateway=fd00:3226:310a::a
ch7\ptp_synce\ethernet\gateway_ipv6=fd00:3226:310a::a
ch7\ptp_synce\ethernet\ip_address=fd00:3226:310a::2
ch7\ptp_synce\ethernet\ip_address_ipv6=fd00:3226:310a::2
ch7\ptp_synce\ethernet\mask=64
ch6\ptp_synce\mode\probe_type=NTP client
ch6\ptp_synce\ntp\server_ip=fd00:3226:301b::3f
ch6\ptp_synce\ntp\server_ip_ipv6=fd00:3226:301b::3f
ch7\ptp_synce\mode\probe_type=PTP slave
ch7\ptp_synce\ptp\master_ip=fd00:3016:3109:face:0:1:0
ch7\ptp_synce\ptp\master_ip_ipv6=fd00:3016:3109:face:0:1:0
`
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		if strings.Contains(r.URL.Path, "getsettings") {
			// FetchSettings
			fmt.Fprintln(w, "[measure]\nch0\\used=No\nch6\\used=Yes\nch7\\used=No")
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
	calnexAPI := api.NewAPI(parsed.Host, true)
	calnexAPI.Client = ts.Client()

	n := &NetworkConfig{
		Eth1: net.ParseIP("fd00:3226:310a::1"),
		Gw1:  net.ParseIP("fd00:3226:310a::a"),
		Eth2: net.ParseIP("fd00:3226:310a::2"),
		Gw2:  net.ParseIP("fd00:3226:310a::a"),
	}

	mc := map[api.Channel]MeasureConfig{
		api.ChannelONE: {
			Target: "fd00:3226:301b::3f",
			Probe:  api.ProbeNTP,
		},
		api.ChannelTWO: {
			Target: "fd00:3016:3109:face:0:1:0",
			Probe:  api.ProbePTP,
		},
	}

	err := Config(parsed.Host, true, n, CalnexConfig(mc), true)
	require.NoError(t, err)
}

func TestConfigFail(t *testing.T) {
	n := &NetworkConfig{}
	mc := map[api.Channel]MeasureConfig{}

	err := Config("localhost", true, n, CalnexConfig(mc), true)
	require.Error(t, err)
}
