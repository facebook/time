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

/*
Package oscillatord implements monitoring protocol used by Orolia's oscillatord,
daemon for disciplining an oscillator.

All references throughout the code relate to the https://github.com/Orolia2s/oscillatord code.
*/
package oscillatord

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
)

// MonitoringPort is an oscillatord monitoring socket port
const MonitoringPort = 2958

// maxStatusBytes bounds one monitoring reply. Status is a fixed three-object struct and a real
// reply is under 400 bytes, so this is only a backstop against a firmware that never stops talking.
const maxStatusBytes = 64 << 10

// AntennaStatus is an enum describing antenna status as reported by oscillatord
type AntennaStatus int

// from oscillatord src/gnss.c
const (
	AntStatusInit AntennaStatus = iota
	AntStatusDontKnow
	AntStatusOK
	AntStatusSHORT
	AntStatusOpen
	AntStatusUndefined
)

var antennaStatusToString = map[AntennaStatus]string{
	AntStatusInit:      "INIT",
	AntStatusDontKnow:  "DONTKNOW",
	AntStatusOK:        "OK",
	AntStatusSHORT:     "SHORT",
	AntStatusOpen:      "OPEN",
	AntStatusUndefined: "UNDEFINED",
}

func (a AntennaStatus) String() string {
	s, found := antennaStatusToString[a]
	if !found {
		return "UNSUPPORTED VALUE"
	}
	return s
}

// AntennaPower is an enum describing antenna power status as reported by oscillatord
type AntennaPower int

// from oscillatord src/gnss.c
const (
	AntPowerOff AntennaPower = iota
	AntPowerOn
	AntPowerDontKnow
	AntPowerIdle
	AntPowerUndefined
)

var antennaPowerToString = map[AntennaPower]string{
	AntPowerOff:       "OFF",
	AntPowerOn:        "ON",
	AntPowerDontKnow:  "DONTKNOW",
	AntPowerIdle:      "IDLE",
	AntPowerUndefined: "UNDEFINED",
}

func (p AntennaPower) String() string {
	s, found := antennaPowerToString[p]
	if !found {
		return "UNSUPPORTED VALUE"
	}
	return s
}

// GNSSFix is an enum describing GNSS fix status as reported by oscillatord
type GNSSFix int

// from oscillatord src/gnss.c
const (
	FixUnknown GNSSFix = iota
	FixNoFix
	FixDROnly
	FixTime
	Fix2D
	Fix3D
	Fix3DDr
	FixRTKFloat
	FixRTKFixed
	FixFloatDr
	FixFixedDr
)

var gnssFixToString = map[GNSSFix]string{
	FixUnknown:  "Unknown",
	FixNoFix:    "No fix",
	FixDROnly:   "DR only",
	FixTime:     "Time",
	Fix2D:       "2D",
	Fix3D:       "3D",
	Fix3DDr:     "3D_DR",
	FixRTKFloat: "RTK_FLOAT",
	FixRTKFixed: "RTK_FIXED",
	FixFloatDr:  "RTK_FLOAT_DR",
	FixFixedDr:  "RTK_FIXED_DR",
}

func (f GNSSFix) String() string {
	s, found := gnssFixToString[f]
	if !found {
		return "UNSUPPORTED VALUE"
	}
	return s
}

// LeapSecondChange is enum that oscillatord uses to indicate leap second change
type LeapSecondChange int

// from oscillatord src/gnss.c
const (
	LeapNoWarning LeapSecondChange = 0
	LeapAddSecond LeapSecondChange = 1
	LeapDelSecond LeapSecondChange = -1
)

var leapSecondChangeToString = map[LeapSecondChange]string{
	LeapNoWarning: "NO WARNING",
	LeapAddSecond: "ADD SECOND",
	LeapDelSecond: "DEL SECOND",
}

func (c LeapSecondChange) String() string {
	s, found := leapSecondChangeToString[c]
	if !found {
		return "UNSUPPORTED VALUE"
	}
	return s
}

// ClockClass is a oscillatord specific ClockClass
type ClockClass ptp.ClockClass

const (
	// ClockClassLock is an alias for ClockClass6
	ClockClassLock = ClockClass(ptp.ClockClass6)
	// ClockClassHoldover is an alias for ClockClass7
	ClockClassHoldover = ClockClass(ptp.ClockClass7)
	// ClockClassCalibrating is an alias for ClockClass13
	ClockClassCalibrating = ClockClass(ptp.ClockClass13)
	// ClockClassUncalibrated is an alias for ClockClass52
	ClockClassUncalibrated = ClockClass(ptp.ClockClass52)
)

// UnmarshalText parses ClockClass from a config string
func (c *ClockClass) UnmarshalText(text []byte) error {
	switch string(text) {
	case "Lock":
		*c = ClockClassLock
	case "Holdover":
		*c = ClockClassHoldover
	case "Calibrating":
		*c = ClockClassCalibrating
	case "Uncalibrated":
		*c = ClockClassUncalibrated
	default:
		return fmt.Errorf("clock class %s not supported", string(text))
	}

	return nil
}

var clockClassToString = map[ClockClass]string{
	ClockClassLock:         "Lock",
	ClockClassHoldover:     "Holdover",
	ClockClassCalibrating:  "Calibrating",
	ClockClassUncalibrated: "Uncalibrated",
}

