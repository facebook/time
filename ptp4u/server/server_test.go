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

	w := NewSendWorker(0, c, st)

	sa := ptp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	scS1 := NewSubscriptionClient(w.queue, sa, sa, ptp.MessageSync, c, time.Second, time.Now().Add(time.Minute))
	w.RegisterSubscription(clipi1, ptp.MessageSync, scS1)
	scS1.running = true
	w.inventoryClients()
	require.Equal(t, 1, len(w.clients))

	scA1 := NewSubscriptionClient(w.queue, sa, sa, ptp.MessageAnnounce, c, time.Second, time.Now().Add(time.Minute))
	w.RegisterSubscription(clipi1, ptp.MessageAnnounce, scA1)
	scA1.running = true
	w.inventoryClients()
	require.Equal(t, 2, len(w.clients))

	scS2 := NewSubscriptionClient(w.queue, sa, sa, ptp.MessageSync, c, time.Second, time.Now().Add(time.Minute))
	w.RegisterSubscription(clipi2, ptp.MessageSync, scS2)
	scS2.running = true
	w.inventoryClients()
	require.Equal(t, 2, len(w.clients[ptp.MessageSync]))

	// Shutting down
	scS1.running = false
	w.inventoryClients()
	require.Equal(t, 1, len(w.clients[ptp.MessageSync]))

	scA1.running = false
	w.inventoryClients()
	require.Equal(t, 0, len(w.clients[ptp.MessageAnnounce]))

	scS2.running = false
	w.inventoryClients()
	require.Equal(t, 0, len(w.clients[ptp.MessageSync]))
}
