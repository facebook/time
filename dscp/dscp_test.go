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

package dscp

import (
	"net"
	"testing"

	"github.com/facebook/time/timestamp"
	"github.com/stretchr/testify/require"
)

func TestEnableDSCP(t *testing.T) {
	conn4, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)
	defer conn4.Close()
	// get connection file descriptor
	fd4, err := timestamp.ConnFd(conn4)
	require.NoError(t, err)
	err = Enable(fd4, net.ParseIP("127.0.0.1"), 42)
	require.NoError(t, err)

	conn6, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("::"), Port: 0})
	require.NoError(t, err)
	defer conn6.Close()
	// get connection file descriptor
	fd6, err := timestamp.ConnFd(conn6)
	require.NoError(t, err)
	err = Enable(fd6, net.ParseIP("::"), 42)
	require.NoError(t, err)
}
