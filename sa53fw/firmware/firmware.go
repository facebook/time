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

package firmware

import (
	"bytes"
	"fmt"
	"os"
	"strings"
)

// Firmware represents firmware file
type Firmware struct {
	filename string
	fd       *os.File
	size     int64
	fwMajor  int
	fwMinor  int
	fwPatch  int
	fwCommit string
}

const fwFooterFormat string = "?v=V%d.%d.%d.%8s"

// Open is to open firmware file and create structure
func Open(filename string) (*Firmware, error) {
	fd, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("cannot open firmware file: %w", err)
	}

	stat, err := fd.Stat()
	if err != nil {
		fd.Close()
		return nil, fmt.Errorf("cannot get firmware file info: %w", err)
	}
	f := &Firmware{
		filename: filename,
		fd:       fd,
		size:     stat.Size(),
	}
	return f, nil
}

// Size returns the size of the firmware file
func (f *Firmware) Size() int64 {
	return f.size
}

// Close is to close firmware file
func (f *Firmware) Close() {
	f.fd.Close()
}

func (f *Firmware) parseFooter(footer string) error {
	n, err := fmt.Sscanf(footer, fwFooterFormat, &f.fwMajor, &f.fwMinor, &f.fwPatch, &f.fwCommit)

	if n != 4 {
		return fmt.Errorf("wrong firmware footer format: %s", footer)
	}
	if err != nil {
		return err
	}
	return nil
}

// ParseVersion finds metadata in the end of the file and parses versions
func (f *Firmware) ParseVersion() error {
	var idx int64
	buf := make([]byte, 256)
	fwFooter := []byte{'?', 'v', '=', 'V'}

	for offset := f.size - 256; offset >= 0; offset -= 256 {
		if _, err := f.fd.ReadAt(buf, offset); err != nil {
			return err
		}

		idx = int64(bytes.Index(buf, fwFooter))
		if idx != -1 {
			idx += offset
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("cannot find firmware footer")
	}

	if _, err := f.fd.ReadAt(buf, idx); err != nil {
		return err
	}
	if _, err := f.fd.Seek(0, 0); err != nil {
		return err
	}

	b := strings.Builder{}
	b.Write(buf)
	err := f.parseFooter(b.String())

	return err
}

// FormatFWVersion return string representation of firmware version
func (f *Firmware) FormatFWVersion() string {
	return fmt.Sprintf("V%d.%d.%d", f.fwMajor, f.fwMinor, f.fwPatch)
}

// Version returns firmware as an int value
func (f *Firmware) Version() int {
	return f.fwMajor*0x10000 + f.fwMinor*0x100 + f.fwPatch
}

// Read implements reader interface
func (f *Firmware) Read(b []byte) (int, error) {
	return f.fd.Read(b)
}
