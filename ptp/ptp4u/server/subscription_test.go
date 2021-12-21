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

package server

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/timestamp"

	"github.com/stretchr/testify/require"
)

func TestRunning(t *testing.T) {
	sc := SubscriptionClient{}
	// Initially subscription is not running (expire time is in the past)
	require.True(t, sc.Expired())

	// Add proper actual expiration time subscription
	sc.setExpire(time.Now().Add(1 * time.Second))
	require.False(t, sc.Expired())

	// Check running status
	require.False(t, sc.Running())
	sc.setRunning(true)
	require.True(t, sc.Running())
}

func TestSubscriptionStart(t *testing.T) {
	w := &sendWorker{}
	c := &Config{clockIdentity: ptp.ClockIdentity(1234)}
	interval := 1 * time.Minute
	expire := time.Now().Add(1 * time.Minute)
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, sa, sa, ptp.MessageAnnounce, c, interval, expire)

	go sc.Start()
	time.Sleep(100 * time.Millisecond)
	require.False(t, sc.Expired())
	require.True(t, sc.Running())
}

func TestSubscriptionExpire(t *testing.T) {
	w := &sendWorker{}
	c := &Config{clockIdentity: ptp.ClockIdentity(1234)}
	interval := 10 * time.Millisecond
	expire := time.Now().Add(200 * time.Millisecond)
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, sa, sa, ptp.MessageDelayResp, c, interval, expire)

	go sc.Start()
	time.Sleep(100 * time.Millisecond)

	require.False(t, sc.Expired())
	require.True(t, sc.Running())

	// Wait to expire
	time.Sleep(110 * time.Millisecond)
	require.True(t, sc.Expired())
	require.False(t, sc.Running())
}

func TestSubscriptionStop(t *testing.T) {
	w := &sendWorker{
		queue: make(chan *SubscriptionClient, 100),
	}
	c := &Config{clockIdentity: ptp.ClockIdentity(1234)}
	interval := 10 * time.Millisecond
	expire := time.Now().Add(1 * time.Second)
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, sa, sa, ptp.MessageAnnounce, c, interval, expire)

	go sc.Start()
	time.Sleep(100 * time.Millisecond)
	require.False(t, sc.Expired())
	sc.Stop()
	require.True(t, sc.Expired())
}

func TestSubscriptionflags(t *testing.T) {
	w := &sendWorker{}
	c := &Config{clockIdentity: ptp.ClockIdentity(1234)}
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, sa, sa, ptp.MessageAnnounce, c, time.Second, time.Time{})

	sc.UpdateSync()
	sc.UpdateFollowup(time.Now())
	sc.UpdateAnnounce()
	require.Equal(t, ptp.FlagUnicast|ptp.FlagTwoStep, sc.Sync().Header.FlagField)
	require.Equal(t, ptp.FlagUnicast, sc.Followup().Header.FlagField)
	require.Equal(t, ptp.FlagUnicast|ptp.FlagPTPTimescale, sc.Announce().Header.FlagField)
}

func TestSyncPacket(t *testing.T) {
	sequenceID := uint16(42)

	w := &sendWorker{}
	c := &Config{clockIdentity: ptp.ClockIdentity(1234)}
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, sa, sa, ptp.MessageAnnounce, c, time.Second, time.Time{})
	sc.sequenceID = sequenceID

	sc.initSync()
	sc.IncSequenceID()
	sc.UpdateSync()
	require.Equal(t, uint16(sequenceID+1), sc.Sync().Header.SequenceID)
}

