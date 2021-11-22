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

package leapsectz

import (
	"bytes"
	"testing"
)

func TestParse(t *testing.T) {
	byteData := []byte{
		'T', 'Z', 'i', 'f', // magic
		0x00, 0x00, 0x00, 0x00, // version
		0x00, 0x00, 0x00, 0x00, // pad
		0x00, 0x00, 0x00, 0x00, // pad
		0x00, 0x00, 0x00, 0x00, // pad
		0x00, 0x00, 0x00, 0x00, // UTC/local
		0x00, 0x00, 0x00, 0x00, // standard/wall
		0x00, 0x00, 0x00, 0x01, // leap
		0x00, 0x00, 0x00, 0x00, // transition
		0x00, 0x00, 0x00, 0x00, // local tz
		0x00, 0x00, 0x00, 0x00, // characters
		0x04, 0xb2, 0x58, 0x00, // leap time
		0x00, 0x00, 0x00, 0x01, // leap count
	}

	r := bytes.NewReader(byteData)

	ls, err := parseVx(r)
	if err != nil {
		t.Error(err)
	}

	if len(ls) != 1 {
		t.Errorf("wrong leap second list length")
	}

	// Saturday, July 1, 1972 12:00:00 AM
	if ls[0].Tleap != 78796800 {
		t.Errorf("wrong leap second time")
	}

	if ls[0].Nleap != 1 {
		t.Errorf("wrong leap second count")
	}
}

func TestParseV2(t *testing.T) {
	byteData := []byte{
		'T', 'Z', 'i', 'f', // magic
		'2', 0x00, 0x00, 0x00, // version
		0x00, 0x00, 0x00, 0x00, // pad
		0x00, 0x00, 0x00, 0x00, // pad
		0x00, 0x00, 0x00, 0x00, // pad
		0x00, 0x00, 0x00, 0x00, // UTC/local
		0x00, 0x00, 0x00, 0x00, // standard/wall
		0x00, 0x00, 0x00, 0x02, // leap
		0x00, 0x00, 0x00, 0x00, // transition
		0x00, 0x00, 0x00, 0x00, // local tz
		0x00, 0x00, 0x00, 0x00, // characters
		0x04, 0xb2, 0x58, 0x00, // leap time
		0x00, 0x00, 0x00, 0x01, // leap count
		0x05, 0xa4, 0xec, 0x01, // leap time
		0x00, 0x00, 0x00, 0x02, // leap count
		'T', 'Z', 'i', 'f', // magic
		'2', 0x00, 0x00, 0x00, // version
		0x00, 0x00, 0x00, 0x00, // pad
		0x00, 0x00, 0x00, 0x00, // pad
		0x00, 0x00, 0x00, 0x00, // pad
		0x00, 0x00, 0x00, 0x00, // UTC/local
		0x00, 0x00, 0x00, 0x00, // standard/wall
		0x00, 0x00, 0x00, 0x02, // leap
		0x00, 0x00, 0x00, 0x00, // transition
		0x00, 0x00, 0x00, 0x00, // local tz
		0x00, 0x00, 0x00, 0x00, // characters
		0x00, 0x00, 0x00, 0x00, // leap time (first 32 bits)
		0x04, 0xb2, 0x58, 0x00, // leap time (last 32 bits)
		0x00, 0x00, 0x00, 0x01, // leap count
		0x00, 0x00, 0x00, 0x00, // leap time (first 32 bits)
		0x05, 0xa4, 0xec, 0x01, // leap time (last 32 bits)
		0x00, 0x00, 0x00, 0x02, // leap count
	}

	r := bytes.NewReader(byteData)

	ls, e := parseVx(r)
	if e != nil {
		t.Error(e)
	}

	if len(ls) != 2 {
		t.Errorf("wrong leap second list length")
	}

	// Saturday, July 1, 1972 12:00:00 AM
	if ls[0].Tleap != 78796800 {
		t.Errorf("wrong leap second time")
	}

	if ls[0].Nleap != 1 {
		t.Errorf("wrong leap second count")
	}
	// January 1, 1973 12:00:00 AM
	if ls[1].Tleap != 94694401 {
		t.Errorf("wrong leap second time element 2")
	}

	if ls[1].Nleap != 2 {
		t.Errorf("wrong leap second count in elemen 2")
	}
}

func TestPrepareHeader(t *testing.T) {
	byteData := []byte{
		'T', 'Z', 'i', 'f', // magic
		'2', 0x00, 0x00, 0x00, // version
		0x00, 0x00, 0x00, 0x00, // pad
		0x00, 0x00, 0x00, 0x00, // pad
		0x00, 0x00, 0x00, 0x00, // pad
		0x00, 0x00, 0x00, 0x01, // UTC/local
		0x00, 0x00, 0x00, 0x01, // standard/wall
		0x00, 0x00, 0x00, 0x01, // leap
		0x00, 0x00, 0x00, 0x00, // transition
		0x00, 0x00, 0x00, 0x01, // local tz
		0x00, 0x00, 0x00, 0x04, // characters
	}

	hdr := prepareHeader('2', 1, "UTC\x00")

	if !bytes.Equal(hdr, byteData) {
		t.Errorf("wrong header")
	}
}

func TestWritePreData(t *testing.T) {
	byteData := []byte{
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 'U', 'T',
		'C', 0x00,
	}
	var b bytes.Buffer
	if err := writePreData(&b, "UTC\x00"); err != nil {
		t.Error(err)
	}

	if !bytes.Equal(b.Bytes(), byteData) {
		t.Errorf("wrong pre-leapseconds data")
	}
}

func TestWritePostData(t *testing.T) {
	byteData := []byte{0x00, 0x00}
	var b bytes.Buffer
	if err := writePostData(&b); err != nil {
		t.Error(err)
	}

	if !bytes.Equal(b.Bytes(), byteData) {
		t.Errorf("wrong post-leapseconds data")
	}
}
