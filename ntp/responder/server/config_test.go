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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigSet(t *testing.T) {
	testIP := "1.2.3.4"

	m := MultiIPs{}
	err := m.Set(testIP)
	require.NoError(t, err)
	require.Equal(t, net.ParseIP(testIP), m[0])
}

func TestConfigSetInvalid(t *testing.T) {
	testIP := "invalid"

	m := MultiIPs{}
	err := m.Set(testIP)
	require.NotNil(t, err)
	require.Empty(t, m)
}

func TestConfigString(t *testing.T) {
	testIP1 := "1.2.3.4"
	testIP2 := "5.6.7.8"

	m := MultiIPs{}
	err := m.Set(testIP1)
	require.NoError(t, err)
	err = m.Set(testIP2)
	require.NoError(t, err)

	require.Equal(t, m.String(), fmt.Sprintf("%s, %s", testIP1, testIP2))
}

func TestConfigSetDefault(t *testing.T) {
	m := MultiIPs{}
	m.SetDefault()

	require.Equal(t, DefaultServerIPs, m)
}
