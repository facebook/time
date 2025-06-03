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
	"os"
	"time"

	"github.com/facebook/time/phc/unix" // a temporary shim for "golang.org/x/sys/unix" until v0.27.0 is cut
)

const (
	// ExtendedNumProbes is the number of samples we request for IoctlPtpSysOffsetExtended
	ExtendedNumProbes = 9
)

// PTPSysOffsetExtended wraps unix.PtpSysOffsetExtended to add methods
type PTPSysOffsetExtended unix.PtpSysOffsetExtended

// PTPSysOffsetPrecise wraps unix.PtpSysOffsetPrecise to add methods
type PTPSysOffsetPrecise unix.PtpSysOffsetPrecise

// SysoffResult is a result of PHC time measurement with related data
type SysoffResult struct {
	Offset     time.Duration
	Delay      time.Duration
	SysTime    time.Time
	SysClockID int
	PHCTime    time.Time
}

// based on sysoff_estimate from ptp4l sysoff.c
func sysoffFromExtendedTS(extendedTS [3]PtpClockTime) SysoffResult {
	t1 := time.Unix(extendedTS[0].Sec, int64(extendedTS[0].Nsec))
	tp := time.Unix(extendedTS[1].Sec, int64(extendedTS[1].Nsec))
	t2 := time.Unix(extendedTS[2].Sec, int64(extendedTS[2].Nsec))
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

// SysoffFromPrecise returns SysoffResult from *PTPSysOffsetPrecise . Code based on sysoff_precise from ptp4l sysoff.c
func SysoffFromPrecise(pre *PTPSysOffsetPrecise) SysoffResult {
	tp := time.Unix(pre.Device.Sec, int64(pre.Device.Nsec))
	tr := time.Unix(pre.Monoraw.Sec, int64(pre.Monoraw.Nsec))
	return SysoffResult{
		SysTime:    tr,
		SysClockID: unix.CLOCK_MONOTONIC_RAW,
		PHCTime:    tp,
		Delay:      0, // They are measured at the same time
		Offset:     tr.Sub(tp),
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

// BestSample finds a sample which took the least time to be read;
// the logic is loosely based on sysoff_estimate from ptp4l sysoff.c
func (extended *PTPSysOffsetExtended) BestSample() SysoffResult {
	best := sysoffFromExtendedTS(extended.Ts[0])
	best.SysClockID = int(extended.ClockID)
	for i := 1; i < int(extended.Samples); i++ {
		sysoff := sysoffFromExtendedTS(extended.Ts[i])
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
	f, err := os.Open(device)
	if err != nil {
		return SysoffResult{}, err
	}
	defer f.Close()
	dev := FromFile(f)

	switch method {
	case MethodSyscallClockGettime:
		var ts unix.Timespec
		ts1 := time.Now()
		err = unix.ClockGettime(dev.ClockID(), &ts)
		ts2 := time.Now()
		if err != nil {
			return SysoffResult{}, fmt.Errorf("failed clock_gettime: %w", err)
		}

		return SysoffEstimateBasic(ts1, time.Unix(ts.Unix()), ts2), nil
	case MethodIoctlSysOffsetExtended:
		extended, err := dev.ReadSysoffExtended()
		if err != nil {
			return SysoffResult{}, err
		}
		return extended.BestSample(), nil
	case MethodIoctlSysOffsetExtendedRealTimeClock:
		extended, err := dev.ReadSysoffExtendedRealTimeClock1()
		if err != nil {
			return SysoffResult{}, err
		}
		return extended.BestSample(), nil
	case MethodIoctlSysOffsetPrecise:
		precise, err := dev.ReadSysoffPrecise()
		if err != nil {
			return SysoffResult{}, err
		}
		return SysoffFromPrecise(precise), nil
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

// Sub returns the estimated difference between two PHC SYS_OFFSET_EXTENDED readings
func (extended *PTPSysOffsetExtended) Sub(a *PTPSysOffsetExtended) (time.Duration, error) {
	return offsetBetweenExtendedReadings(a, extended)
}

// Sub returns the estimated difference between two PHC SYS_OFFSET_PRECISE readings
func (precise *PTPSysOffsetPrecise) Sub(a *PTPSysOffsetPrecise) time.Duration {
	return offsetBetweenPreciseReadings(a, precise)
}

// offsetBetweenExtendedReadings returns estimated difference between two PHC SYS_OFFSET_EXTENDED readings
func offsetBetweenExtendedReadings(extendedA, extendedB *PTPSysOffsetExtended) (time.Duration, error) {
	// SYS_OFFSET_EXTENDED can provide different system clock, we can compare samples with same system clock id
	if extendedA.ClockID != extendedB.ClockID {
		return 0, fmt.Errorf("different system clock ids")
	}
	// we expect both probes to have same number of measures
	numProbes := int(extendedA.Samples)
	if int(extendedB.Samples) < numProbes {
		numProbes = int(extendedB.Samples)
	}
	// calculate sys time midpoint from both samples
	sysoffA := sysoffFromExtendedTS(extendedA.Ts[0])
	sysoffB := sysoffFromExtendedTS(extendedB.Ts[0])
	// offset between sys time midpoints
	sysOffset := sysoffB.SysTime.Sub(sysoffA.SysTime)
	// compensate difference between PHC time by difference in system time
	phcOffset := sysoffB.PHCTime.Sub(sysoffA.PHCTime) - sysOffset
	shortest := phcOffset
	// look for smallest difference between system time midpoints
	for i := 1; i < numProbes; i++ {
		sysoffA = sysoffFromExtendedTS(extendedA.Ts[i])
		sysoffB = sysoffFromExtendedTS(extendedB.Ts[i])
		sysOffset = sysoffB.SysTime.Sub(sysoffA.SysTime)
		phcOffset = sysoffB.PHCTime.Sub(sysoffA.PHCTime) - sysOffset

		if abs(phcOffset) < abs(shortest) {
			shortest = phcOffset
		}
	}
	return shortest, nil
}

// offsetBetweenPreciseReadings returns estimated difference between two PHC SYS_OFFSET_PRECISE readings
func offsetBetweenPreciseReadings(preciseA, preciseB *PTPSysOffsetPrecise) time.Duration {
	// calculate sys time midpoint from both samples
	sysoffA := SysoffFromPrecise(preciseA)
	sysoffB := SysoffFromPrecise(preciseB)
	// offset between sys time midpoints
	sysOffset := sysoffB.SysTime.Sub(sysoffA.SysTime)
	// compensate difference between PHC time by difference in system time
	phcOffset := sysoffB.PHCTime.Sub(sysoffA.PHCTime) - sysOffset
	return phcOffset
}

// OffsetBetweenDevices returns estimated difference between two PHC devices
func OffsetBetweenDevices(deviceA, deviceB *os.File) (time.Duration, error) {
	adev, bdev := FromFile(deviceA), FromFile(deviceB)
	preciseA, err := adev.ReadSysoffPrecise()
	if err == nil {
		preciseB, err := bdev.ReadSysoffPrecise()
		if err != nil {
			return 0, err
		}
		return preciseB.Sub(preciseA), nil
	}
	extendedA, err := adev.ReadSysoffExtended()
	if err != nil {
		return 0, err
	}
	extendedB, err := bdev.ReadSysoffExtended()
	if err != nil {
		return 0, err
	}
	return extendedB.Sub(extendedA)
}
