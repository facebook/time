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
	"net"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

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
			OriginTimestamp: ptp.NewTimestamp(time.Now()),
		},
	}
}

func syncPkt(seq int) *ptp.SyncDelayReq {
	l := binary.Size(ptp.SyncDelayReq{})
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
	mcfg := &MeasurementConfig{}
	statsServer := NewMockStatsServer(ctrl)
	c, err := newClient("127.0.0.1", cid, eventConn, mcfg, statsServer)
	require.NoError(t, err)

	// handle whatever client is sending over eventConn
	statsServer.EXPECT().UpdateCounterBy("sptp.portstats.rx.sync", int64(1))
	statsServer.EXPECT().UpdateCounterBy("sptp.portstats.rx.announce", int64(1))
	statsServer.EXPECT().UpdateCounterBy("sptp.portstats.tx.delay_req", int64(1))
	eventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any()).DoAndReturn(func(b []byte, _ net.Addr) (int, time.Time, error) {
		delayReq := &ptp.SyncDelayReq{}
		err := ptp.FromBytes(b, delayReq)
		require.Nil(t, err, "reading delayReq msg")

		sync := syncPkt(0)
		syncBytes, err := ptp.Bytes(sync)
		require.Nil(t, err)
		c.inChan <- &inPacket{
			data: syncBytes,
			ts:   time.Now(),
		}

		announce = announcePkt(0)
		announceBytes, err := ptp.Bytes(announce)
		require.Nil(t, err)
		c.inChan <- &inPacket{
			data: announceBytes,
		}

		return len(syncBytes), time.Now(), nil
	})

	ctx := context.Background()
	runResult := c.RunOnce(ctx, 100*time.Millisecond)
	require.NotNil(t, runResult)
	require.NoError(t, runResult.Error, "full client run should succeed")
	require.Equal(t, "127.0.0.1", runResult.Server, "run result should have correct server")
	require.NotNil(t, runResult.Measurement, "run result should have measurements")
	require.Equal(t, *announce, runResult.Measurement.Announce)
	require.NotEqual(t, 0, runResult.Measurement.Delay)
	require.NotEqual(t, 0, runResult.Measurement.ServerToClientDiff)
	require.NotEqual(t, 0, runResult.Measurement.ClientToServerDiff)
	require.False(t, runResult.Measurement.Timestamp.IsZero())
}

func TestClientTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cid := ptp.ClockIdentity(0xc42a1fffe6d7ca6)

	eventConn := NewMockUDPConnWithTS(ctrl)
	mcfg := &MeasurementConfig{}
	statsServer := NewMockStatsServer(ctrl)
	c, err := newClient("127.0.0.1", cid, eventConn, mcfg, statsServer)
	require.NoError(t, err)
	statsServer.EXPECT().UpdateCounterBy("sptp.portstats.tx.delay_req", int64(1))
	eventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any())

	ctx := context.Background()
	runResult := c.RunOnce(ctx, 100*time.Millisecond)
	require.NotNil(t, runResult)
	require.Error(t, runResult.Error, "full client run should fail")
}
