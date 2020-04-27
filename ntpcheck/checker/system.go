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

package checker

import (
	"strconv"

	"github.com/facebookincubator/ntp/protocol/control"
	"github.com/pkg/errors"
)

// SystemVariables holds System Variables extracted from k=v pairs, as described in http://doc.ntp.org/current-stable/ntpq.html
type SystemVariables struct {
	Version   string
	Processor string
	System    string
	Leap      int
	Stratum   int
	Precision int
	RootDelay float64
	RootDisp  float64
	Peer      int
	TC        int
	MinTC     int
	Clock     string
	RefID     string
	RefTime   string
	Offset    float64
	SysJitter float64
	Frequency float64
	ClkWander float64
	ClkJitter float64
	Tai       int
}

// sanityCheckSysVars checks if we parsed enough info from NTPD response
func sanityCheckSysVars(sysVars *SystemVariables) error {
	if sysVars == nil {
		return errors.New("No system variables")
	}
	if sysVars.Stratum == 0 {
		return errors.New("Incomplete data, stratum 0 in system variables")
	}
	return nil
}

// NewSystemVariables constructs System from NTPControlMsg packet
func NewSystemVariables(p *control.NTPControlMsg) (*SystemVariables, error) {
	m, err := p.GetAssociationInfo()
	if err != nil {
		return nil, err
	}
	// data comes as k=v pairs in packet, and those kv pairs are parsed by GetAssociationInfo.
	// If data is severely corrupted GetAssociationInfo will return error.
	// It's ok to have some fields missing, thus we don't check for errors below.
	leap, _ := strconv.Atoi(m["leap"])
	stratum, _ := strconv.Atoi(m["stratum"])
	precision, _ := strconv.Atoi(m["precision"])
	rootdelay, _ := strconv.ParseFloat(m["rootdelay"], 64)
	rootdisp, _ := strconv.ParseFloat(m["rootdisp"], 64)
	peer, _ := strconv.Atoi(m["peer"])
	tc, _ := strconv.Atoi(m["tc"])
	mintc, _ := strconv.Atoi(m["mintc"])
	offset, _ := strconv.ParseFloat(m["offset"], 64)
	sysjitter, _ := strconv.ParseFloat(m["sys_jitter"], 64)
	frequency, _ := strconv.ParseFloat(m["frequency"], 64)
	clkwander, _ := strconv.ParseFloat(m["clk_wander"], 64)
	clkjitter, _ := strconv.ParseFloat(m["clk_jitter"], 64)
	tai, _ := strconv.Atoi(m["tai"])

	system := SystemVariables{
		// from variables
		Version:   m["version"],
		Processor: m["processor"],
		System:    m["system"],
		Leap:      leap,
		Stratum:   stratum,
		Precision: precision,
		RootDelay: rootdelay,
		RootDisp:  rootdisp,
		Peer:      peer,
		TC:        tc,
		MinTC:     mintc,
		Clock:     m["clock"],
		RefID:     m["refid"],
		RefTime:   m["reftime"],
		Offset:    offset,
		SysJitter: sysjitter,
		Frequency: frequency,
		ClkWander: clkwander,
		ClkJitter: clkjitter,
		Tai:       tai,
	}
	// sometimes NTPD returns malformed k=v pairs and we can't parse important info
	if err := sanityCheckSysVars(&system); err != nil {
		return nil, err
	}
	return &system, nil
}
