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
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

var vnMode = MakeVnMode(3, Mode)

// fakeConn gives us fake io.ReadWriter interacted implementation for which we can provide fake outputs
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
	// here we may require writes
	return 0, nil
}

// Test if we have errors when there is nothing on the line to read
func TestCommunicateEOF(t *testing.T) {
	conn := newConn([]*bytes.Buffer{
		bytes.NewBuffer([]byte{}),
	})
	client := NTPClient{Sequence: 1, Connection: conn}
	_, err := client.Communicate(&NTPControlMsgHead{
		VnMode: vnMode,
		REMOp:  OpReadStatus,
	})
	require.Error(t, err)
}

// Test if we can read single packet (more bit set to 0)
func TestCommunicateSingle(t *testing.T) {
	conn := newConn([]*bytes.Buffer{
		bytes.NewBuffer([]byte{
			0x1e, 0x81, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00,
		}),
	})
	client := NTPClient{Sequence: 1, Connection: conn}
	p, err := client.Communicate(&NTPControlMsgHead{
		VnMode: vnMode,
		REMOp:  OpReadStatus,
	})
	require.NoError(t, err)
	expected := &NTPControlMsg{
		NTPControlMsgHead{
			VnMode: 0x1E,
			REMOp:  0x81, // response bit is set to 1, more bit set to 0
		},
		[]byte{},
	}
	require.Equal(t, expected, p)
}

// Test if we can read split packet, when first has more bit set to 1
func TestCommunicateMulti(t *testing.T) {
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
		VnMode: vnMode,
		REMOp:  OpReadStatus,
	})
	require.NoError(t, err)
	expected := &NTPControlMsg{
		NTPControlMsgHead{
			VnMode: 0x1E,
			REMOp:  0x81, // response bit is set to 1, more bit set to 0 as we've read all of them
			Count:  2,
		},
		[]byte{0x74, 0x69, 0x6d, 0x65},
	}
	require.Equal(t, expected, p)
}

// Test that a response whose Count field exceeds the octets we actually
// received is rejected, instead of being padded out or read past the buffer
func TestCommunicateBadCount(t *testing.T) {
	tests := []struct {
		name      string
		responses [][]byte
		wantErr   string
	}{
		{
			name: "count larger than the read buffer",
			responses: [][]byte{{
				0x1e, 0x81, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x07, 0xd0, // count set to 2000
			}},
			wantErr: "response declares 2000 data octets, but only 0 were received",
		},
		{
			name: "count that wraps once the header size is added",
			responses: [][]byte{{
				0x1e, 0x81, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0xff, 0xff, // count set to 65535
			}},
			wantErr: "response declares 65535 data octets, but only 0 were received",
		},
		{
			name: "count larger than the data sent",
			responses: [][]byte{{
				0x1e, 0x81, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x04, // count set to 4
				0x74, 0x69, // only 2 octets of data
			}},
			wantErr: "response declares 4 data octets, but only 2 were received",
		},
		{
			name: "truncated header",
			responses: [][]byte{{
				0x1e, 0x81, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
			}},
			wantErr: "truncated response: got 8 bytes, want at least 12",
		},
		{
			name: "bad count in the second half of a split response",
			responses: [][]byte{
				{
					0x1e, 0xa1, 0x00, 0x00, // more bit set to 1
					0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x02, // count set to 2
					0x74, 0x69, // 2 octets of data
				},
				{
					0x1e, 0x81, 0x00, 0x00, // more bit set to 0
					0x00, 0x00, 0x00, 0x00,
					0x00, 0x00, 0x00, 0x04, // count set to 4
					0x6d, 0x65, // only 2 octets of data
				},
			},
			wantErr: "response declares 4 data octets, but only 2 were received",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputs := make([]*bytes.Buffer, 0, len(tt.responses))
			for _, response := range tt.responses {
				outputs = append(outputs, bytes.NewBuffer(response))
			}
			client := NTPClient{Sequence: 1, Connection: newConn(outputs)}
			_, err := client.Communicate(&NTPControlMsgHead{
				VnMode: vnMode,
				REMOp:  OpReadStatus,
			})
			require.EqualError(t, err, tt.wantErr)
		})
	}
}

// Test that the octets ntpd pads the data section with, and the authenticator
// that can follow it, do not make a well-formed Count look too large
func TestCommunicatePaddedResponse(t *testing.T) {
	response := []byte{
		0x1e, 0x81, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x05, // count set to 5
		0x74, 0x69, 0x6d, 0x65, 0x73, // 5 octets of data
		0x00, 0x00, 0x00, // padded up to a 4 octet boundary
		0x00, 0x00, 0x00, 0x01, // key id
		0x93, 0xdf, 0x1f, 0xf4, // 16 octet message digest
		0x81, 0x2b, 0x0e, 0x36,
		0x39, 0x4b, 0xd1, 0x4c,
		0x7f, 0xa8, 0x62, 0x05,
	}
	client := NTPClient{Sequence: 1, Connection: newConn([]*bytes.Buffer{bytes.NewBuffer(response)})}
	p, err := client.Communicate(&NTPControlMsgHead{
		VnMode: vnMode,
		REMOp:  OpReadStatus,
	})
	require.NoError(t, err)
	require.Equal(t, []byte{0x74, 0x69, 0x6d, 0x65, 0x73}, p.Data)
}

// headSizeBytes slices the header off the wire, so it has to keep matching the
// struct binary.Read fills from those same bytes
func TestHeadSizeBytes(t *testing.T) {
	require.Equal(t, headSizeBytes, binary.Size(NTPControlMsgHead{}))
}
