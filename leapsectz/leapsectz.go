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

// Package leapsectz is a utility package for obtaining leap second
// information from the system timezone database

package leapsectz

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"time"
)

const file = "/usr/share/zoneinfo/right/UTC"

var errBadData = errors.New("malformed time zone information")

// LeapSecond represents a leap second
type LeapSecond struct {
	Tleap uint32
	Nleap uint32
}

// Time returns when the leap second event occurs
func (l LeapSecond) Time() time.Time {
	return time.Unix(int64(l.Tleap-l.Nleap+1), 0)
}

// Parse returns the list of leap seconds
func Parse() ([]LeapSecond, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return parse(f)
}

func parse(r io.Reader) ([]LeapSecond, error) {
	// 4-byte magic "TZif"
	magic := make([]byte, 4)
	if r.Read(magic); string(magic) != "TZif" {
		return nil, errBadData
	}

	// 1-byte version, then 15 bytes of padding
	p := make([]byte, 16)
	if n, _ := r.Read(p); n != 16 {
		return nil, errBadData
	} else if p[0] != 0 && p[0] != '2' && p[0] != '3' {
		return nil, errBadData
	}

	// six big-endian 32-bit integers:
	//	number of UTC/local indicators
	//	number of standard/wall indicators
	//	number of leap seconds
	//	number of transition times
	//	number of local time zones
	//	number of characters of time zone abbrev strings
	const (
		NUTCLocal = iota
		NStdWall
		NLeap
		NTime
		NZone
		NChar
	)
	var n [6]int
	for i := 0; i < 6; i++ {
		var nn uint32
		err := binary.Read(r, binary.BigEndian, &nn)
		if err != nil {
			return nil, err
		}
		n[i] = int(nn)
	}

	// skip uninteresting data:
	//  tzh_timecnt (char [4])s  coded transition times a la time(2)
	//  tzh_timecnt (unsigned char)s types of local time starting at above
	//  tzh_typecnt repetitions of
	//   one (char [4])  coded UT offset in seconds
	//   one (unsigned char) used to set tm_isdst
	//   one (unsigned char) that's an abbreviation list index
	//  tzh_charcnt (char)s  '\0'-terminated zone abbreviations
	skip := n[NTime]*5 + n[NZone]*6 + n[NChar]
	data := make([]byte, skip)
	if n, _ := r.Read(data); n != skip {
		return nil, errBadData
	}

	var ret []LeapSecond
	for i := 0; i < int(n[NLeap]); i++ {
		l := LeapSecond{}
		err := binary.Read(r, binary.BigEndian, &l)
		if err != nil {
			return nil, err
		}
		ret = append(ret, l)
	}

	return ret, nil
}
