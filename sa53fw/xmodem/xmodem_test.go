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
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockXModem struct {
	Buf *bytes.Buffer
}

func (p mockXModem) Write(b []byte) (n int, err error) {
	return p.Buf.Write(b)
}

func (p mockXModem) Read(b []byte) (n int, err error) {
	b[0] = 0x06
	return 1, nil
}

type mockXModemError struct {
	Buf *bytes.Buffer
}

func (p mockXModemError) Write(b []byte) (n int, err error) {
	return p.Buf.Write(b)
}

func (p mockXModemError) Read(b []byte) (n int, err error) {
	b[0] = 0x15
	return 1, nil
}

func TestCRC16(t *testing.T) {
	data := []byte{0x10, 0x20}

	crc := CRC16(data[:2])
	require.Equal(t, uint16(0x2711), crc)
}

func TestSend1K(t *testing.T) {
	xm := mockXModem{
		Buf: bytes.NewBuffer(make([]byte, 1029)),
	}
	xm.Buf.Reset()
	data := make([]byte, 1024)
	data[0] = 0x10
	data[1] = 0x20
	err := SendBlock1K(xm, 1, data, 2)
	require.NoError(t, err)

	res := xm.Buf.Bytes()
	// require STX value first
	require.Equal(t, uint8(0x02), res[0])
	// require block number
	require.Equal(t, uint8(0x01), res[1])
	// require (255 - block number)
	require.Equal(t, uint8(0xFE), res[2])
	// then goes data
	require.Equal(t, uint8(0x10), res[3])
	require.Equal(t, uint8(0x20), res[4])
	// check the rest to be fulfilled by CPMEOF byte
	for i := 5; i < 1027; i++ {
		require.Equal(t, uint8(0x1A), res[i])
	}
	// check for CRC16 in BigEndian
	require.Equal(t, uint8(0x80), res[1027])
	require.Equal(t, uint8(0xB3), res[1028])
}

func TestSendEOT(t *testing.T) {
	xm := mockXModem{
		Buf: bytes.NewBuffer(make([]byte, 2)),
	}
	xm.Buf.Reset()
	err := SendEOT(xm)
	require.NoError(t, err)
	res := xm.Buf.Bytes()
	// require EOT value first
	require.Equal(t, uint8(0x04), res[0])
	xme := mockXModemError{
		Buf: bytes.NewBuffer(make([]byte, 2)),
	}
	xme.Buf.Reset()
	err = SendEOT(xme)
	require.Error(t, err)
}
