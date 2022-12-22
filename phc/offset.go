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

const (
	// ExtendedNumProbes is the number of samples we request for IOCTL SYS_OFFSET_EXTENDED
	ExtendedNumProbes = 9
)

// SysoffResult is a result of PHC time measurement with related data
type SysoffResult struct {
	Offset  time.Duration
	Delay   time.Duration
	SysTime time.Time
	PHCTime time.Time
}

// based on sysoff_estimate from ptp4l sysoff.c
func sysoffFromExtendedTS(extendedTS [3]PTPClockTime) SysoffResult {
	t1 := extendedTS[0].Time()
	tp := extendedTS[1].Time()
	t2 := extendedTS[2].Time()
	interval := t2.Sub(t1)
	timestamp := t1.Add(interval / 2)
	offset := timestamp.Sub(tp)
	return SysoffResult{
		SysTime: timestamp,
		PHCTime: tp,
		Delay:   interval,
		Offset:  offset,
	}
}

// SysoffEstimateBasic logic based on calculate_offset from ptp4l phc_ctl.c
func SysoffEstimateBasic(ts1, rt, ts2 time.Time) SysoffResult {
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

// SysoffEstimateExtended finds sample which took least time to be read,
// logic loosely based on sysoff_estimate from ptp4l sysoff.c
func SysoffEstimateExtended(extended *PTPSysOffsetExtended) SysoffResult {
	best := sysoffFromExtendedTS(extended.TS[0])
	for i := 1; i < int(extended.NSamples); i++ {
		sysoff := sysoffFromExtendedTS(extended.TS[i])
		if sysoff.Delay < best.Delay {
			best = sysoff
		}
	}
	return best
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

		return SysoffEstimateBasic(ts1, time.Unix(ts.Unix()), ts2), nil
	case MethodIoctlSysOffsetExtended:
		extended, err := ReadPTPSysOffsetExtended(device, ExtendedNumProbes)
		if err != nil {
			return SysoffResult{}, err
		}
		return SysoffEstimateExtended(extended), nil
	}
	return SysoffResult{}, fmt.Errorf("unknown method to get PHC time %q", method)
}

// generics in standard library can't come soon enough...
func abs(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

// OffsetBetweenExtendedReadings returns estimated difference between two PHC SYS_OFFSET_EXTENDED readings
func OffsetBetweenExtendedReadings(extendedA, extendedB *PTPSysOffsetExtended) time.Duration {
	// we expect both probes to have same number of measures
	numProbes := int(extendedA.NSamples)
	if int(extendedB.NSamples) < numProbes {
		numProbes = int(extendedB.NSamples)
	}
	// calculate sys time midpoint from both samples
	sysoffA := sysoffFromExtendedTS(extendedA.TS[0])
	sysoffB := sysoffFromExtendedTS(extendedB.TS[0])
	// offset between sys time midpoints
	sysOffset := sysoffB.SysTime.Sub(sysoffA.SysTime)
	// compensate difference between PHC time by difference in system time
	phcOffset := sysoffB.PHCTime.Sub(sysoffA.PHCTime) - sysOffset
	shortest := phcOffset
	// look for smallest difference between system time midpoints
	for i := 1; i < numProbes; i++ {
		sysoffA = sysoffFromExtendedTS(extendedA.TS[i])
		sysoffB = sysoffFromExtendedTS(extendedB.TS[i])
		sysOffset = sysoffB.SysTime.Sub(sysoffA.SysTime)
		phcOffset = sysoffB.PHCTime.Sub(sysoffA.PHCTime) - sysOffset

		if abs(phcOffset) < abs(shortest) {
			shortest = phcOffset
		}
	}
	return shortest
}

// OffsetBetweenDevices returns estimated difference between two PHC devices
func OffsetBetweenDevices(deviceA, deviceB string) (time.Duration, error) {
	extendedA, err := ReadPTPSysOffsetExtended(deviceA, ExtendedNumProbes)
	if err != nil {
		return 0, err
	}
	extendedB, err := ReadPTPSysOffsetExtended(deviceB, ExtendedNumProbes)
	if err != nil {
		return 0, err
	}
	return OffsetBetweenExtendedReadings(extendedA, extendedB), nil
}
