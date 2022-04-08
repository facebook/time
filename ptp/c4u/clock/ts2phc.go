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

	c.ClockAccuracy = ptp.ClockAccuracyFromOffset(phcOffset)

	return c, nil
}
