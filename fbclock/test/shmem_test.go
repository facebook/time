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

package test

import (
	"math"
	"os"
	"testing"

	lib "github.com/facebook/time/fbclock"

	"github.com/stretchr/testify/require"
)

func TestFloatAsUint32(t *testing.T) {
	v := 0.312
	g := lib.FloatAsUint32(v)
	require.Equal(t, uint32(0x4fdf), g)
	require.InDelta(t, v, lib.Uint32AsFloat(g), 0.001)

	v = 32333233
	g = lib.FloatAsUint32(v)
	require.Equal(t, uint32(math.MaxUint32), g)
}

func TestUint64ToUint32(t *testing.T) {
	var v uint64 = 100
	g := lib.Uint64ToUint32(v)
	require.Equal(t, uint32(v), g)

	v = 32333233333
	g = lib.Uint64ToUint32(v)
	require.Equal(t, uint32(math.MaxUint32), g)
}

func TestShmem(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "shmemtest")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())
	shm, err := lib.OpenFBClockShmCustom(tmpfile.Name())
	require.NoError(t, err)
	defer shm.Close()
	d := lib.Data{
		IngressTimeNS:        1648137249050666302,
		ErrorBoundNS:         314000000, // over 65k, our old limit
		HoldoverMultiplierNS: 1.001,
	}
	err = lib.StoreFBClockData(shm.File.Fd(), d)
	require.NoError(t, err)

	shmdata, err := lib.MmapShmpData(shm.File.Fd())
	require.NoError(t, err)

	readD, err := lib.ReadFBClockData(shmdata)
	require.NoError(t, err)
	require.Equal(t, d.IngressTimeNS, readD.IngressTimeNS)
	require.Equal(t, d.ErrorBoundNS, readD.ErrorBoundNS)
	require.InDelta(t, d.HoldoverMultiplierNS, readD.HoldoverMultiplierNS, 0.001)
}

func TestShmemV2(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "shmemtest_v2")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())
	shm, err := lib.OpenFBClockShmCustomVer(tmpfile.Name(), 2)
	require.NoError(t, err)
	defer shm.Close()
	d := lib.DataV2{
		IngressTimeNS:        1648137249050666302,
		ErrorBoundNS:         314000000, // over 65k, our old limit
		HoldoverMultiplierNS: 1.001,
		PHCTimeNS:            1648137249050666302,
		SysclockTimeNS:       1648137249050666302,
		ClockID:              1, // CLOCK_MONOTONIC
	}
	err = lib.StoreFBClockDataV2(shm.File.Fd(), d)
	require.NoError(t, err)

	shmdata, err := lib.MmapShmpDataV2(shm.File.Fd())
	require.NoError(t, err)

	readD, err := lib.ReadFBClockDataV2(shmdata)
	require.NoError(t, err)
	require.Equal(t, d.IngressTimeNS, readD.IngressTimeNS)
	require.Equal(t, d.ErrorBoundNS, readD.ErrorBoundNS)
	require.InDelta(t, d.HoldoverMultiplierNS, readD.HoldoverMultiplierNS, 0.001)
	require.Equal(t, d.PHCTimeNS, readD.PHCTimeNS)
	require.Equal(t, d.SysclockTimeNS, readD.SysclockTimeNS)
	require.Equal(t, d.ClockID, readD.ClockID)
}
