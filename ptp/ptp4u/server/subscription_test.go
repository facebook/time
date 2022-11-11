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
	"context"
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
	sc.SetExpire(time.Now().Add(1 * time.Second))
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
	sc := NewSubscriptionClient(w.queue, w.signalingQueue, sa, nil, ptp.MessageAnnounce, c, interval, expire)
	sc.SetGclisa(sa)

	go sc.Start(context.Background())
	time.Sleep(100 * time.Millisecond)
	require.False(t, sc.Expired())
	require.True(t, sc.Running())
}

func TestSubscriptionExpire(t *testing.T) {
	w := &sendWorker{
		signalingQueue: make(chan *SubscriptionClient, 100),
	}
	c := &Config{clockIdentity: ptp.ClockIdentity(1234)}
	interval := 10 * time.Millisecond
	expire := time.Now().Add(200 * time.Millisecond)
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageDelayResp, c, interval, expire)

	go sc.Start(context.Background())
	time.Sleep(100 * time.Millisecond)

	require.False(t, sc.Expired())
	require.True(t, sc.Running())

	// Wait to expire
	time.Sleep(150 * time.Millisecond)
	require.True(t, sc.Expired())
	require.False(t, sc.Running())
}

func TestSubscriptionStop(t *testing.T) {
	w := &sendWorker{
		queue:          make(chan *SubscriptionClient, 100),
		signalingQueue: make(chan *SubscriptionClient, 100),
	}
	c := &Config{clockIdentity: ptp.ClockIdentity(1234)}
	interval := 32 * time.Second
	expire := time.Now().Add(1 * time.Minute)
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageAnnounce, c, interval, expire)

	go sc.Start(context.Background())
	time.Sleep(100 * time.Millisecond)
	require.False(t, sc.Expired())
	require.True(t, sc.Running())

	sc.Stop()
	time.Sleep(100 * time.Millisecond)

	require.True(t, sc.Expired())
	require.False(t, sc.Running())

	// No matter how many times we run stop we should not lock
	sc.Stop()
	sc.Stop()
	sc.Stop()

	require.Equal(t, 1, len(w.signalingQueue))
	s := <-w.signalingQueue
	require.Equal(t, ptp.TLVCancelUnicastTransmission, s.signaling.TLVs[0].(*ptp.CancelUnicastTransmissionTLV).TLVHead.TLVType)
	require.Equal(t, uint16(binary.Size(ptp.Header{})+binary.Size(ptp.PortIdentity{})+binary.Size(ptp.CancelUnicastTransmissionTLV{})), s.signaling.Header.MessageLength)
}

func TestSubscriptionEnd(t *testing.T) {
	w := &sendWorker{
		signalingQueue: make(chan *SubscriptionClient, 100),
	}
	c := &Config{clockIdentity: ptp.ClockIdentity(1234)}
	interval := 10 * time.Millisecond
	expire := time.Now().Add(300 * time.Millisecond)
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageDelayResp, c, interval, expire)

	ctx, cancel := context.WithCancel(context.Background())
	go sc.Start(ctx)

	time.Sleep(100 * time.Millisecond)
	require.True(t, sc.Running())

	cancel()
	time.Sleep(100 * time.Millisecond)
	require.False(t, sc.Running())
}

func TestSubscriptionflags(t *testing.T) {
	w := &sendWorker{}
	c := &Config{clockIdentity: ptp.ClockIdentity(1234)}
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageAnnounce, c, time.Second, time.Time{})

	sc.UpdateSync()
	sc.UpdateFollowup(time.Now())
	sc.UpdateAnnounce()
	require.Equal(t, ptp.FlagUnicast|ptp.FlagTwoStep, sc.Sync().Header.FlagField)
	require.Equal(t, ptp.FlagUnicast, sc.Followup().Header.FlagField)
	require.Equal(t, ptp.FlagUnicast|ptp.FlagPTPTimescale, sc.Announce().Header.FlagField)
}

