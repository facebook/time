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
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"time"
)

// leapFile is a file containing leap second information
var leapFile = "/usr/share/zoneinfo/right/UTC"

var errBadData = errors.New("malformed time zone information")
var errUnsupportedVersion = errors.New("unsupported version")
var errNoLeapSeconds = errors.New("no leap seconds information found")

// LeapSecond represents a leap second
type LeapSecond struct {
	Tleap uint64
	Nleap int32
}

// Header represents file header structure. Fields names are copied from doc
type Header struct {
	// A four-octet unsigned integer specifying the number of UTC/local indicators contained in the body.
	IsUtcCnt uint32
	// A four-octet unsigned integer specifying the number of standard/wall indicators contained in the body.
	IsStdCnt uint32
	// A four-octet unsigned integer specifying the number of leap second records contained in the body.
	LeapCnt uint32
	// A four-octet unsigned integer specifying the number of transition times contained in the body.
	TimeCnt uint32
	// A four-octet unsigned integer specifying the number of local time type Records contained in the body - MUST NOT be zero.
	TypeCnt uint32
	// A four-octet unsigned integer specifying the total number of octets used by the set of time zone designations contained in the body.
	CharCnt uint32
}

// Time returns when the leap second event occurs
func (l LeapSecond) Time() time.Time {
	return time.Unix(int64(l.Tleap-uint64(l.Nleap)+1), 0)
}

// Parse returns the list of leap seconds from srcfile. Pass "" to use default file
func Parse(srcfile string) ([]LeapSecond, error) {
	if srcfile == "" {
		srcfile = leapFile
	}
	f, err := os.Open(srcfile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return parseVx(f)
}

// Latest returns the latest leap second from srcfile. Pass "" to use default file
func Latest(srcfile string) (*LeapSecond, error) {
	res := LeapSecond{}
	leapSeconds, err := Parse(srcfile)
	if err != nil {
		return nil, err
	}

	for _, leapSecond := range leapSeconds {
		if leapSecond.Time().After(res.Time()) && leapSecond.Time().Before(time.Now()) {
			res = leapSecond
		}
	}

	return &res, nil
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
			return nil, errUnsupportedVersion
		}

		if v > version {
			return nil, errBadData
		}

		var hdr Header
		err := binary.Read(r, binary.BigEndian, &hdr)
		if err != nil {
			return nil, err
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
			skip = int(hdr.TimeCnt)*5 + int(hdr.TypeCnt)*6 + int(hdr.CharCnt)
		} else {
			skip = int(hdr.TimeCnt)*9 + int(hdr.TypeCnt)*6 + int(hdr.CharCnt)
		}

		// if it's first part of two parts file (version 2 or 3)
		// then skip it completely
		if v == 0 && version > 0 {
			skip += int(hdr.LeapCnt)*8 + int(hdr.IsUtcCnt) + int(hdr.IsStdCnt)
		}

		if n, _ := io.CopyN(io.Discard, r, int64(skip)); n != int64(skip) {
			return nil, errBadData
		}

		if v == 0 && version > 0 {
			continue
		}

		// calculate the amount of bytes to skip after reading leap seconds array
		skip = int(hdr.IsUtcCnt) + int(hdr.IsStdCnt)

		for i := 0; i < int(hdr.LeapCnt); i++ {
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
		_, _ = io.CopyN(io.Discard, r, int64(skip))
		break
	}
	if len(ret) == 0 {
		return nil, errNoLeapSeconds
	}

	return ret, nil
}

func prepareHeader(ver byte, lsCnt int, name string) []byte {
	const magicHeader = "TZif"

	h := new(bytes.Buffer)

	hdr := Header{
		IsUtcCnt: 1,
		IsStdCnt: 1,
		LeapCnt:  uint32(lsCnt),
		TimeCnt:  0,
		TypeCnt:  1, //mandatory >0
		CharCnt:  uint32(len(name)),
	}

	h.WriteString(magicHeader)
	h.WriteByte(ver)
	padding := make([]byte, 15)
	h.Write(padding)
	_ = binary.Write(h, binary.BigEndian, hdr)
	return h.Bytes()
}

func writePreData(f io.Writer, name string) error {
	// we have zero-sized transition times array - skip it

	// one mandatory "local time type Record"
	var sixZeros = []byte{0, 0, 0, 0, 0, 0}
	if _, err := f.Write(sixZeros); err != nil {
		return err
	}

	// null terminated string of time zone
	if _, err := io.WriteString(f, name); err != nil {
		return err
	}
	return nil
}

func writePostData(f io.Writer) error {
	var twoZeros = []byte{0, 0}
	if _, err := f.Write(twoZeros); err != nil {
		return err
	}
	return nil
}

// Write dumps arrays of leap seconds into file with newly created header
func Write(f io.Writer, ver byte, ls []LeapSecond, name string) error {
	if ver != 0 && ver != '2' {
		return errUnsupportedVersion
	}

	var nameFormatted string
	if name == "" {
		nameFormatted = "UTC\x00"
	} else {
		nameFormatted = name + "\x00"
	}

	// prepare header which will be reused in case of version 2
	hdr := prepareHeader(ver, len(ls), nameFormatted)

	// Write prepared header
	if _, err := f.Write(hdr); err != nil {
		return err
	}

	// data before array of leap seconds
	if err := writePreData(f, nameFormatted); err != nil {
		return err
	}

	// array of leap seconds
	for i := 0; i < len(ls); i++ {
		l := []uint32{uint32(ls[i].Tleap), uint32(ls[i].Nleap)}
		if err := binary.Write(f, binary.BigEndian, &l); err != nil {
			return err
		}
	}

	// write data after leap seconds array
	if err := writePostData(f); err != nil {
		return err
	}

	if ver != '2' {
		return nil
	}

	// now we have to write version 2 part of file
	// prepared header could be reused
	if _, err := f.Write(hdr); err != nil {
		return err
	}

	// data before array of leap seconds
	if err := writePreData(f, nameFormatted); err != nil {
		return err
	}

	// array of leap seconds version 2
	for i := 0; i < len(ls); i++ {
		l := ls[i]
		if err := binary.Write(f, binary.BigEndian, &l); err != nil {
			return err
		}
	}

	// write data after leap seconds array
	if err := writePostData(f); err != nil {
		return err
	}

	// and now we have to write POSIZ TZ string along with new line separators
	// usually it's the same string as in the header
	posixTz := "\n" + name + "\n"
	if _, err := io.WriteString(f, posixTz); err != nil {
		return err
	}

	return nil
}
