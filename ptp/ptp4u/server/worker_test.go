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
	"net"
	"testing"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/ptp4u/stats"
	"github.com/facebook/time/timestamp"
	"github.com/stretchr/testify/require"
)

func TestWorkerQueue(t *testing.T) {
	c := &Config{
		clockIdentity: ptp.ClockIdentity(1234),
		StaticConfig: StaticConfig{
			TimestampType: timestamp.SW,
			Interface:     "lo",
		},
	}

	st := stats.NewJSONStats()
	go st.Start(0)
	time.Sleep(time.Millisecond)
	q := make(chan *SubscriptionClient)
	sq := make(chan *SubscriptionClient)

	w := &sendWorker{
		id:             0,
		queue:          q,
		signalingQueue: sq,
		stats:          st,
		config:         c,
	}

	go w.Start()

	interval := time.Millisecond
	expire := time.Now().Add(time.Millisecond)
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)

	scA := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageAnnounce, c, interval, expire)
	for i := 0; i < 10; i++ {
		w.queue <- scA
	}

	scS := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageSync, c, interval, expire)
	for i := 0; i < 10; i++ {
		w.queue <- scS
	}

	scDR := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageDelayResp, c, interval, expire)
	for i := 0; i < 10; i++ {
		w.queue <- scDR
	}

	scSig := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageSignaling, c, interval, expire)
	for i := 0; i < 10; i++ {
		w.signalingQueue <- scSig
	}

	// Just for fun add signaling into a subscription queue to make sure we discard it.
	w.queue <- scSig

	require.Equal(t, 0, len(w.queue))
	require.Equal(t, 0, len(w.signalingQueue))
}

func TestFindSubscription(t *testing.T) {
	c := &Config{
		clockIdentity: ptp.ClockIdentity(1234),
		StaticConfig: StaticConfig{
			TimestampType: timestamp.SW,
		},
	}

	w := &sendWorker{
		id:      0,
		queue:   make(chan *SubscriptionClient),
		clients: make(map[ptp.MessageType]map[ptp.PortIdentity]*SubscriptionClient),
	}

	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageAnnounce, c, time.Millisecond, time.Now().Add(time.Second))

	sp := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: ptp.ClockIdentity(1234),
	}

	w.RegisterSubscription(sp, ptp.MessageAnnounce, sc)

	sub := w.FindSubscription(sp, ptp.MessageAnnounce)
	require.Equal(t, sc, sub)
}

func TestFindClients(t *testing.T) {
	c := &Config{
		clockIdentity: ptp.ClockIdentity(1234),
		StaticConfig: StaticConfig{
			TimestampType: timestamp.SW,
		},
	}

	w := &sendWorker{
		id:      0,
		queue:   make(chan *SubscriptionClient),
		clients: make(map[ptp.MessageType]map[ptp.PortIdentity]*SubscriptionClient),
	}

	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageAnnounce, c, time.Millisecond, time.Now().Add(time.Second))

	sp := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: ptp.ClockIdentity(1234),
	}

	w.RegisterSubscription(sp, ptp.MessageAnnounce, sc)

	cli := w.FindClients(ptp.MessageAnnounce)

	require.Equal(t, sc, cli[sp])
}

func TestInventoryClients(t *testing.T) {
	clipi1 := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: ptp.ClockIdentity(1234),
	}
	clipi2 := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: ptp.ClockIdentity(5678),
	}
	c := &Config{
		clockIdentity: ptp.ClockIdentity(1234),
		StaticConfig: StaticConfig{
			QueueSize: 100, // Making sure subscriptions aren't blocked
		},
	}

	st := stats.NewJSONStats()
	go st.Start(0)
	time.Sleep(10 * time.Millisecond)

	w := newSendWorker(0, c, st)

	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	scS1 := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageSync, c, 10*time.Millisecond, time.Now().Add(time.Minute))
	w.RegisterSubscription(clipi1, ptp.MessageSync, scS1)
	go scS1.Start(context.Background())
	time.Sleep(10 * time.Millisecond)

	w.inventoryClients()
	require.Equal(t, 1, len(w.clients))

	scA1 := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageAnnounce, c, 10*time.Millisecond, time.Now().Add(time.Minute))
	w.RegisterSubscription(clipi1, ptp.MessageAnnounce, scA1)
	go scA1.Start(context.Background())
	time.Sleep(10 * time.Millisecond)

	w.inventoryClients()
	require.Equal(t, 2, len(w.clients))

	scS2 := NewSubscriptionClient(w.queue, w.signalingQueue, sa, sa, ptp.MessageSync, c, 10*time.Millisecond, time.Now().Add(time.Minute))
	w.RegisterSubscription(clipi2, ptp.MessageSync, scS2)
	go scS2.Start(context.Background())
	time.Sleep(10 * time.Millisecond)

	w.inventoryClients()
	require.Equal(t, 2, len(w.clients[ptp.MessageSync]))

	// Shutting down
	scS1.SetExpire(time.Now())
	time.Sleep(50 * time.Millisecond)
	w.inventoryClients()
	require.Equal(t, 1, len(w.clients[ptp.MessageSync]))

	scA1.SetExpire(time.Now())
	time.Sleep(50 * time.Millisecond)
	w.inventoryClients()
	require.Equal(t, 0, len(w.clients[ptp.MessageAnnounce]))

	scS2.Stop()
	time.Sleep(50 * time.Millisecond)
	w.inventoryClients()
	require.Equal(t, 0, len(w.clients[ptp.MessageSync]))
}
