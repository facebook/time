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
	"fmt"
	"io"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
)

// MonitoringPort is an oscillatord monitoring socket port
const MonitoringPort = 2958

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

// ReadStatus talks to oscillatord via monitoring port connection and reads reported Status
func ReadStatus(conn io.ReadWriter) (*Status, error) {
	// send newline to make oscillatord send us data
	_, err := conn.Write([]byte(`{}`))
	if err != nil {
		return nil, fmt.Errorf("writing to oscillatord conn: %w", err)
	}
	buf := make([]byte, 1000)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("reading from oscillatord conn: %w", err)
	}
	if n == 0 {
		return nil, fmt.Errorf("read 0 bytes from oscillatord")
	}
	var status Status
	if err := json.Unmarshal(buf[:n], &status); err != nil {
		return nil, fmt.Errorf("unmarshalling JSON: %w", err)
	}
	return &status, nil
}
