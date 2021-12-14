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
	"math/rand"
	"testing"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/ptp4u/stats"
	"github.com/stretchr/testify/require"
)

func TestFindWorker(t *testing.T) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	c := &Config{
		clockIdentity: ptp.ClockIdentity(1234),
		TimestampType: ptp.SWTIMESTAMP,
		SendWorkers:   10,
	}
	s := Server{
		Config: c,
		Stats:  stats.NewJSONStats(),
		sw:     make([]*sendWorker, c.SendWorkers),
	}

	for i := 0; i < s.Config.SendWorkers; i++ {
		s.sw[i] = NewSendWorker(i, c, s.Stats)
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
