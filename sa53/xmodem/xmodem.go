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

package xmodem

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	cSOH    byte = 0x01
	cSTX    byte = 0x02
	cEOT    byte = 0x04
	cACK    byte = 0x06
	cNAK    byte = 0x15
	cPOLL   byte = 0x43
	cCPMEOF byte = 0x1A

	//XModem1KBlockSsize is the size of the block that must be provided
	XModem1KBlockSsize uint16 = 1024
)

// CRC16 calculates CRC16 for data frame
func CRC16(data []byte) uint16 {
	var u16CRC uint16

	for _, character := range data {
		part := uint16(character)

		u16CRC = u16CRC ^ (part << 8)
		for range 8 {
			if u16CRC&0x8000 > 0 {
				u16CRC = u16CRC<<1 ^ 0x1021
			} else {
				u16CRC = u16CRC << 1
			}
		}
	}

	return u16CRC
}

// SendBlock1K is used to send 1K data block over XModem
func SendBlock1K(p io.ReadWriter, block uint8, data []byte, size uint16) error {
	try := 10
	if len(data) != int(XModem1KBlockSsize) {
		return fmt.Errorf("data block should be = 1024 bytes")
	}

	for ; size < XModem1KBlockSsize; size++ {
		data[size] = cCPMEOF
	}

	for ; try > 0; try-- {
		if _, err := p.Write([]byte{cSTX}); err != nil {
			return err
		}
		if _, err := p.Write([]byte{block}); err != nil {
			return err
		}
		if _, err := p.Write([]byte{255 - block}); err != nil {
			return err
		}
		if _, err := p.Write(data); err != nil {
			return err
		}
		uCRC16 := CRC16(data)
		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, uCRC16)
		if _, err := p.Write(b); err != nil {
			return err
		}

		_, err := p.Read(b)
		if err != nil {
			return err
		}
		if b[0] == cACK {
			break
		}
	}

	if try == 0 {
		return fmt.Errorf("block NACKed")
	}
	return nil
}

// SendEOT is used to finish the transmission of the data
func SendEOT(p io.ReadWriter) error {
	_, err := p.Write([]byte{cEOT})
	if err != nil {
		return err
	}
	b := make([]byte, 1)
	_, err = p.Read(b)
	if err != nil {
		return err
	}
	if b[0] == cNAK {
		return fmt.Errorf("EOT was NACKed")
	}
	return nil
}
