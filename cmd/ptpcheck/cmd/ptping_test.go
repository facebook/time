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

package cmd

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/sptp/client"
)

func TestTimestamps(t *testing.T) {
	now := time.Now()
	timeout := time.Millisecond
	s := client.ReqDelay(ptp.ClockIdentity(0xc42a1fffe6d7ca6), 1)
	s.OriginTimestamp = ptp.NewTimestamp(now)
	sb, err := s.MarshalBinary()
	require.NoError(t, err)

	a := client.ReqAnnounce(ptp.ClockIdentity(0xc42a1fffe6d7ca6), 1, now)
	ab, err := a.MarshalBinary()
	require.NoError(t, err)

	p := &ptping{
		inChan: make(chan *client.InPacket, 2),
	}

	_, _, _, err = p.timestamps(timeout)
	require.Equal(t, fmt.Errorf("timeout waiting"), err)

	p.inChan <- client.NewInPacket(sb, now)
	_, t2, t4, err := p.timestamps(timeout)
	require.NoError(t, err)
	require.Equal(t, now.Nanosecond(), t2.Nanosecond())
	require.Equal(t, now.Nanosecond(), t4.Nanosecond())

	p.inChan <- client.NewInPacket(ab, now)
	p.inChan <- client.NewInPacket(sb, now)

	t1, t2, t4, err := p.timestamps(timeout)
	require.NoError(t, err)
	require.Equal(t, now.Nanosecond(), t1.Nanosecond())
	require.Equal(t, now.Nanosecond(), t2.Nanosecond())
	require.Equal(t, now.Nanosecond(), t4.Nanosecond())
}
