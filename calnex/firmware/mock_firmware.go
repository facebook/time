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

package firmware

import (
	"github.com/facebook/time/calnex/api"
	"github.com/stretchr/testify/mock"
)

// MockCalnexUpgrader mock implementation of a calnex upgrader
type MockCalnexUpgrader struct {
	mock.Mock
}

// Firmware mock
func (m *MockCalnexUpgrader) Firmware(target string, insecureTLS bool, fw FW, apply bool, force bool) error {
	args := m.Called(target, insecureTLS, fw, apply, force)
	return args.Error(0)
}

// InProgress mock
func (m *MockCalnexUpgrader) InProgress(target string, api *api.API) (bool, error) {
	args := m.Called(target, api)
	return args.Bool(0), args.Error(1)
}

// ShouldUpgrade mock
func (m *MockCalnexUpgrader) ShouldUpgrade(target string, api *api.API, fw FW, force bool) (bool, error) {
	args := m.Called(target, api, fw, fw, force)
	return args.Bool(0), args.Error(1)
}
