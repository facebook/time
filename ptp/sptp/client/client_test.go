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

package client

import (
	"context"
	"encoding/binary"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/sys/unix"

	ptp "github.com/facebook/time/ptp/protocol"
)

func announcePkt(seq int) *ptp.Announce {
	l := binary.Size(ptp.Header{}) + binary.Size(ptp.AnnounceBody{})
	return &ptp.Announce{
		Header: ptp.Header{
			SdoIDAndMsgType:    ptp.NewSdoIDAndMsgType(ptp.MessageAnnounce, 0),
			Version:            ptp.Version,
			SequenceID:         uint16(seq),
			MessageLength:      uint16(l),
			FlagField:          ptp.FlagUnicast,
			LogMessageInterval: 0x7f,
		},
		AnnounceBody: ptp.AnnounceBody{
			OriginTimestamp:     ptp.NewTimestamp(time.Now()),
			GrandmasterIdentity: 42,
		},
	}
}

func syncPkt(seq int) *ptp.SyncDelayReq {
	l := binary.Size(ptp.Header{}) + binary.Size(ptp.SyncDelayReqBody{}) //#nosec G115
	return &ptp.SyncDelayReq{
		Header: ptp.Header{
			SdoIDAndMsgType:    ptp.NewSdoIDAndMsgType(ptp.MessageSync, 0),
			Version:            ptp.Version,
			SequenceID:         uint16(seq),
			MessageLength:      uint16(l),
			FlagField:          ptp.FlagUnicast,
			LogMessageInterval: 0x7f,
		},
		SyncDelayReqBody: ptp.SyncDelayReqBody{
			OriginTimestamp: ptp.NewTimestamp(time.Now()),
		},
	}
}

func TestClientRun(t *testing.T) {
	var announce *ptp.Announce

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cid := ptp.ClockIdentity(0xc42a1fffe6d7ca6)

	eventConn := NewMockUDPConnWithTS(ctrl)
	cfg := Config{
		Measurement: MeasurementConfig{
			PathDelayFilterLength:         0,
			PathDelayFilter:               "",
			PathDelayDiscardFilterEnabled: false,
			PathDelayDiscardBelow:         0,
		},
	}
	statsServer := NewMockStatsServer(ctrl)
	c, err := NewClient(netip.MustParseAddr("127.0.0.1"), ptp.PortEvent, cid, eventConn, &cfg, statsServer)
	require.NoError(t, err)

	// put stuff into measurements to make sure it got cleaned before the run
	c.m.data[124] = &mData{
		t2: time.Now(),
	}

	// handle whatever client is sending over eventConn
	statsServer.EXPECT().IncTXDelayReq()
	// unexpected packet we just ignore
	eventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any()).DoAndReturn(func(b []byte, _ unix.Sockaddr) (int, time.Time, error) {
		delayReq := &ptp.SyncDelayReq{}
		err := ptp.FromBytes(b, delayReq)
		require.Nil(t, err, "reading delayReq msg")

		sync := syncPkt(int(delayReq.SequenceID))
		syncBytes, err := ptp.Bytes(sync)
		require.Nil(t, err)
		c.handleSync(sync, time.Now())
		c.inChan <- true
		// send in irrelevant packet client should ignore
		c.inChan <- true

		announce = announcePkt(int(delayReq.SequenceID))
		require.Nil(t, err)
		c.handleAnnounce(announce)
		c.inChan <- true

		return len(syncBytes), time.Now(), nil
	})

	ctx := context.Background()
	runResult := c.RunOnce(ctx, &Config{ExchangeTimeout: defaultTestTimeout})
	require.NotNil(t, runResult)
	require.NoError(t, runResult.Error, "full client run should succeed")
	require.Equal(t, netip.MustParseAddr("127.0.0.1"), runResult.Server, "run result should have correct server")
	require.NotNil(t, runResult.Measurement, "run result should have measurements")
	require.Equal(t, *announce, runResult.Measurement.Announce)
	require.NotEqual(t, 0, runResult.Measurement.Delay)
	require.NotEqual(t, 0, runResult.Measurement.S2CDelay)
	require.NotEqual(t, 0, runResult.Measurement.C2SDelay)
	require.False(t, runResult.Measurement.Timestamp.IsZero())
	// make sure only latest measurements are stored, none of the previous stuff
	require.Nil(t, c.m.data[123])
	require.Equal(t, 1, len(c.m.data))
}

func TestClientTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cid := ptp.ClockIdentity(0xc42a1fffe6d7ca6)

	eventConn := NewMockUDPConnWithTS(ctrl)
	cfg := Config{
		Measurement: MeasurementConfig{
			PathDelayFilterLength:         0,
			PathDelayFilter:               "",
			PathDelayDiscardFilterEnabled: false,
			PathDelayDiscardBelow:         0,
		},
	}
	statsServer := NewMockStatsServer(ctrl)
	c, err := NewClient(netip.MustParseAddr("127.0.0.1"), ptp.PortEvent, cid, eventConn, &cfg, statsServer)
	require.NoError(t, err)
	statsServer.EXPECT().IncTXDelayReq()
	eventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any())

	ctx := context.Background()
	runResult := c.RunOnce(ctx, &Config{ExchangeTimeout: defaultTestTimeout})
	require.NotNil(t, runResult)
	require.Error(t, runResult.Error, "full client run should fail")
}

func TestClientBadPacket(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cid := ptp.ClockIdentity(0xc42a1fffe6d7ca6)

	eventConn := NewMockUDPConnWithTS(ctrl)
	cfg := Config{
		Measurement: MeasurementConfig{
			PathDelayFilterLength:         0,
			PathDelayFilter:               "",
			PathDelayDiscardFilterEnabled: false,
			PathDelayDiscardBelow:         0,
		},
	}
	statsServer := NewMockStatsServer(ctrl)
	c, err := NewClient(netip.MustParseAddr("127.0.0.1"), ptp.PortEvent, cid, eventConn, &cfg, statsServer)
	require.NoError(t, err)

	// handle whatever client is sending over eventConn
	statsServer.EXPECT().IncTXDelayReq()
	eventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any()).DoAndReturn(func(b []byte, _ unix.Sockaddr) (int, time.Time, error) {
		delayReq := &ptp.SyncDelayReq{}
		err := ptp.FromBytes(b, delayReq)
		require.Nil(t, err, "reading delayReq msg")
		c.inChan <- true

		return 10, time.Now(), nil
	})

	ctx := context.Background()
	runResult := c.RunOnce(ctx, &Config{ExchangeTimeout: defaultTestTimeout})
	require.NotNil(t, runResult)
	require.Error(t, runResult.Error, "full client run should fail")
	require.Equal(t, netip.MustParseAddr("127.0.0.1"), runResult.Server, "run result should have correct server")
}

func TestClientIncrementSequence(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cid := ptp.ClockIdentity(0xc42a1fffe6d7ca6)

	eventConn := NewMockUDPConnWithTS(ctrl)
	cfg := Config{
		Measurement: MeasurementConfig{
			PathDelayFilterLength:         0,
			PathDelayFilter:               "",
			PathDelayDiscardFilterEnabled: false,
			PathDelayDiscardBelow:         0,
		},
		SequenceIDMaskBits:  2,
		SequenceIDMaskValue: 3,
	}
	statsServer := NewMockStatsServer(ctrl)
	c, err := NewClient(netip.MustParseAddr("127.0.0.1"), ptp.PortEvent, cid, eventConn, &cfg, statsServer)
	require.NoError(t, err)
	require.Equal(t, uint16(0x3FFF), c.sequenceIDMask)
	require.Equal(t, uint16(0xC000), c.sequenceIDValue)

	c.eventSequence = c.sequenceIDValue + uint16(1)
	c.incrementSequence()
	require.Equal(t, uint16(0xC002), c.eventSequence)
	c.eventSequence = uint16(0xFFFF)
	c.incrementSequence()
	require.Equal(t, uint16(0xC000), c.eventSequence)
}

func TestReqAnnounce(t *testing.T) {
	now := time.Now()
	a := ReqAnnounce(ptp.ClockIdentity(0xc42a1fffe6d7ca6), 1, now)
	require.Equal(t, now.Nanosecond(), a.OriginTimestamp.Time().Nanosecond())
}
