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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIfaceInfoToPHCDevice(t *testing.T) {
	info := &EthtoolTSinfo{
		PHCIndex: 0,
	}
	got, err := ifaceInfoToPHCDevice(info)
	require.NoError(t, err)
	require.Equal(t, "/dev/ptp0", got)

	info.PHCIndex = 23
	got, err = ifaceInfoToPHCDevice(info)
	require.NoError(t, err)
	require.Equal(t, "/dev/ptp23", got)

	info.PHCIndex = -1
	_, err = ifaceInfoToPHCDevice(info)
	require.Error(t, err)
}

func TestMaxAdjFreq(t *testing.T) {
	caps := &PTPClockCaps{
		MaxAdj: 1000000000,
	}

	got := maxAdj(caps)
	require.InEpsilon(t, 1000000000.0, got, 0.00001)

	caps.MaxAdj = 0
	got = maxAdj(caps)
	require.InEpsilon(t, 500000.0, got, 0.00001)

}
