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

package phc

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeviceFromIfaceNotSupported(t *testing.T) {
	dev, err := DeviceFromIface("lo")
	require.Equal(t, fmt.Errorf("no PHC support for lo"), err)
	require.Equal(t, "", dev)
}

func TestDeviceFromIfaceNotFound(t *testing.T) {
	dev, err := DeviceFromIface("lol-does-not-exist")
	require.Equal(t, fmt.Errorf("lol-does-not-exist interface is not found"), err)
	require.Equal(t, "", dev)
}
