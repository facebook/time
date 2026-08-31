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

package daemon

import (
	"net"
	"testing"
	"time"

	"github.com/facebook/time/leapsectz"
	"github.com/stretchr/testify/require"
)

// trackingReply is a real reply captured from chronyd, the same fixture the
// chrony package decodes in its own tests.
var trackingReply = []uint8{
	0x06, 0x02, 0x00, 0x00, 0x00, 0x21, 0x00, 0x05, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xe6, 0x25,
	0xc6, 0x6e, 0x24, 0x01, 0xdb, 0x00, 0x31, 0x10, 0x21, 0x32,
	0xfa, 0xce, 0x00, 0x00, 0x00, 0x8e, 0x00, 0x00, 0x00, 0x02,
	0x00, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x61, 0x38, 0xe1, 0x81, 0x36, 0x94, 0x8d, 0xd5, 0xdf, 0x19,
	0x2d, 0xb7, 0xdf, 0x42, 0x83, 0xf5, 0xe2, 0xeb, 0xca, 0x12,
	0x05, 0x39, 0xe1, 0x11, 0xeb, 0x7b, 0x3e, 0x5d, 0xf4, 0xb0,
	0x75, 0x12, 0xea, 0xe7, 0x5b, 0x0c, 0xf0, 0x88, 0x1d, 0x4e,
	0x16, 0x82, 0x1f, 0x69,
}

// fakeChronyd answers the first request on a loopback UDP port; nil leaves it unanswered.
func fakeChronyd(t *testing.T, reply []uint8) (address string) {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	go func() {
		buf := make([]uint8, 1024)
		_, client, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if reply != nil {
			_, _ = conn.WriteToUDP(reply, client)
		}
	}()
	return conn.LocalAddr().String()
}

func TestFetchTracking(t *testing.T) {
	cfg := &Config{Interval: 2 * time.Second}

	tracking, err := (&ChronyFetcher{address: fakeChronyd(t, trackingReply)}).FetchTracking(cfg)
	require.NoError(t, err)
	require.Equal(t, uint16(3), tracking.Stratum)
	require.Equal(t, time.Unix(0, 1631117697915705301), tracking.RefTime)
	require.Equal(t, 0.0010384710039943457, tracking.RootDispersion)
}

func TestFetchTrackingTimeout(t *testing.T) {
	// UDP gives no connection-refused, so the read deadline ends the poll
	cfg := &Config{Interval: 20 * time.Millisecond}

	_, err := (&ChronyFetcher{address: fakeChronyd(t, nil)}).FetchTracking(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to get tracking from chronyd")
}

// testUTCOffsetS is what current tzdata yields (latest leap 2017-01-01), and
// leap2017 is that leap instant: Tleap-Nleap+1 of the record below.
const (
	testUTCOffsetS = 37
	leap2015       = 1435708800
	leap2017       = 1483228800
)

var (
	// testLeaps is verbatim from /usr/share/zoneinfo/right/UTC
	testLeaps = []leapsectz.LeapSecond{
		{Tleap: 1435708825, Nleap: 26},
		{Tleap: 1483228826, Nleap: 27},
	}
	// testSysTime is the CLOCK_REALTIME read the daemon stamps at fetch time, a
	// second after the fixtures' RefTime so assertions can tell them apart.
	testSysTime = time.Unix(1755000001, 0)
)

func TestCurrentUTCOffsetS(t *testing.T) {
	testCases := []struct {
		name string
		now  time.Time
		want int32
	}{
		{name: "long after the latest leap", now: testSysTime, want: 37},
		{name: "one second before the leap", now: time.Unix(leap2017-1, 0), want: 36},
		{name: "at the leap instant", now: time.Unix(leap2017, 0), want: 37},
		// older than every record: floors at the same offset the client subtracts
		{name: "before every record we hold", now: time.Unix(leap2015-1, 0), want: 36},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, currentUTCOffsetS(testLeaps, tc.now))
		})
	}
}
