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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
	c := Config{Interface: "lo"}

	c.IP = net.ParseIP("::")
	found, err := c.IfaceHasIP()
	require.Nil(t, err)
	require.True(t, found)

	c.IP = net.ParseIP("1.2.3.4")
	found, err = c.IfaceHasIP()
	require.Nil(t, err)
	require.False(t, found)

	c = Config{Interface: "lol-does-not-exist"}
	c.IP = net.ParseIP("::")
	found, err = c.IfaceHasIP()
	require.NotNil(t, err)
	require.False(t, found)
}

func TestConfigSetUTCOffsetFromSHM(t *testing.T) {
	utcoffset := 42 * time.Second
	c := Config{UTCOffset: utcoffset}
	err := c.SetUTCOffsetFromSHM()
	require.NotNil(t, err)
	require.Equal(t, utcoffset, c.UTCOffset)
}

func TestConfigSetUTCOffsetFromLeapsectz(t *testing.T) {
	utcoffset := 42 * time.Second
	c := Config{UTCOffset: utcoffset}
	err := c.SetUTCOffsetFromLeapsectz()
	require.NoError(t, err)
	require.Greater(t, 50*time.Second, c.UTCOffset)
	require.Less(t, 30*time.Second, c.UTCOffset)
}

func TestLeapSanity(t *testing.T) {
	require.False(t, leapSanity(10))
	require.False(t, leapSanity(60))
	require.True(t, leapSanity(37))
}
