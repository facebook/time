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

package oscillatord

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOscillatordRead(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() {
		// read newline
		b := make([]byte, 1)
		_, err := server.Read(b)
		require.Nil(t, err)
		// write response
		data := `{ "oscillator": { "model": "sa3x", "fine_ctrl": 0, "coarse_ctrl": 0, "lock": false, "temperature": 45.944000000000003 }, "gnss": { "fix": 5, "fixOk": true, "antenna_power": 1, "antenna_status": 4, "lsChange": 0, "leap_seconds": 18 } }`
		_, err = server.Write([]byte(data))
		require.Nil(t, err)
	}()
	status, err := ReadStatus(client)
	require.Nil(t, err)
	want := &Status{
		Oscillator: Oscillator{
			Model:       "sa3x",
			FineCtrl:    0,
			CoarseCtrl:  0,
			Lock:        false,
			Temperature: 45.944,
		},
		GNSS: GNSS{
			Fix:           Fix3D,
			FixOK:         true,
			AntennaPower:  AntPowerOn,
			AntennaStatus: AntStatusOpen,
			LSChange:      LeapNoWarning,
			LeapSeconds:   18,
		},
	}
	require.Equal(t, want, status)
}

func TestOscillatordReadFail(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go func() {
		// read newline
		b := make([]byte, 1)
		_, err := server.Read(b)
		require.Nil(t, err)
		server.Close()
	}()
	_, err := ReadStatus(client)
	require.Error(t, err)
}

func TestOscillatordReadGarbage(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() {
		// read newline
		b := make([]byte, 1)
		_, err := server.Read(b)
		require.Nil(t, err)
		// write response
		data := `{ fdkfjd }`
		_, err = server.Write([]byte(data))
		require.Nil(t, err)
	}()
	_, err := ReadStatus(client)
	require.Error(t, err)
}
