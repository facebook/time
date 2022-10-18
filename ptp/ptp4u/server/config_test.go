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
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestConfigifaceIPs(t *testing.T) {
	ips, err := ifaceIPs("lo")
	require.Nil(t, err)

	los := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("0.0.0.0")}

	found := 0
	for _, ip := range ips {
		for _, lo := range los {
			if ip.Equal(lo) {
				found++
				break
			}
		}
	}
	require.Equal(t, len(los), found)
}

func TestConfigIfaceHasIP(t *testing.T) {
	c := Config{StaticConfig: StaticConfig{Interface: "lo"}}

	c.IP = net.ParseIP("::")
	found, err := c.IfaceHasIP()
	require.Nil(t, err)
	require.True(t, found)

	c.IP = net.ParseIP("1.2.3.4")
	found, err = c.IfaceHasIP()
	require.Nil(t, err)
	require.False(t, found)

	c = Config{StaticConfig: StaticConfig{Interface: "lol-does-not-exist"}}
	c.IP = net.ParseIP("::")
	found, err = c.IfaceHasIP()
	require.NotNil(t, err)
	require.False(t, found)
}

func TestReadDynamicConfigOk(t *testing.T) {
	expected := &DynamicConfig{
		ClockAccuracy:  0,
		ClockClass:     1,
		DrainInterval:  2 * time.Second,
		MaxSubDuration: 3 * time.Hour,
		MetricInterval: 4 * time.Minute,
		MinSubInterval: 5 * time.Second,
		UTCOffset:      37 * time.Second,
	}

	dc, err := ReadDynamicConfig("")
	require.Error(t, err)
	require.Nil(t, dc)
	cfg, err := os.CreateTemp("", "ptp4u")
	require.NoError(t, err)
	defer os.Remove(cfg.Name())

	config := `clockaccuracy: 0
clockclass: 1
draininterval: "2s"
maxsubduration: "3h"
metricinterval: "4m"
minsubinterval: "5s"
utcoffset: "37s"
`
	_, err = cfg.WriteString(config)
	require.NoError(t, err)

	dc, err = ReadDynamicConfig(cfg.Name())
	require.NoError(t, err)
	require.Equal(t, expected, dc)
}

func TestReadDynamicConfigInvalid(t *testing.T) {
	config := `clockaccuracy: 1
clockclass: 2
draininterval: "3s"
maxsubduration: "4h"
metricinterval: "5m"
minsubinterval: "6s"
utcoffset: "7s"
`
	cfg, err := os.CreateTemp("", "ptp4u")
	require.NoError(t, err)
	defer os.Remove(cfg.Name())

	_, err = cfg.WriteString(config)
	require.NoError(t, err)

	dc, err := ReadDynamicConfig(cfg.Name())
	require.ErrorIs(t, err, errInsaneUTCoffset)
	require.Nil(t, dc)
}

func TestReadDynamicConfigDamaged(t *testing.T) {
	config := "Random stuff"
	cfg, err := os.CreateTemp("", "ptp4u")
	require.NoError(t, err)
	defer os.Remove(cfg.Name())

	_, err = cfg.WriteString(config)
	require.NoError(t, err)

	dc, err := ReadDynamicConfig(cfg.Name())
	require.Error(t, err)
	require.Nil(t, dc)
}

func TestWriteDynamicConfig(t *testing.T) {
	expected := `clockaccuracy: 0
clockclass: 1
draininterval: 2s
maxsubduration: 3h0m0s
metricinterval: 4m0s
minsubinterval: 5s
utcoffset: 37s
`
	dc := &DynamicConfig{
		ClockAccuracy:  0,
		ClockClass:     1,
		DrainInterval:  2 * time.Second,
		MaxSubDuration: 3 * time.Hour,
		MetricInterval: 4 * time.Minute,
		MinSubInterval: 5 * time.Second,
		UTCOffset:      37 * time.Second,
	}

	cfg, err := os.CreateTemp("", "ptp4u")
	require.NoError(t, err)
	os.Remove(cfg.Name())
	require.NoFileExists(t, cfg.Name())

	err = dc.Write(cfg.Name())
	defer os.Remove(cfg.Name())
	require.NoError(t, err)

	rl, err := os.ReadFile(cfg.Name())
	require.NoError(t, err)
	require.Equal(t, expected, string(rl))
}

func TestUTCOffsetSanity(t *testing.T) {
	dc := &DynamicConfig{}
	dc.UTCOffset = 10 * time.Second
	require.ErrorIs(t, errInsaneUTCoffset, dc.UTCOffsetSanity())
	dc.UTCOffset = 60 * time.Second
	require.ErrorIs(t, errInsaneUTCoffset, dc.UTCOffsetSanity())
	dc.UTCOffset = 37 * time.Second
	require.NoError(t, dc.UTCOffsetSanity())
}

func TestPidFile(t *testing.T) {
	cfg, err := os.CreateTemp("", "ptp4u")
	require.NoError(t, err)
	defer os.Remove(cfg.Name())
	c := &Config{StaticConfig: StaticConfig{PidFile: cfg.Name()}}

	_, err = cfg.WriteString("rubbish")
	require.NoError(t, err)
	pid, err := ReadPidFile(c.PidFile)
	require.Error(t, err)
	require.Equal(t, 0, pid)
	os.Remove(cfg.Name())
	require.NoFileExists(t, cfg.Name())

	err = c.CreatePidFile()
	require.NoError(t, err)
	require.FileExists(t, c.PidFile)

	pid, err = ReadPidFile(c.PidFile)
	require.NoError(t, err)
	require.Equal(t, unix.Getpid(), pid)

	err = c.DeletePidFile()
	require.NoError(t, err)
	require.NoFileExists(t, c.PidFile)
}