func TestFollowupPacket(t *testing.T) {
	sequenceID := uint16(42)
	now := time.Now()
	interval := 3 * time.Second

	w := &sendWorker{}
	c := &Config{clockIdentity: ptp.ClockIdentity(1234)}
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, sa, sa, ptp.MessageAnnounce, c, time.Second, time.Time{})
	sc.sequenceID = sequenceID
	sc.setInterval(interval)

	i, err := ptp.NewLogInterval(interval)
	require.NoError(t, err)

	sc.initFollowup()
	sc.IncSequenceID()
	sc.UpdateFollowup(now)
	require.Equal(t, sequenceID+1, sc.Followup().Header.SequenceID)
	require.Equal(t, i, sc.Followup().Header.LogMessageInterval)
	require.Equal(t, now.Unix(), sc.Followup().FollowUpBody.PreciseOriginTimestamp.Time().Unix())
}

func TestAnnouncePacket(t *testing.T) {
	UTCOffset := 3 * time.Second
	sequenceID := uint16(42)
	interval := 3 * time.Second

	w := &sendWorker{}
	c := &Config{clockIdentity: ptp.ClockIdentity(1234), UTCOffset: UTCOffset}
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, sa, sa, ptp.MessageAnnounce, c, time.Second, time.Time{})
	sc.sequenceID = sequenceID
	sc.setInterval(interval)

	i, err := ptp.NewLogInterval(interval)
	require.NoError(t, err)

	sp := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: ptp.ClockIdentity(1234),
	}

	sc.initAnnounce()
	sc.IncSequenceID()
	sc.UpdateAnnounce()
	require.Equal(t, sequenceID+1, sc.Announce().Header.SequenceID)
	require.Equal(t, sp, sc.Announce().Header.SourcePortIdentity)
	require.Equal(t, i, sc.Announce().Header.LogMessageInterval)
	require.Equal(t, int16(UTCOffset.Seconds()), sc.Announce().AnnounceBody.CurrentUTCOffset)
}

func TestDelayRespPacket(t *testing.T) {
	sequenceID := uint16(42)
	now := time.Now()

	w := &sendWorker{}
	c := &Config{clockIdentity: ptp.ClockIdentity(1234)}
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, sa, sa, ptp.MessageAnnounce, c, time.Second, time.Time{})

	sp := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: ptp.ClockIdentity(1234),
	}
	h := &ptp.Header{
		SequenceID:         sequenceID,
		CorrectionField:    ptp.NewCorrection(100500),
		SourcePortIdentity: sp,
	}

	sc.initDelayResp()
	sc.UpdateDelayResp(h, now)
	require.Equal(t, sequenceID, sc.DelayResp().Header.SequenceID)
	require.Equal(t, 100500, int(sc.DelayResp().Header.CorrectionField.Nanoseconds()))
	require.Equal(t, sp, sc.DelayResp().Header.SourcePortIdentity)
	require.Equal(t, now.Unix(), sc.DelayResp().DelayRespBody.ReceiveTimestamp.Time().Unix())
	require.Equal(t, ptp.FlagUnicast, sc.DelayResp().Header.FlagField)
}

func TestGrantPacket(t *testing.T) {
	interval := 3 * time.Second

	w := &sendWorker{}
	c := &Config{clockIdentity: ptp.ClockIdentity(1234)}
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, sa, sa, ptp.MessageAnnounce, c, time.Second, time.Time{})
	sg := &ptp.Signaling{}

	mt := ptp.NewUnicastMsgTypeAndFlags(ptp.MessageAnnounce, 0)
	i, err := ptp.NewLogInterval(interval)
	require.NoError(t, err)
	duration := uint32(3)

	tlv := &ptp.GrantUnicastTransmissionTLV{
		TLVHead: ptp.TLVHead{
			TLVType:     ptp.TLVGrantUnicastTransmission,
			LengthField: uint16(binary.Size(ptp.GrantUnicastTransmissionTLV{}) - binary.Size(ptp.TLVHead{})),
		},
		MsgTypeAndReserved:    mt,
		LogInterMessagePeriod: i,
		DurationField:         duration,
		Reserved:              0,
		Renewal:               1,
	}

	sc.initGrant()
	sc.UpdateGrant(sg, mt, i, duration)

	require.Equal(t, tlv, sc.Grant().TLVs[0])

}
