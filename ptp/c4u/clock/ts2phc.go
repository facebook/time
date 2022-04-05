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

package clock

import (
	"time"

	"github.com/facebook/time/phc"
	ptp "github.com/facebook/time/ptp/protocol"
)

const (
	phcTimeCardPath = "/dev/ptp_tcard"
	phcNICPath      = "/dev/ptp0"
)

func ts2phc() (*ptp.ClockQuality, error) {
	c := &ptp.ClockQuality{}

	tcard, err := phc.TimeAndOffsetFromDevice(phcTimeCardPath, phc.MethodIoctlSysOffsetExtended)
	if err != nil {
		return nil, err
	}

	tnic, err := phc.TimeAndOffsetFromDevice(phcNICPath, phc.MethodIoctlSysOffsetExtended)
	if err != nil {
		return nil, err
	}

	sysOffset := tcard.SysTime.Sub(tnic.SysTime)
	phcOffset := tcard.PHCTime.Sub(tnic.PHCTime)
	phcOffset -= sysOffset
	if phcOffset < 0 {
		phcOffset *= -1
	}

	// https://datatracker.ietf.org/doc/html/rfc8173#section-7.6.2.4
	// https://datatracker.ietf.org/doc/html/rfc8173#section-7.6.2.5
	if phcOffset < 100*time.Nanosecond {
		c.ClockAccuracy = ptp.ClockAccuracyNanosecond100 // 100ns
	} else if phcOffset < time.Microsecond {
		c.ClockAccuracy = ptp.ClockAccuracyMicrosecond1 // 1us
	} else if phcOffset < 250*time.Microsecond {
		c.ClockAccuracy = ptp.ClockAccuracyMicrosecond250 // 250us
	} else {
		c.ClockAccuracy = ptp.ClockAccuracySecondGreater10 // >10 second
	}

	return c, nil
}
