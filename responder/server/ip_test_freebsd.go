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

	"github.com/stretchr/testify/assert"
)

func Test_checkIP(t *testing.T) {
	iface, err := net.InterfaceByName("lo0")
	assert.Nil(t, err)

	ip := net.ParseIP("::1")
	assigned, err := checkIP(iface, &ip)

	assert.Nil(t, err)
	assert.True(t, assigned)
}

func Test_checkIPFalse(t *testing.T) {
	iface, err := net.InterfaceByName("lo0")
	assert.Nil(t, err)

	ip := net.ParseIP("8.8.8.8")
	assigned, err := checkIP(iface, &ip)

	assert.Nil(t, err)
	assert.False(t, assigned)
}