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
	"os"
	"testing"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/ptp4u/stats"
	"github.com/facebook/time/timestamp"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestFindWorker(t *testing.T) {
	c := &Config{
		clockIdentity: ptp.ClockIdentity(1234),
		StaticConfig: StaticConfig{
			TimestampType: timestamp.SW,
			SendWorkers:   10,
		},
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
	require.Equal(t, 3, s.findWorker(clipi1, 0).id)
	require.Equal(t, 3, s.findWorker(clipi1, 0).id)
	require.Equal(t, 3, s.findWorker(clipi1, 0).id)

	require.Equal(t, 7, s.findWorker(clipi2, 0).id)
	require.Equal(t, 6, s.findWorker(clipi3, 0).id)
}

func TestStartEventListener(t *testing.T) {
	ptp.PortEvent = 0
	c := &Config{
		clockIdentity: ptp.ClockIdentity(1234),
		StaticConfig: StaticConfig{
			TimestampType: timestamp.SW,
			SendWorkers:   10,
			RecvWorkers:   10,
			IP:            net.ParseIP("127.0.0.1"),
			Interface:     "lo",
		},
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
		StaticConfig: StaticConfig{
			TimestampType: timestamp.SW,
			SendWorkers:   10,
			RecvWorkers:   10,
			IP:            net.ParseIP("127.0.0.1"),
		},
	}
	s := Server{
		Config: c,
		Stats:  stats.NewJSONStats(),
		sw:     make([]*sendWorker, c.SendWorkers),
	}
	go s.startGeneralListener()
	time.Sleep(100 * time.Millisecond)
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

func TestHandleSighup(t *testing.T) {
	expected := &Config{
		DynamicConfig: DynamicConfig{
			ClockAccuracy:  0,
			ClockClass:     1,
			DrainInterval:  2 * time.Second,
			MaxSubDuration: 3 * time.Hour,
			MetricInterval: 4 * time.Minute,
			MinSubInterval: 5 * time.Second,
			UTCOffset:      37 * time.Second,
		},
	}

	c := &Config{
		StaticConfig: StaticConfig{
			SendWorkers: 2,
			QueueSize:   10,
		},
	}
	s := Server{
		Config: c,
		Stats:  stats.NewJSONStats(),
		sw:     make([]*sendWorker, c.SendWorkers),
	}
	clipi := ptp.PortIdentity{
		PortNumber:    1,
		ClockIdentity: ptp.ClockIdentity(1234),
	}

	s.sw[0] = newSendWorker(0, s.Config, s.Stats)
	s.sw[1] = newSendWorker(0, s.Config, s.Stats)
	sa := timestamp.IPToSockaddr(net.ParseIP("127.0.0.1"), 123)
	scA := NewSubscriptionClient(s.sw[0].queue, s.sw[0].signalingQueue, sa, sa, ptp.MessageAnnounce, c, time.Second, time.Now().Add(time.Minute))
	scS := NewSubscriptionClient(s.sw[1].queue, s.sw[1].signalingQueue, sa, sa, ptp.MessageSync, c, time.Second, time.Now().Add(time.Minute))
	s.sw[0].RegisterSubscription(clipi, ptp.MessageAnnounce, scA)
	s.sw[1].RegisterSubscription(clipi, ptp.MessageSync, scS)

	go scA.Start(context.Background())
	go scS.Start(context.Background())
	time.Sleep(100 * time.Millisecond)

	require.Equal(t, 1, len(s.sw[0].queue))
	require.Equal(t, 1, len(s.sw[1].queue))

	cfg, err := os.CreateTemp("", "ptp4u")
	require.NoError(t, err)
	defer os.Remove(cfg.Name())

	config := `clockaccuracy: 0
clockclass: 1
draininterval: "2s"
maxsubduration: "3h"
metricinterval: "4m"
minsubinterval: "5s"
utcoffset: "37s"
`
	_, err = cfg.WriteString(config)
	require.NoError(t, err)

	c.ConfigFile = cfg.Name()

	go s.handleSighup()
	time.Sleep(100 * time.Millisecond)

	err = unix.Kill(unix.Getpid(), unix.SIGHUP)
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	dcMux.Lock()
	require.Equal(t, expected.DynamicConfig, c.DynamicConfig)
	dcMux.Unlock()

	require.Equal(t, 1, len(s.sw[0].queue))
	require.Equal(t, 1, len(s.sw[1].queue))
}

func TestHandleSigterm(t *testing.T) {
	cfg, err := os.CreateTemp("", "ptp4u")
	require.NoError(t, err)
	os.Remove(cfg.Name())
	require.NoFileExists(t, cfg.Name())

	c := &Config{StaticConfig: StaticConfig{PidFile: cfg.Name()}}
	s := Server{
		Config: c,
		Stats:  stats.NewJSONStats(),
	}

	err = c.CreatePidFile()
	require.NoError(t, err)
	require.FileExists(t, c.PidFile)

	// Delayed SIGTERM
	go func() {
		time.Sleep(100 * time.Millisecond)
		err = unix.Kill(unix.Getpid(), unix.SIGTERM)
	}()

	// Must exit the method
	s.handleSigterm()
	require.NoFileExists(t, cfg.Name())
}
