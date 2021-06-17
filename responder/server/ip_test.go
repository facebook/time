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

const testIP = "1.2.3.4"

func TestAddIPToInterfaceError(t *testing.T) {
	lc := ListenConfig{Iface: "lol-does-not-exist"}
	s := &Server{ListenConfig: lc}
	err := s.addIPToInterface(net.ParseIP(testIP))
	assert.NotNil(t, err)
}

func TestDeleteIPFromInterfaceError(t *testing.T) {
	lc := ListenConfig{Iface: "lol-does-not-exist"}
	s := &Server{ListenConfig: lc}
	err := s.deleteIPFromInterface(net.ParseIP(testIP))
	assert.NotNil(t, err)
}
