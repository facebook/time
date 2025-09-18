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
	"errors"
	"net"
	"testing"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/stretchr/testify/require"
)

func TestOscillatordRead(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() {
		// read empty json
		b := make([]byte, 2)
		_, err := server.Read(b)
		require.Nil(t, err)
		// write response
		data := `{ "oscillator": { "model": "sa3x", "fine_ctrl": 0, "coarse_ctrl": 0, "lock": false, "temperature": 45.944000000000003 }, "gnss": { "fix": 5, "fixOk": true, "antenna_power": 1, "antenna_status": 4, "lsChange": 0, "leap_seconds": 18, "satellites_count": 10, "time_accuracy": 13 }, "clock": { "class": "Holdover", "offset": -265095 } }`
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
			Fix:             Fix3D,
			FixOK:           true,
			AntennaPower:    AntPowerOn,
			AntennaStatus:   AntStatusOpen,
			LSChange:        LeapNoWarning,
			LeapSeconds:     18,
			SatellitesCount: 10,
			TimeAccuracy:    13,
		},
		Clock: Clock{
			Class:  ClockClassHoldover,
			Offset: -265095,
		},
	}
	require.Equal(t, want, status)
}

func TestOscillatordReadFail(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go func() {
		// read empty json
		b := make([]byte, 2)
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
		// read empty json
		b := make([]byte, 2)
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

func TestAntennaStatus(t *testing.T) {
	var a AntennaStatus
	require.Equal(t, AntStatusInit, a)
	require.Equal(t, antennaStatusToString[AntStatusInit], AntStatusInit.String())
	for k := range antennaStatusToString {
		require.Equal(t, antennaStatusToString[k], k.String())
	}

	a = 42
	require.Equal(t, "UNSUPPORTED VALUE", a.String())
}

func TestAntennaPower(t *testing.T) {
	var a AntennaPower
	require.Equal(t, AntPowerOff, a)
	require.Equal(t, antennaPowerToString[AntPowerOff], AntPowerOff.String())
	for k := range antennaPowerToString {
		require.Equal(t, antennaPowerToString[k], k.String())
	}

	a = 42
	require.Equal(t, "UNSUPPORTED VALUE", a.String())
}

func TestGNSSFix(t *testing.T) {
	var g GNSSFix
	require.Equal(t, FixUnknown, g)
	require.Equal(t, gnssFixToString[FixUnknown], FixUnknown.String())
	for k := range gnssFixToString {
		require.Equal(t, gnssFixToString[k], k.String())
	}

	g = 42
	require.Equal(t, "UNSUPPORTED VALUE", g.String())
}

func TestLeapSecondChange(t *testing.T) {
	var l LeapSecondChange
	require.Equal(t, LeapNoWarning, l)
	require.Equal(t, leapSecondChangeToString[LeapNoWarning], LeapNoWarning.String())
	for k := range leapSecondChangeToString {
		require.Equal(t, leapSecondChangeToString[k], k.String())
	}

	l = 42
	require.Equal(t, "UNSUPPORTED VALUE", l.String())
}

func TestClockClass(t *testing.T) {
	require.Equal(t, ClockClass(ptp.ClockClass6), ClockClassLock)
	require.Equal(t, ClockClass(ptp.ClockClass7), ClockClassHoldover)
	require.Equal(t, ClockClass(ptp.ClockClass13), ClockClassCalibrating)
	require.Equal(t, ClockClass(ptp.ClockClass52), ClockClassUncalibrated)

	require.Equal(t, clockClassToString[ClockClassLock], ClockClassLock.String())
	for k := range clockClassToString {
		require.Equal(t, clockClassToString[k], k.String())
	}

	require.Equal(t, "UNSUPPORTED VALUE", ClockClass(42).String())
}

func TestClockClassUnmarshalText(t *testing.T) {
	c := ClockClass(42)
	err := c.UnmarshalText([]byte("Lock"))
	require.NoError(t, err)
	require.Equal(t, ClockClassLock, c)

	err = c.UnmarshalText([]byte("Holdover"))
	require.NoError(t, err)
	require.Equal(t, ClockClassHoldover, c)

	err = c.UnmarshalText([]byte("Calibrating"))
	require.NoError(t, err)
	require.Equal(t, ClockClassCalibrating, c)

	err = c.UnmarshalText([]byte("Uncalibrated"))
	require.NoError(t, err)
	require.Equal(t, ClockClassUncalibrated, c)

	err = c.UnmarshalText([]byte("blah"))
	require.Equal(t, errors.New("clock class blah not supported"), err)
}

func TestJSON(t *testing.T) {
	expected := `{"ptp.timecard.clock.class":7,"ptp.timecard.clock.offset_ns":-265095,"ptp.timecard.gnss.antenna_power":1,"ptp.timecard.gnss.antenna_status":4,"ptp.timecard.gnss.fix_num":5,"ptp.timecard.gnss.fix_ok":1,"ptp.timecard.gnss.leap_second_change":0,"ptp.timecard.gnss.leap_seconds":18,"ptp.timecard.gnss.satellites_count":10,"ptp.timecard.gnss.time_accuracy_ns":13,"ptp.timecard.oscillator.coarse_ctrl":42,"ptp.timecard.oscillator.fine_ctrl":4242,"ptp.timecard.oscillator.lock":0,"ptp.timecard.oscillator.temperature":45.944}`
	s := &Status{
		Oscillator: Oscillator{
			Model:       "sa5x",
			FineCtrl:    4242,
			CoarseCtrl:  42,
			Lock:        false,
			Temperature: 45.944,
		},
		GNSS: GNSS{
			Fix:             Fix3D,
			FixOK:           true,
			AntennaPower:    AntPowerOn,
			AntennaStatus:   AntStatusOpen,
			LSChange:        LeapNoWarning,
			LeapSeconds:     18,
			SatellitesCount: 10,
			TimeAccuracy:    13,
		},
		Clock: Clock{
			Class:  ClockClassHoldover,
			Offset: -265095,
		},
	}
	j, err := s.MonitoringJSON("ptp.timecard")
	require.NoError(t, err)

	require.Equal(t, expected, string(j))
}

func TestBool2int(t *testing.T) {
	res := bool2int(true)
	require.Equal(t, int64(1), res)

	res = bool2int(false)
	require.Equal(t, int64(0), res)
}
