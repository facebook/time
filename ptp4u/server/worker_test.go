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

func TestWorkerQueue(t *testing.T) {
	c := &Config{
		clockIdentity: ptp.ClockIdentity(1234),
		TimestampType: ptp.SWTIMESTAMP,
	}

	st := stats.NewJSONStats()
	go st.Start(0)
	time.Sleep(time.Millisecond)
	queue := make(chan *SubscriptionClient)

	w := &sendWorker{
		id:     0,
		queue:  queue,
		stats:  st,
		config: c,
	}

	go w.Start()

	interval := time.Millisecond
	expire := time.Now().Add(time.Millisecond)
	sc := NewSubscriptionClient(w, net.ParseIP("127.0.0.1"), ptp.MessageAnnounce, c, interval, expire)

	for i := 0; i < 10; i++ {
		w.queue <- sc
	}
	require.Equal(t, 0, len(queue))
}
