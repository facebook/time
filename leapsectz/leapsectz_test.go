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
		'T', 'Z', 'i', 'f',     // magic
		'2', 0x00, 0x00, 0x00,  // version
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

	ls, err := parse(r)
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
