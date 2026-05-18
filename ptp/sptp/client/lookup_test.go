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

package client

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLookupNetIPWithIPv4(t *testing.T) {
	ip, err := LookupNetIP("192.168.1.1")
	require.NoError(t, err)
	require.Equal(t, "192.168.1.1", ip.String())
}

func TestLookupNetIPWithIPv6(t *testing.T) {
	ip, err := LookupNetIP("::1")
	require.NoError(t, err)
	require.Equal(t, "::1", ip.String())
}

func TestLookupNetIPWithIPv6Full(t *testing.T) {
	ip, err := LookupNetIP("2001:db8::1")
	require.NoError(t, err)
	require.Equal(t, "2001:db8::1", ip.String())
}

func TestLookupNetIPInvalid(t *testing.T) {
	_, err := LookupNetIP("definitely.not.resolvable.invalid")
	require.Error(t, err)
}

func TestLookupNetIPLocalhost(t *testing.T) {
	ip, err := LookupNetIP("localhost")
	require.NoError(t, err)
	require.True(t, ip.IsLoopback())
}
