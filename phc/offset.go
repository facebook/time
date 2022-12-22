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
	"golang.org/x/sys/unix"
	"os"
	"time"
)

// SysoffResult is a result of PHC time measurement with related data
type SysoffResult struct {
	Offset  time.Duration
	Delay   time.Duration
	SysTime time.Time
	PHCTime time.Time
}

// based on calculate_offset from ptp4l phc_ctl.c
func sysoffEstimateBasic(ts1, rt, ts2 time.Time) SysoffResult {
	interval := ts2.Sub(ts1)
	sysTime := ts1.Add(interval / 2)
	offset := ts2.Sub(rt) - (interval / 2)

	return SysoffResult{
		SysTime: sysTime,
		PHCTime: rt,
		Delay:   ts2.Sub(ts1),
		Offset:  offset,
	}
}

// loosely based on sysoff_estimate from ptp4l sysoff.c
func sysoffEstimateExtended(extended *PTPSysOffsetExtended) SysoffResult {
	t1 := extended.TS[0][0].Time()
	tp := extended.TS[0][1].Time()
	t2 := extended.TS[0][2].Time()
	shortestInterval := t2.Sub(t1)
	bestSysTS := t1.Add(shortestInterval / 2)
	bestPhcTS := tp
	bestOffset := bestSysTS.Sub(tp)
	for i := 1; i < int(extended.NSamples); i++ {
		t1 := extended.TS[i][0].Time()
		tp := extended.TS[i][1].Time()
		t2 := extended.TS[i][2].Time()
		interval := t2.Sub(t1)
		timestamp := t1.Add(interval / 2)
		offset := timestamp.Sub(tp)
		if interval < shortestInterval {
			shortestInterval = interval
			bestSysTS = timestamp
			bestOffset = offset
			bestPhcTS = tp
		}
	}
	return SysoffResult{
		SysTime: bestSysTS,
		PHCTime: bestPhcTS,
		Delay:   shortestInterval,
		Offset:  bestOffset,
	}
}

// TimeAndOffset returns time we got from network card + offset
func TimeAndOffset(iface string, method TimeMethod) (SysoffResult, error) {
	device, err := IfaceToPHCDevice(iface)
	if err != nil {
		return SysoffResult{}, err
	}
	return TimeAndOffsetFromDevice(device, method)
}

// TimeAndOffsetFromDevice returns time we got from phc device + offset
func TimeAndOffsetFromDevice(device string, method TimeMethod) (SysoffResult, error) {
	switch method {
	case MethodSyscallClockGettime:
		f, err := os.Open(device)
		if err != nil {
			return SysoffResult{}, err
		}
		defer f.Close()
		var ts unix.Timespec
		ts1 := time.Now()
		err = unix.ClockGettime(FDToClockID(f.Fd()), &ts)
		ts2 := time.Now()
		if err != nil {
			return SysoffResult{}, fmt.Errorf("failed clock_gettime: %w", err)
		}

		return sysoffEstimateBasic(ts1, time.Unix(ts.Unix()), ts2), nil
	case MethodIoctlSysOffsetExtended:
		extended, err := ReadPTPSysOffsetExtended(device, 5)
		if err != nil {
			return SysoffResult{}, err
		}
		return sysoffEstimateExtended(extended), nil
	}
	return SysoffResult{}, fmt.Errorf("unknown method to get PHC time %q", method)
}

// CalcPHCOffet calculates the offset between 2 SysoffResult
func CalcPHCOffet(timeAndOffsetA, timeAndOffsetB SysoffResult) (PHCDiff time.Duration) {
	sysOffset := timeAndOffsetB.SysTime.Sub(timeAndOffsetA.SysTime)
	phcOffset := timeAndOffsetB.PHCTime.Sub(timeAndOffsetA.PHCTime)
	phcOffset -= sysOffset

	return phcOffset
}
