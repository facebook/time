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
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeConn gives us fake io.ReadWriter interacted implementation for which we can provide fake outputs.
// writeErr/readErr/readZero short-circuit before the outputs queue is consulted.
type fakeConn struct {
	readCount int
	outputs   []*bytes.Buffer
	// writeErr, when non-nil, is returned from Write instead of (0, nil).
	writeErr error
	// readErr, when non-nil, is returned from Read (with n=0) instead of consuming outputs.
	readErr error
	// readZero, when true, makes Read return (0, nil) instead of the default EOF fallback.
	readZero bool
}

func newConn(outputs []*bytes.Buffer) *fakeConn {
	return &fakeConn{
		readCount: 0,
		outputs:   outputs,
	}
}

func (c *fakeConn) Read(p []byte) (n int, err error) {
	if c.readErr != nil {
		return 0, c.readErr
	}
	if c.readZero {
		return 0, nil
	}
	pos := c.readCount
	if c.readCount < len(c.outputs) {
		c.readCount++
		return c.outputs[pos].Read(p)
	}
	return 0, fmt.Errorf("EOF")
}

func (c *fakeConn) Write(p []byte) (n int, err error) {
	if c.writeErr != nil {
		return 0, c.writeErr
	}
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
		Reply:    RpyTracking,
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
		Reply:    RpyTracking,
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
		IPAddr:             IPAddr{IP: IPToBytes(net.ParseIP("192.168.0.10")), Family: IPAddrInet4},
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

// TestCommunicateWriteError verifies that a Write failure on the underlying
// connection is wrapped with a descriptive error and preserves the %w chain.
func TestCommunicateWriteError(t *testing.T) {
	sentinel := errors.New("network is down")
	conn := &fakeConn{writeErr: sentinel}
	client := Client{Sequence: 1, Connection: conn}
	_, err := client.Communicate(NewTrackingPacket())
	require.Error(t, err)
	require.ErrorIs(t, err, sentinel)
	require.ErrorContains(t, err, "failed to write packet to connection")
}

// TestCommunicateReadError verifies that a non-EOF Read failure on the underlying
// connection is wrapped with a descriptive error and preserves the %w chain.
func TestCommunicateReadError(t *testing.T) {
	sentinel := errors.New("connection reset by peer")
	conn := &fakeConn{readErr: sentinel}
	client := Client{Sequence: 1, Connection: conn}
	_, err := client.Communicate(NewTrackingPacket())
	require.Error(t, err)
	require.ErrorIs(t, err, sentinel)
	require.ErrorContains(t, err, "connection.Read failed")
}

// TestCommunicateNoData verifies that a successful read of zero bytes (no error)
// produces the "no data received" error rather than attempting to decode.
func TestCommunicateNoData(t *testing.T) {
	conn := &fakeConn{readZero: true}
	client := Client{Sequence: 1, Connection: conn}
	_, err := client.Communicate(NewTrackingPacket())
	require.Error(t, err)
	require.ErrorContains(t, err, "no data received")
}