func TestSyncPacket(t *testing.T) {
	sequenceID := uint16(42)
	domainNumber := uint8(13)

	w := &sendWorker{}
	c := &Config{
		clockIdentity: ptp.ClockIdentity(1234),
		StaticConfig: StaticConfig{
			DomainNumber: uint(domainNumber),
		},
	}
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageAnnounce, c, time.Second, time.Time{})
	sc.sequenceID = sequenceID

	sc.initSync()
	sc.IncSequenceID()
	sc.UpdateSync()
	require.Equal(t, uint16(44), sc.Sync().Header.MessageLength) // check packet length
	require.Equal(t, sequenceID+1, sc.Sync().Header.SequenceID)
	require.Equal(t, domainNumber, sc.Sync().Header.DomainNumber)
}

func TestFollowupPacket(t *testing.T) {
	sequenceID := uint16(42)
	now := time.Now()
	interval := 3 * time.Second
	domainNumber := uint8(13)

	w := &sendWorker{}

	c := &Config{
		clockIdentity: ptp.ClockIdentity(1234),
		StaticConfig: StaticConfig{
			DomainNumber: uint(domainNumber),
		},
	}
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageAnnounce, c, time.Second, time.Time{})
	sc.sequenceID = sequenceID
	sc.SetInterval(interval)

	i, err := ptp.NewLogInterval(interval)
	require.NoError(t, err)

	sc.initFollowup()
	sc.IncSequenceID()
	sc.UpdateFollowup(now)
	require.Equal(t, uint16(44), sc.Followup().Header.MessageLength) // check packet length
	require.Equal(t, sequenceID+1, sc.Followup().Header.SequenceID)
	require.Equal(t, i, sc.Followup().Header.LogMessageInterval)
	require.Equal(t, now.Unix(), sc.Followup().FollowUpBody.PreciseOriginTimestamp.Time().Unix())
	require.Equal(t, domainNumber, sc.Followup().Header.DomainNumber)
}

func TestAnnouncePacket(t *testing.T) {
	UTCOffset := 3 * time.Second
	sequenceID := uint16(42)
	interval := 3 * time.Second
	clockClass := ptp.ClockClass7
	clockAccuracy := ptp.ClockAccuracyMicrosecond1
	domainNumber := uint8(13)

	w := &sendWorker{}
	c := &Config{
		clockIdentity: ptp.ClockIdentity(1234),
		DynamicConfig: DynamicConfig{
			ClockClass:    clockClass,
			ClockAccuracy: clockAccuracy,
			UTCOffset:     UTCOffset,
		},
		StaticConfig: StaticConfig{
			DomainNumber: uint(domainNumber),
		},
	}
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageAnnounce, c, time.Second, time.Time{})
	sc.sequenceID = sequenceID
	sc.SetInterval(interval)

	i, err := ptp.NewLogInterval(interval)
	require.NoError(t, err)

	sp := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: ptp.ClockIdentity(1234),
	}

	sc.initAnnounce()
	sc.IncSequenceID()
	sc.UpdateAnnounce()
	require.Equal(t, uint16(64), sc.Announce().Header.MessageLength) // check packet length
	require.Equal(t, sequenceID+1, sc.Announce().Header.SequenceID)
	require.Equal(t, sp, sc.Announce().Header.SourcePortIdentity)
	require.Equal(t, i, sc.Announce().Header.LogMessageInterval)
	require.Equal(t, ptp.ClockClass7, sc.Announce().AnnounceBody.GrandmasterClockQuality.ClockClass)
	require.Equal(t, ptp.ClockAccuracyMicrosecond1, sc.Announce().AnnounceBody.GrandmasterClockQuality.ClockAccuracy)
	require.Equal(t, int16(UTCOffset.Seconds()), sc.Announce().AnnounceBody.CurrentUTCOffset)
	require.Equal(t, domainNumber, sc.Announce().Header.DomainNumber)
}

func TestDelayRespPacket(t *testing.T) {
	sequenceID := uint16(42)
	now := time.Now()
	domainNumber := uint8(13)

	w := &sendWorker{}
	c := &Config{
		clockIdentity: ptp.ClockIdentity(1234),
		StaticConfig: StaticConfig{
			DomainNumber: uint(domainNumber),
		},
	}
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageAnnounce, c, time.Second, time.Time{})

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
	require.Equal(t, uint16(54), sc.DelayResp().Header.MessageLength) // check packet length
	require.Equal(t, sequenceID, sc.DelayResp().Header.SequenceID)
	require.Equal(t, 100500, int(sc.DelayResp().Header.CorrectionField.Nanoseconds()))
	require.Equal(t, sp, sc.DelayResp().Header.SourcePortIdentity)
	require.Equal(t, now.Unix(), sc.DelayResp().DelayRespBody.ReceiveTimestamp.Time().Unix())
	require.Equal(t, ptp.FlagUnicast, sc.DelayResp().Header.FlagField)
	require.Equal(t, domainNumber, sc.DelayResp().Header.DomainNumber)
}

