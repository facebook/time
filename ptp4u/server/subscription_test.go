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

	"github.com/stretchr/testify/require"
)

func TestRunning(t *testing.T) {
	sc := SubscriptionClient{}
	sc.setRunning(true)
	require.True(t, sc.Running())

	sc.setRunning(false)
	require.False(t, sc.Running())
}

func TestSubscriptionStart(t *testing.T) {
	w := &sendWorker{}
	c := &Config{clockIdentity: ptp.ClockIdentity(1234)}
	interval := 1 * time.Minute
	expire := time.Now().Add(1 * time.Second)
	sc := NewSubscriptionClient(w, net.ParseIP("127.0.0.1"), ptp.MessageAnnounce, c, interval, expire)

	go sc.Start()
	time.Sleep(100 * time.Millisecond)
	require.True(t, sc.Running())
}

func TestSubscriptionStop(t *testing.T) {
	w := &sendWorker{
		queue: make(chan *SubscriptionClient, 100),
	}
	c := &Config{clockIdentity: ptp.ClockIdentity(1234)}
	interval := 10 * time.Millisecond
	expire := time.Now().Add(1 * time.Second)
	sc := NewSubscriptionClient(w, net.ParseIP("127.0.0.1"), ptp.MessageAnnounce, c, interval, expire)

	go sc.Start()
	time.Sleep(100 * time.Millisecond)
	require.True(t, sc.Running())
	require.Equal(t, int64(1000), w.load)
	sc.Stop()
	time.Sleep(100 * time.Millisecond)
	require.False(t, sc.Running())
}

func TestSubscriptionflags(t *testing.T) {
	c := &Config{clockIdentity: ptp.ClockIdentity(1234)}
	sc := SubscriptionClient{
		serverConfig: c,
		interval:     time.Second,
	}

	require.Equal(t, ptp.FlagUnicast|ptp.FlagTwoStep, sc.syncPacket().Header.FlagField)
	require.Equal(t, ptp.FlagUnicast, sc.followupPacket(time.Now()).Header.FlagField)
	require.Equal(t, ptp.FlagUnicast|ptp.FlagPTPTimescale, sc.announcePacket().Header.FlagField)
}

func TestSyncMapSub(t *testing.T) {
	sm := syncMapSub{}
	sm.init()
	require.Equal(t, 0, len(sm.keys()))

	ci := ptp.ClockIdentity(1234)
	c := &Config{clockIdentity: ci}
	sc := &SubscriptionClient{serverConfig: c}
	st := ptp.MessageAnnounce
	sm.store(st, sc)

	sct, ok := sm.load(st)
	require.True(t, ok)
	require.Equal(t, sc, sct)
	require.Equal(t, 1, len(sm.keys()))
}

func TestSyncMapCli(t *testing.T) {
	sm := syncMapCli{}
	sm.init()
	require.Equal(t, 0, len(sm.keys()))

	pi := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: ptp.ClockIdentity(1234),
	}

	val := &syncMapSub{}
	val.init()

	sm.store(pi, val)
	require.Equal(t, 1, len(sm.keys()))

	valt, ok := sm.load(pi)
	require.True(t, ok)
	require.Equal(t, val, valt)
	require.Equal(t, 1, len(sm.keys()))
}
