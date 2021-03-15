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

	"github.com/stretchr/testify/assert"
)

func Test_ConfigSet(t *testing.T) {
	testIP := "1.2.3.4"

	m := MultiIPs{}
	err := m.Set(testIP)
	assert.Nil(t, err)
	assert.Equal(t, net.ParseIP(testIP), m[0])
}

func Test_ConfigSetInvalid(t *testing.T) {
	testIP := "invalid"

	m := MultiIPs{}
	err := m.Set(testIP)
	assert.NotNil(t, err)
	assert.Empty(t, m)
}

func Test_ConfigString(t *testing.T) {
	testIP1 := "1.2.3.4"
	testIP2 := "5.6.7.8"

	m := MultiIPs{}
	err := m.Set(testIP1)
	assert.Nil(t, err)
	err = m.Set(testIP2)
	assert.Nil(t, err)

	assert.Equal(t, m.String(), fmt.Sprintf("%s, %s", testIP1, testIP2))
}

func Test_ConfigSetDefault(t *testing.T) {
	m := MultiIPs{}
	m.SetDefault()

	assert.Equal(t, DefaultServerIPs, m)
}