func TestSignalingGrantPacket(t *testing.T) {
	interval := 3 * time.Second

	w := &sendWorker{}
	c := &Config{clockIdentity: ptp.ClockIdentity(1234)}
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageAnnounce, c, time.Second, time.Time{})
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

	sc.initSignaling()
	sc.UpdateSignalingGrant(sg, mt, i, duration)

	require.Equal(t, uint16(56), sc.Signaling().Header.MessageLength) // check packet length
	require.Equal(t, tlv, sc.Signaling().TLVs[0])
}

func TestSignalingCancelPacket(t *testing.T) {
	w := &sendWorker{}
	c := &Config{clockIdentity: ptp.ClockIdentity(1234)}
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageAnnounce, c, time.Second, time.Time{})

	sc.signaling.Header.MessageLength = uint16(binary.Size(ptp.Header{}) + binary.Size(ptp.PortIdentity{}) + binary.Size(ptp.CancelUnicastTransmissionTLV{}))
	tlv := &ptp.CancelUnicastTransmissionTLV{
		TLVHead:         ptp.TLVHead{TLVType: ptp.TLVCancelUnicastTransmission, LengthField: uint16(binary.Size(ptp.CancelUnicastTransmissionTLV{}) - binary.Size(ptp.TLVHead{}))},
		Reserved:        0,
		MsgTypeAndFlags: ptp.NewUnicastMsgTypeAndFlags(ptp.MessageAnnounce, 0),
	}

	sc.initSignaling()
	sc.UpdateSignalingCancel()

	require.Equal(t, uint16(50), sc.Signaling().Header.MessageLength) // check packet length
	require.Equal(t, tlv, sc.Signaling().TLVs[0])
}

func TestSendSignalingGrant(t *testing.T) {
	w := &sendWorker{
		signalingQueue: make(chan *SubscriptionClient, 10),
	}
	c := &Config{
		clockIdentity: ptp.ClockIdentity(1234),
		StaticConfig: StaticConfig{
			SendWorkers: 10,
		},
	}

	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageAnnounce, c, time.Second, time.Time{})

	require.Equal(t, 0, len(w.signalingQueue))
	sc.sendSignalingGrant(&ptp.Signaling{}, 0, 0, 0)
	require.Equal(t, 1, len(w.signalingQueue))

	s := <-w.signalingQueue
	require.Equal(t, ptp.TLVGrantUnicastTransmission, s.signaling.TLVs[0].(*ptp.GrantUnicastTransmissionTLV).TLVHead.TLVType)
	require.Equal(t, uint16(binary.Size(ptp.Header{})+binary.Size(ptp.PortIdentity{})+binary.Size(ptp.GrantUnicastTransmissionTLV{})), s.signaling.Header.MessageLength)
}

func TestSendSignalingCancel(t *testing.T) {
	w := &sendWorker{
		signalingQueue: make(chan *SubscriptionClient, 10),
	}
	c := &Config{
		clockIdentity: ptp.ClockIdentity(1234),
		StaticConfig: StaticConfig{
			SendWorkers: 10,
		},
	}

	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageAnnounce, c, time.Second, time.Time{})

	require.Equal(t, 0, len(w.signalingQueue))
	sc.sendSignalingCancel()
	require.Equal(t, 1, len(w.signalingQueue))

	s := <-w.signalingQueue
	require.Equal(t, ptp.TLVCancelUnicastTransmission, s.signaling.TLVs[0].(*ptp.CancelUnicastTransmissionTLV).TLVHead.TLVType)
	require.Equal(t, uint16(binary.Size(ptp.Header{})+binary.Size(ptp.PortIdentity{})+binary.Size(ptp.CancelUnicastTransmissionTLV{})), s.signaling.Header.MessageLength)
}
