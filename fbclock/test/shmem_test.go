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

func TestShmemV1(t *testing.T) {
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
	err = lib.StoreFBClockDataV1(shm.File.Fd(), d)
	require.NoError(t, err)

	shmdata, err := lib.MmapShmpDataV1(shm.File.Fd())
	require.NoError(t, err)

	readD, err := lib.ReadFBClockDataV1(shmdata)
	require.NoError(t, err)
	require.Equal(t, d.IngressTimeNS, readD.IngressTimeNS)
	require.Equal(t, d.ErrorBoundNS, readD.ErrorBoundNS)
	require.InDelta(t, d.HoldoverMultiplierNS, readD.HoldoverMultiplierNS, 0.001)
}

func TestShmem(t *testing.T) {
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

func TestNewFBClockCustomNonexistentPath(t *testing.T) {
	_, err := lib.NewFBClockCustom("/nonexistent/fbclock/shm/path")
	require.Error(t, err)
	require.Contains(t, err.Error(), "initializing FBClock")
}

func TestShmemV2WithCoefPPB(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "shmemtest_v2_coef")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())
	shm, err := lib.OpenFBClockShmCustomVer(tmpfile.Name(), 2)
	require.NoError(t, err)
	defer shm.Close()
	primary := lib.DataV2{
		IngressTimeNS:        1749167822494826022,
		ErrorBoundNS:         48,
		HoldoverMultiplierNS: 64.5,
		SmearingStartS:       1483228836,
		UTCOffsetPreS:        36,
		UTCOffsetPostS:       37,
		PHCTimeNS:            1749167859494830869,
		SysclockTimeNS:       1749167822494826022,
		ClockID:              4, // CLOCK_MONOTONIC_RAW
		CoefPPB:              -493,
	}
	// REALTIME anchor section: its own full clockdata_v2 with clockId = REALTIME
	// and an independent anchor/error bound.
	realtime := lib.DataV2{
		IngressTimeNS:        1749167822494826022,
		ErrorBoundNS:         50,
		HoldoverMultiplierNS: 64.5,
		SmearingStartS:       1483228836,
		UTCOffsetPreS:        36,
		UTCOffsetPostS:       37,
		PHCTimeNS:            1749167859494831100,
		SysclockTimeNS:       1749167822494826200,
		ClockID:              0, // CLOCK_REALTIME
		CoefPPB:              -491,
	}
	err = lib.StoreFBClockDataV2(shm.File.Fd(), primary)
	require.NoError(t, err)
	err = lib.StoreFBClockDataRealtime(shm.File.Fd(), realtime)
	require.NoError(t, err)

	shmdata, err := lib.MmapShmpDataV2(shm.File.Fd())
	require.NoError(t, err)

	readPrimary, err := lib.ReadFBClockDataV2(shmdata)
	require.NoError(t, err)
	require.Equal(t, primary.IngressTimeNS, readPrimary.IngressTimeNS)
	require.Equal(t, primary.ErrorBoundNS, readPrimary.ErrorBoundNS)
	require.InDelta(t, primary.HoldoverMultiplierNS, readPrimary.HoldoverMultiplierNS, 0.001)
	require.Equal(t, primary.PHCTimeNS, readPrimary.PHCTimeNS)
	require.Equal(t, primary.SysclockTimeNS, readPrimary.SysclockTimeNS)
	require.Equal(t, primary.ClockID, readPrimary.ClockID)
	require.Equal(t, primary.CoefPPB, readPrimary.CoefPPB)

	readRealtime, err := lib.ReadFBClockDataRealtime(shmdata)
	require.NoError(t, err)
	require.Equal(t, realtime.PHCTimeNS, readRealtime.PHCTimeNS)
	require.Equal(t, realtime.SysclockTimeNS, readRealtime.SysclockTimeNS)
	require.Equal(t, realtime.ClockID, readRealtime.ClockID)
	require.Equal(t, realtime.CoefPPB, readRealtime.CoefPPB)
	require.Equal(t, realtime.ErrorBoundNS, readRealtime.ErrorBoundNS)
}

func TestFloatAsUint32EdgeCases(t *testing.T) {
	require.Equal(t, uint32(0), lib.FloatAsUint32(0))
	require.Equal(t, uint32(math.MaxUint32), lib.FloatAsUint32(100000))

	v := 1.0
	encoded := lib.FloatAsUint32(v)
	decoded := lib.Uint32AsFloat(encoded)
	require.InDelta(t, v, decoded, 0.001)
}

func TestUint64ToUint32EdgeCases(t *testing.T) {
	require.Equal(t, uint32(0), lib.Uint64ToUint32(0))
	require.Equal(t, uint32(1), lib.Uint64ToUint32(1))
	require.Equal(t, uint32(math.MaxUint32), lib.Uint64ToUint32(math.MaxUint32))
	require.Equal(t, uint32(math.MaxUint32), lib.Uint64ToUint32(math.MaxUint64))
}

func TestShmemMultipleWrites(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "shmemtest_multi")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())
	shm, err := lib.OpenFBClockShmCustom(tmpfile.Name())
	require.NoError(t, err)
	defer shm.Close()

	shmdata, err := lib.MmapShmpData(shm.File.Fd())
	require.NoError(t, err)

	for i := range 10 {
		d := lib.Data{
			IngressTimeNS:        int64(1648137249050666302 + i*1000000000),
			ErrorBoundNS:         uint64(100 + i*10),
			HoldoverMultiplierNS: 1.0 + float64(i)*0.1,
		}
		err = lib.StoreFBClockDataV1(shm.File.Fd(), d)
		require.NoError(t, err)

		readD, err := lib.ReadFBClockDataV1(shmdata)
		require.NoError(t, err)
		require.Equal(t, d.IngressTimeNS, readD.IngressTimeNS)
		require.Equal(t, d.ErrorBoundNS, readD.ErrorBoundNS)
		require.InDelta(t, d.HoldoverMultiplierNS, readD.HoldoverMultiplierNS, 0.001)
	}
}
