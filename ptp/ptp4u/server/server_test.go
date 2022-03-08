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
	"math/rand"
	"net"
	"testing"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/ptp4u/stats"
	"github.com/facebook/time/timestamp"
	"github.com/stretchr/testify/require"
)

func TestFindWorker(t *testing.T) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	c := &Config{
		clockIdentity: ptp.ClockIdentity(1234),
		TimestampType: timestamp.SWTIMESTAMP,
		SendWorkers:   10,
	}
	s := Server{
		Config: c,
		Stats:  stats.NewJSONStats(),
		sw:     make([]*sendWorker, c.SendWorkers),
	}

	for i := 0; i < s.Config.SendWorkers; i++ {
		s.sw[i] = newSendWorker(i, c, s.Stats)
	}

	clipi1 := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: ptp.ClockIdentity(1234),
	}

	clipi2 := ptp.PortIdentity{
		PortNumber:    2,
		ClockIdentity: ptp.ClockIdentity(1234),
	}

	clipi3 := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: ptp.ClockIdentity(5678),
	}

	// Consistent across multiple calls
	require.Equal(t, 0, s.findWorker(clipi1, r).id)
	require.Equal(t, 0, s.findWorker(clipi1, r).id)
	require.Equal(t, 0, s.findWorker(clipi1, r).id)

	require.Equal(t, 3, s.findWorker(clipi2, r).id)
	require.Equal(t, 1, s.findWorker(clipi3, r).id)
}

func TestStartEventListener(t *testing.T) {
	ptp.PortEvent = 0
	c := &Config{
		clockIdentity: ptp.ClockIdentity(1234),
		TimestampType: timestamp.SWTIMESTAMP,
		SendWorkers:   10,
		RecvWorkers:   10,
		IP:            net.ParseIP("127.0.0.1"),
	}
	s := Server{
		Config: c,
		Stats:  stats.NewJSONStats(),
		sw:     make([]*sendWorker, c.SendWorkers),
	}
	go s.startEventListener()
	time.Sleep(100 * time.Millisecond)
}

func TestStartGeneralListener(t *testing.T) {
	ptp.PortGeneral = 0
	c := &Config{
		clockIdentity: ptp.ClockIdentity(1234),
		TimestampType: timestamp.SWTIMESTAMP,
		SendWorkers:   10,
		RecvWorkers:   10,
		IP:            net.ParseIP("127.0.0.1"),
	}
	s := Server{
		Config: c,
		Stats:  stats.NewJSONStats(),
		sw:     make([]*sendWorker, c.SendWorkers),
	}
	go s.startGeneralListener()
	time.Sleep(100 * time.Millisecond)
}

func TestSendGrant(t *testing.T) {
	w := &sendWorker{}
	c := &Config{
		clockIdentity: ptp.ClockIdentity(1234),
		SendWorkers:   10,
	}
	s := Server{
		Config: c,
		Stats:  stats.NewJSONStats(),
		sw:     make([]*sendWorker, c.SendWorkers),
	}
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	sc := NewSubscriptionClient(w.queue, sa, sa, ptp.MessageAnnounce, c, time.Second, time.Time{})

	s.sendGrant(sc, &ptp.Signaling{}, 0, 0, 0, sa)
}

func TestDrain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := Server{
		Stats:  stats.NewJSONStats(),
		ctx:    ctx,
		cancel: cancel,
	}

	require.NoError(t, s.ctx.Err())
	s.Drain()
	require.ErrorIs(t, context.Canceled, s.ctx.Err())
}

func TestUndrain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := Server{
		Stats:  stats.NewJSONStats(),
		ctx:    ctx,
		cancel: cancel,
	}

	s.Drain()
	require.ErrorIs(t, context.Canceled, s.ctx.Err())
	s.Undrain()
	require.NoError(t, s.ctx.Err())
}
