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

package control

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

// fakeConn gives us fake io.ReadWriter interace implementation for which we can provide fake outputs
type fakeConn struct {
	readCount int
	outputs   []*bytes.Buffer
}

func newConn(outputs []*bytes.Buffer) *fakeConn {
	return &fakeConn{
		readCount: 0,
		outputs:   outputs,
	}
}

func (c *fakeConn) Read(p []byte) (n int, err error) {
	pos := c.readCount
	if c.readCount < len(c.outputs) {
		c.readCount++
		return c.outputs[pos].Read(p)
	}
	return 0, fmt.Errorf("EOF")
}

func (c *fakeConn) Write(p []byte) (n int, err error) {
	// here we may assert writes
	return 0, nil
}

// Test if we have errors when there is nothing on the line to read
func TestCommunicateEOF(t *testing.T) {
	require := require.New(t)
	conn := newConn([]*bytes.Buffer{
		bytes.NewBuffer([]byte{}),
	})
	client := NTPClient{Sequence: 1, Connection: conn}
	_, err := client.Communicate(&NTPControlMsgHead{
		VnMode: 0x1E,
		REMOp:  0x01,
	})
	require.NotNil(err)
}

// Test if we can read single packet (more bit set to 0)
func TestCommunicateSingle(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	conn := newConn([]*bytes.Buffer{
		bytes.NewBuffer([]byte{
			0x1e, 0x81, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00,
		}),
	})
	client := NTPClient{Sequence: 1, Connection: conn}
	p, err := client.Communicate(&NTPControlMsgHead{
		VnMode: 0x1E,
		REMOp:  0x01,
	})
	require.Nil(err)
	expected := &NTPControlMsg{
		NTPControlMsgHead{
			VnMode: 0x1E,
			REMOp:  0x81, // response bit is set to 1, more bit set to 0
		},
		[]byte{},
	}
	assert.Equal(expected, p)
}

// Test if we can read split packet, when first has more bit set to 1
func TestCommunicateMulti(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	conn := newConn([]*bytes.Buffer{
		bytes.NewBuffer([]byte{
			0x1e, 0xa1, 0x00, 0x00, // more bit set to 1
			0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x02, // count set to 2
			0x74, 0x69, // 2 octets of data
		}),
		bytes.NewBuffer([]byte{
			0x1e, 0x81, 0x00, 0x00, // more bit set to 0
			0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x02, // count set to 2
			0x6d, 0x65, // 2 octets of data
		}),
	})
	client := NTPClient{Sequence: 1, Connection: conn}
	p, err := client.Communicate(&NTPControlMsgHead{
		VnMode: 0x1E,
		REMOp:  0x01,
	})
	require.Nil(err)
	expected := &NTPControlMsg{
		NTPControlMsgHead{
			VnMode: 0x1E,
			REMOp:  0x81, // response bit is set to 1, more bit set to 0 as we've read all of them
			Count:  2,
		},
		[]byte{0x74, 0x69, 0x6d, 0x65},
	}
	assert.Equal(expected, p)
}