// String representation of the ClockClass
func (c ClockClass) String() string {
	s, found := clockClassToString[c]
	if !found {
		return "UNSUPPORTED VALUE"
	}
	return s
}

// Oscillator describes structure that oscillatord returns for oscillator
type Oscillator struct {
	Model       string  `json:"model"`
	FwVersion   string  `json:"fw_version"`
	FineCtrl    int     `json:"fine_ctrl"`
	CoarseCtrl  int     `json:"coarse_ctrl"`
	Lock        bool    `json:"lock"`
	Temperature float64 `json:"temperature"`
}

// GNSS describes structure that oscillatord returns for gnss
type GNSS struct {
	Fix             GNSSFix          `json:"fix"`
	FixOK           bool             `json:"fixOk"`
	AntennaPower    AntennaPower     `json:"antenna_power"`
	AntennaStatus   AntennaStatus    `json:"antenna_status"`
	LSChange        LeapSecondChange `json:"lsChange"`
	LeapSeconds     int              `json:"leap_seconds"`
	SatellitesCount int              `json:"satellites_count"`
	TimeAccuracy    time.Duration    `json:"time_accuracy"`
}

// Clock describes structure that oscillatord returns for clock
type Clock struct {
	Class  ClockClass    `json:"class"`
	Offset time.Duration `json:"offset"`
}

// Status is whole structure that oscillatord returns for monitoring
type Status struct {
	Oscillator Oscillator `json:"oscillator"`
	GNSS       GNSS       `json:"gnss"`
	Clock      Clock      `json:"clock"`
}

// MonitoringJSON returns a json representation of status
func (s *Status) MonitoringJSON(prefix string) ([]byte, error) {
	if prefix != "" {
		prefix = fmt.Sprintf("%s.", prefix)
	}

	output := map[string]any{
		fmt.Sprintf("%soscillator.temperature", prefix):  s.Oscillator.Temperature,
		fmt.Sprintf("%soscillator.fine_ctrl", prefix):    int64(s.Oscillator.FineCtrl),
		fmt.Sprintf("%soscillator.coarse_ctrl", prefix):  int64(s.Oscillator.CoarseCtrl),
		fmt.Sprintf("%soscillator.lock", prefix):         bool2int(s.Oscillator.Lock),
		fmt.Sprintf("%soscillator.fw_version", prefix):   fwVersionToInt(s.Oscillator.FwVersion),
		fmt.Sprintf("%sgnss.fix_num", prefix):            int64(s.GNSS.Fix),
		fmt.Sprintf("%sgnss.fix_ok", prefix):             bool2int(s.GNSS.FixOK),
		fmt.Sprintf("%sgnss.antenna_power", prefix):      int64(s.GNSS.AntennaPower),
		fmt.Sprintf("%sgnss.antenna_status", prefix):     int64(s.GNSS.AntennaStatus),
		fmt.Sprintf("%sgnss.leap_second_change", prefix): int64(s.GNSS.LSChange),
		fmt.Sprintf("%sgnss.leap_seconds", prefix):       int64(s.GNSS.LeapSeconds),
		fmt.Sprintf("%sgnss.satellites_count", prefix):   int64(s.GNSS.SatellitesCount),
		fmt.Sprintf("%sgnss.time_accuracy_ns", prefix):   int64(s.GNSS.TimeAccuracy),
		fmt.Sprintf("%sclock.class", prefix):             int64(s.Clock.Class),
		fmt.Sprintf("%sclock.offset_ns", prefix):         int64(s.Clock.Offset),
	}
	return json.Marshal(output)
}

func bool2int(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// fwVersionToInt encodes an SA5x firmware version as major*10000 + minor*100 +
// patch, e.g. "V1.6.5.0.669A7202" -> 10605.
//
// The version has the format Vx.x.xx.0.XXXXXX, where the trailing ".0" is a
// constant and XXXXXX is a build/commit id (see time/sa53/protocol). Both are
// dropped, and each remaining field is assumed to be < 100.
func fwVersionToInt(version string) int64 {
	var major, minor, patch int
	if n, _ := fmt.Sscanf(version, "V%d.%d.%d", &major, &minor, &patch); n != 3 {
		return 0
	}
	return int64(major*10000 + minor*100 + patch)
}

// ReadStatus talks to oscillatord via monitoring port connection and reads reported Status.
// The response is decoded as a stream: it is not guaranteed to arrive in a single read, and
// its length is set by oscillatord's firmware rather than by anything we control.
func ReadStatus(conn io.ReadWriter) (*Status, error) {
	if _, err := conn.Write([]byte(`{}`)); err != nil {
		return nil, fmt.Errorf("writing to oscillatord conn: %w", err)
	}
	bounded := &io.LimitedReader{R: conn, N: maxStatusBytes}
	var status Status
	if err := json.NewDecoder(bounded).Decode(&status); err != nil {
		// The cap only explains the failure when the decoder ran out of stream. A malformed
		// reply can also exhaust the budget, and reporting that as a truncation hides it.
		if bounded.N <= 0 && (errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF)) {
			return nil, fmt.Errorf("oscillatord reply did not complete within %d bytes", maxStatusBytes)
		}
		return nil, fmt.Errorf("decoding JSON from oscillatord conn: %w", err)
	}
	return &status, nil
}
