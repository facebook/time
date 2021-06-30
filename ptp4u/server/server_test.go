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
	"net"
	"testing"
	"time"

	ptp "github.com/facebookincubator/ptp/protocol"
	"github.com/facebookincubator/ptp/ptp4u/stats"
	"github.com/stretchr/testify/require"
)

func TestServerRegisterSubscription(t *testing.T) {
	var (
		scE *SubscriptionClient
		scS *SubscriptionClient
		scA *SubscriptionClient
		scT *SubscriptionClient
	)

	ci := ptp.ClockIdentity(1234)
	pi := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: ptp.ClockIdentity(1234),
	}

	c := &Config{clockIdentity: ci}
	s := Server{Config: c}
	s.clients.init()

	// Nothing should be there
	scE = s.findSubscription(pi, ptp.MessageSync)
	require.Nil(t, scE)

	scE = s.findSubscription(pi, ptp.MessageAnnounce)
	require.Nil(t, scE)

	// Add Sync. Check we got
	scS = NewSubscriptionClient(nil, net.ParseIP("127.0.0.1"), ptp.MessageSync, c, time.Second, time.Now())
	s.registerSubscription(pi, ptp.MessageSync, scS)
	// Check Sync is saved
	scT = s.findSubscription(pi, ptp.MessageSync)
	require.Equal(t, scS, scT)

	// Add announce. Check we have now both
	scA = NewSubscriptionClient(nil, net.ParseIP("127.0.0.1"), ptp.MessageAnnounce, c, time.Second, time.Now())
	s.registerSubscription(pi, ptp.MessageAnnounce, scA)
	// First check Sync
	scT = s.findSubscription(pi, ptp.MessageSync)
	require.Equal(t, scS, scT)
	// Then check Announce
	scT = s.findSubscription(pi, ptp.MessageAnnounce)
	require.Equal(t, scA, scT)

	// Override announce
	scA = NewSubscriptionClient(nil, net.ParseIP("127.0.0.1"), ptp.MessageAnnounce, c, time.Second, time.Now())
	s.registerSubscription(pi, ptp.MessageAnnounce, scA)
	// Check new Announce is saved
	scT = s.findSubscription(pi, ptp.MessageAnnounce)
	require.Equal(t, scA, scT)
}

func TestServerInventoryClients(t *testing.T) {
	clipi1 := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: ptp.ClockIdentity(1234),
	}
	clipi2 := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: ptp.ClockIdentity(5678),
	}
	c := &Config{clockIdentity: ptp.ClockIdentity(1234)}

	st := stats.NewJSONStats()
	go st.Start(0)
	time.Sleep(time.Millisecond)

	s := Server{Config: c, Stats: st}
	s.clients.init()

	scS1 := NewSubscriptionClient(nil, net.ParseIP("127.0.0.1"), ptp.MessageSync, c, time.Second, time.Now().Add(time.Minute))
	s.registerSubscription(clipi1, ptp.MessageSync, scS1)
	scS1.running = true
	s.inventoryClients()
	require.Equal(t, 1, len(s.clients.keys()))

	scA1 := NewSubscriptionClient(nil, net.ParseIP("127.0.0.1"), ptp.MessageAnnounce, c, time.Second, time.Now().Add(time.Minute))
	s.registerSubscription(clipi1, ptp.MessageSync, scA1)
	scA1.running = true
	s.inventoryClients()
	require.Equal(t, 1, len(s.clients.keys()))

	scS2 := NewSubscriptionClient(nil, net.ParseIP("127.0.0.1"), ptp.MessageSync, c, time.Second, time.Now().Add(time.Minute))
	s.registerSubscription(clipi2, ptp.MessageSync, scS2)
	scS2.running = true
	s.inventoryClients()
	require.Equal(t, 2, len(s.clients.keys()))

	// Shutting down
	scS1.running = false
	s.inventoryClients()
	require.Equal(t, 2, len(s.clients.keys()))

	scA1.running = false
	s.inventoryClients()
	require.Equal(t, 1, len(s.clients.keys()))

	scS2.running = false
	s.inventoryClients()
	require.Equal(t, 0, len(s.clients.keys()))
}

func TestDelayRespPacket(t *testing.T) {
	c := &Config{clockIdentity: ptp.ClockIdentity(1234)}
	st := stats.NewJSONStats()
	s := Server{Config: c, Stats: st}
	sp := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: ptp.ClockIdentity(1234),
	}
	h := &ptp.Header{
		SequenceID:         42,
		CorrectionField:    ptp.NewCorrection(100500),
		SourcePortIdentity: sp,
	}

	now := time.Now()

	dResp := s.delayRespPacket(h, now)
	// Unicast flag
	require.Equal(t, uint16(42), dResp.Header.SequenceID)
	require.Equal(t, 100500, int(dResp.Header.CorrectionField.Nanoseconds()))
	require.Equal(t, sp, dResp.Header.SourcePortIdentity)
	require.Equal(t, now.Unix(), dResp.DelayRespBody.ReceiveTimestamp.Time().Unix())
	require.Equal(t, ptp.FlagUnicast, dResp.Header.FlagField)
}
