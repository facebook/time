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

package chrony

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

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
	// here we may assert writes
	return 0, nil
}

// Test if we have errors when there is nothing on the line to read
func TestCommunicateEOF(t *testing.T) {
	conn := newConn([]*bytes.Buffer{
		bytes.NewBuffer([]byte{}),
	})
	client := Client{Sequence: 1, Connection: conn}
	_, err := client.Communicate(NewTrackingPacket())
	require.Error(t, err)
}

func TestCommunicateError(t *testing.T) {
	var err error
	buf := &bytes.Buffer{}
	packetHead := ReplyHead{
		Version:  protoVersionNumber,
		PKTType:  pktTypeCmdReply,
		Res1:     0,
		Res2:     0,
		Command:  reqTracking,
		Reply:    rpyTracking,
		Status:   sttNoSuchSource,
		Pad1:     0,
		Pad2:     0,
		Pad3:     0,
		Sequence: 2,
		Pad4:     0,
		Pad5:     0,
	}
	packetBody := replyTrackingContent{}
	err = binary.Write(buf, binary.BigEndian, packetHead)
	require.NoError(t, err)
	err = binary.Write(buf, binary.BigEndian, packetBody)
	require.NoError(t, err)
	conn := newConn([]*bytes.Buffer{
		buf,
	})
	client := Client{Sequence: 1, Connection: conn}
	_, err = client.Communicate(NewTrackingPacket())
	require.Error(t, err)
}

// Test if we can read reply properly
func TestCommunicateOK(t *testing.T) {
	var err error
	buf := &bytes.Buffer{}
	packetHead := ReplyHead{
		Version:  protoVersionNumber,
		PKTType:  pktTypeCmdReply,
		Res1:     0,
		Res2:     0,
		Command:  reqTracking,
		Reply:    rpyTracking,
		Status:   sttSuccess,
		Pad1:     0,
		Pad2:     0,
		Pad3:     0,
		Sequence: 2,
		Pad4:     0,
		Pad5:     0,
	}
	packetBody := replyTrackingContent{
		RefID:              1,
		IPAddr:             *newIPAddr(net.IP([]byte{192, 168, 0, 10})),
		Stratum:            3,
		LeapStatus:         0,
		RefTime:            timeSpec{},
		CurrentCorrection:  0,
		LastOffset:         12345,
		RMSOffset:          0,
		FreqPPM:            0,
		ResidFreqPPM:       0,
		SkewPPM:            0,
		RootDelay:          0,
		RootDispersion:     0,
		LastUpdateInterval: 0,
	}
	err = binary.Write(buf, binary.BigEndian, packetHead)
	require.NoError(t, err)
	err = binary.Write(buf, binary.BigEndian, packetBody)
	require.NoError(t, err)
	conn := newConn([]*bytes.Buffer{
		buf,
	})
	client := Client{Sequence: 1, Connection: conn}
	p, err := client.Communicate(NewTrackingPacket())
	require.NoError(t, err)
	expected := &ReplyTracking{
		ReplyHead: packetHead,
		Tracking: Tracking{
			RefID:              packetBody.RefID,
			IPAddr:             net.IP([]byte{192, 168, 0, 10}),
			Stratum:            packetBody.Stratum,
			LeapStatus:         packetBody.LeapStatus,
			RefTime:            packetBody.RefTime.ToTime(),
			CurrentCorrection:  packetBody.CurrentCorrection.ToFloat(),
			LastOffset:         packetBody.LastOffset.ToFloat(),
			RMSOffset:          packetBody.RMSOffset.ToFloat(),
			FreqPPM:            packetBody.FreqPPM.ToFloat(),
			ResidFreqPPM:       packetBody.ResidFreqPPM.ToFloat(),
			SkewPPM:            packetBody.SkewPPM.ToFloat(),
			RootDelay:          packetBody.RootDelay.ToFloat(),
			RootDispersion:     packetBody.RootDispersion.ToFloat(),
			LastUpdateInterval: packetBody.LastUpdateInterval.ToFloat(),
		},
	}
	require.Equal(t, expected, p)
}
