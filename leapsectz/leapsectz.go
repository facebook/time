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
	"io/ioutil"
	"os"
	"time"
)

const file = "/usr/share/zoneinfo/right/UTC"

var errBadData = errors.New("malformed time zone information")
var errBadVersion = errors.New("version in file is not supported")

// LeapSecond represents a leap second
type LeapSecond struct {
	Tleap uint64
	Nleap int32
}

// Time returns when the leap second event occurs
func (l LeapSecond) Time() time.Time {
	return time.Unix(int64(l.Tleap-uint64(l.Nleap)+1), 0)
}

// Parse returns the list of leap seconds
func Parse() ([]LeapSecond, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return parseVx(f)

}

func parseVx(r io.Reader) ([]LeapSecond, error) {
	var ret []LeapSecond
	var v byte
	for v = 0; v < 2; v++ {
		// 4-byte magic "TZif"
		magic := make([]byte, 4)
		if _, _ = r.Read(magic); string(magic) != "TZif" {
			return nil, errBadData
		}

		// 1-byte version, then 15 bytes of padding
		var version byte
		p := make([]byte, 16)
		if n, _ := r.Read(p); n != 16 {
			return nil, errBadData
		}

		version = p[0]
		if version != 0 && version != '2' && version != '3' {
			return nil, errBadVersion
		}

		if v > version {
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
		//  tzh_timecnt (char [4] or char [8] for ver 2)s  coded transition times a la time(2)
		//  tzh_timecnt (unsigned char)s types of local time starting at above
		//  tzh_typecnt repetitions of
		//   one (char [4])  coded UT offset in seconds
		//   one (unsigned char) used to set tm_isdst
		//   one (unsigned char) that's an abbreviation list index
		//  tzh_charcnt (char)s  '\0'-terminated zone abbreviations
		var skip int
		if v == 0 {
			skip = n[NTime]*5 + n[NZone]*6 + n[NChar]
		} else {
			skip = n[NTime]*9 + n[NZone]*6 + n[NChar]
		}

		// if it's first part of two parts file (version 2 or 3)
		// then skip it completely
		if v == 0 && version > 0 {
			skip += n[NLeap]*8 + n[NUTCLocal] + n[NStdWall]
		}

		if n, _ := io.CopyN(ioutil.Discard, r, int64(skip)); n != int64(skip) {
			return nil, errBadData
		}

		if v == 0 && version > 0 {
			v++
			continue
		}

		// calculate the amount of bytes to skip after reading leap seconds array
		skip = n[NUTCLocal] + n[NStdWall]

		for i := 0; i < int(n[NLeap]); i++ {
			var l LeapSecond
			if version == 0 {
				lsv0 := []uint32{0, 0}
				err := binary.Read(r, binary.BigEndian, &lsv0)
				if err != nil {
					return nil, err
				}
				l.Tleap = uint64(lsv0[0])
				l.Nleap = int32(lsv0[1])
			} else {
				err := binary.Read(r, binary.BigEndian, &l)
				if err != nil {
					return nil, err
				}
			}
			ret = append(ret, l)
		}
		// we need to skip the rest of the data
		_, _ = io.CopyN(ioutil.Discard, r, int64(skip))
		break
	}
	return ret, nil
}
