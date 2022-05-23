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

package protocol

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeIntervalNanoseconds(t *testing.T) {
	tests := []struct {
		in   TimeInterval
		want float64
	}{
		{
			in:   13697024,
			want: 209,
		},
		{
			in:   0x0000000000028000,
			want: 2.5,
		},
		{
			in:   -9240576,
			want: -141,
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("TimeInterval.Nanoseconds t=%d", tt.in), func(t *testing.T) {
			// first, convert from TimeInterval to time.Time
			got := tt.in.Nanoseconds()
			require.Equal(t, tt.want, got)
			// then convert time.Time we just got back to Timestamp
			gotTI := NewTimeInterval(got)
			assert.Equal(t, tt.in, gotTI)
		})
	}
}

func TestTimestamp(t *testing.T) {
	tests := []struct {
		in   Timestamp
		want time.Time
	}{
		{
			in: Timestamp{
				Seconds:     [6]byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x02},
				Nanoseconds: 1,
			},
			want: time.Unix(2, 1),
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("Timestamp t=%d", tt.in), func(t *testing.T) {
			// first, convert from Timestamp to time.Time
			got := tt.in.Time()
			require.Equal(t, tt.want, got)
			// then convert time.Time we just got back to Timestamp
			gotTS := NewTimestamp(got)
			assert.Equal(t, tt.in, gotTS)
		})
	}
}

func TestLogInterval(t *testing.T) {
	tests := []struct {
		in   LogInterval
		want float64 // seconds
	}{
		{
			in:   0,
			want: 1,
		},
		{
			in:   1,
			want: 2,
		},
		{
			in:   5,
			want: 32,
		},
		{
			in:   -1,
			want: 0.5,
		},
		{
			in:   -7,
			want: 0.0078125,
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("LogInterval t=%d", tt.in), func(t *testing.T) {
			// first, convert from LogInterval to Seconds
			gotDuration := tt.in.Duration()
			require.Equal(t, tt.want, gotDuration.Seconds())
			// then convert time.Duration we just got back to LogInterval
			gotLI, err := NewLogInterval(gotDuration)
			require.Nil(t, err)
			assert.Equal(t, tt.in, gotLI)
		})
	}
}

func TestClockIdentity(t *testing.T) {
	macStr := "0c:42:a1:6d:7c:a6"
	mac, err := net.ParseMAC(macStr)
	require.Nil(t, err)
	got, err := NewClockIdentity(mac)
	require.Nil(t, err)
	want := ClockIdentity(0xc42a1fffe6d7ca6)
	assert.Equal(t, want, got)
	wantStr := "0c42a1.fffe.6d7ca6"
	assert.Equal(t, wantStr, got.String())
	back := got.MAC()
	assert.Equal(t, mac, back)
}

func TestPTPText(t *testing.T) {
	tests := []struct {
		name    string
		in      []byte
		want    string
		wantErr bool
	}{
		{
			name:    "no data",
			in:      []byte{},
			want:    "",
			wantErr: true,
		},
		{
			name:    "empty",
			in:      []byte{0},
			want:    "",
			wantErr: false,
		},
		{
			name:    "some text",
			in:      []byte{4, 65, 108, 101, 120},
			want:    "Alex",
			wantErr: false,
		},
		{
			name:    "padding",
			in:      []byte{3, 120, 101, 108, 0},
			want:    "xel",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var text PTPText
			err := text.UnmarshalBinary(tt.in)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.Nil(t, err)
				assert.Equal(t, tt.want, string(text))

				gotBytes, err := text.MarshalBinary()
				require.Nil(t, err)
				assert.Equal(t, tt.in, gotBytes)
			}
		})
	}
}

func TestPortAddress(t *testing.T) {
	tests := []struct {
		name      string
		in        []byte
		want      *PortAddress
		wantIP    net.IP
		wantErr   bool
		wantIPErr bool
	}{
		{
			name:    "no data",
			in:      []byte{},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "empty",
			in:      []byte{0},
			want:    nil,
			wantErr: true,
		},
		{
			name:      "unsupported protocol",
			in:        []byte{0x00, 0x04, 0x00, 0x04, 192, 168, 0, 1},
			want:      nil,
			wantErr:   false,
			wantIPErr: true,
		},
		{
			name: "ipv4",
			in:   []byte{0x00, 0x01, 0x00, 0x04, 192, 168, 0, 1},
			want: &PortAddress{
				NetworkProtocol: TransportTypeUDPIPV4,
				AddressLength:   4,
				AddressField:    []byte{192, 168, 0, 1},
			},
			wantIP:  net.ParseIP("192.168.0.1"),
			wantErr: false,
		},
		{
			name: "ipv6",
			in:   []byte{0x00, 0x02, 0x00, 0x10, 0x24, 0x01, 0xdb, 0x00, 0xff, 0xfe, 0x01, 0x23, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want: &PortAddress{
				NetworkProtocol: TransportTypeUDPIPV6,
				AddressLength:   16,
				AddressField:    []byte{0x24, 0x01, 0xdb, 0x00, 0xff, 0xfe, 0x01, 0x23, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			},
			wantIP:  net.ParseIP("2401:db00:fffe:123::"),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var addr PortAddress
			err := addr.UnmarshalBinary(tt.in)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.Nil(t, err)
				ip, err := addr.IP()
				if tt.wantIPErr {
					require.Error(t, err)
					return
				}
				require.Nil(t, err)

				assert.Equal(t, *tt.want, addr)
				assert.True(t, tt.wantIP.Equal(ip), "expect parsed IP %v to be equal to %v", ip, tt.wantIP)

				gotBytes, err := addr.MarshalBinary()
				require.Nil(t, err)
				assert.Equal(t, tt.in, gotBytes)
			}
		})
	}
}

func TestClockAccuracyFromOffset(t *testing.T) {
	require.Equal(t, ClockAccuracyNanosecond25, ClockAccuracyFromOffset(-8*time.Nanosecond))
	require.Equal(t, ClockAccuracyNanosecond100, ClockAccuracyFromOffset(42*time.Nanosecond))
	require.Equal(t, ClockAccuracyNanosecond250, ClockAccuracyFromOffset(-242*time.Nanosecond))
	require.Equal(t, ClockAccuracyMicrosecond1, ClockAccuracyFromOffset(567*time.Nanosecond))
	require.Equal(t, ClockAccuracyMicrosecond2point5, ClockAccuracyFromOffset(2*time.Microsecond))
	require.Equal(t, ClockAccuracyMicrosecond10, ClockAccuracyFromOffset(8*time.Microsecond))
	require.Equal(t, ClockAccuracyMicrosecond25, ClockAccuracyFromOffset(11*time.Microsecond))
	require.Equal(t, ClockAccuracyMicrosecond100, ClockAccuracyFromOffset(-42*time.Microsecond))
	require.Equal(t, ClockAccuracyMicrosecond250, ClockAccuracyFromOffset(123*time.Microsecond))
	require.Equal(t, ClockAccuracyMillisecond1, ClockAccuracyFromOffset(678*time.Microsecond))
	require.Equal(t, ClockAccuracyMillisecond2point5, ClockAccuracyFromOffset(2499*time.Microsecond))
	require.Equal(t, ClockAccuracyMillisecond10, ClockAccuracyFromOffset(-8*time.Millisecond))
	require.Equal(t, ClockAccuracyMillisecond25, ClockAccuracyFromOffset(24*time.Millisecond))
	require.Equal(t, ClockAccuracyMillisecond100, ClockAccuracyFromOffset(69*time.Millisecond))
	require.Equal(t, ClockAccuracyMillisecond250, ClockAccuracyFromOffset(222*time.Millisecond))
	require.Equal(t, ClockAccuracySecond1, ClockAccuracyFromOffset(-999*time.Millisecond))
	require.Equal(t, ClockAccuracySecond10, ClockAccuracyFromOffset(10*time.Second))
	require.Equal(t, ClockAccuracySecondGreater10, ClockAccuracyFromOffset(9*time.Minute))
}

func TestAccuracyNSFromClockQuality(t *testing.T) {
	require.Equal(t, time.Nanosecond*25, AccuracyNSFromClockQuality(ClockQuality{ClockClass: ClockClass6, ClockAccuracy: ClockAccuracyNanosecond25, OffsetScaledLogVariance: 0}))
	require.Equal(t, time.Nanosecond*100, AccuracyNSFromClockQuality(ClockQuality{ClockClass: ClockClass6, ClockAccuracy: ClockAccuracyNanosecond100, OffsetScaledLogVariance: 0}))
	require.Equal(t, time.Nanosecond*250, AccuracyNSFromClockQuality(ClockQuality{ClockClass: ClockClass6, ClockAccuracy: ClockAccuracyNanosecond250, OffsetScaledLogVariance: 0}))
	require.Equal(t, time.Microsecond, AccuracyNSFromClockQuality(ClockQuality{ClockClass: ClockClass6, ClockAccuracy: ClockAccuracyMicrosecond1, OffsetScaledLogVariance: 0}))
	require.Equal(t, time.Nanosecond*2500, AccuracyNSFromClockQuality(ClockQuality{ClockClass: ClockClass6, ClockAccuracy: ClockAccuracyMicrosecond2point5, OffsetScaledLogVariance: 0}))
	require.Equal(t, time.Microsecond*10, AccuracyNSFromClockQuality(ClockQuality{ClockClass: ClockClass6, ClockAccuracy: ClockAccuracyMicrosecond10, OffsetScaledLogVariance: 0}))
	require.Equal(t, time.Microsecond*25, AccuracyNSFromClockQuality(ClockQuality{ClockClass: ClockClass6, ClockAccuracy: ClockAccuracyMicrosecond25, OffsetScaledLogVariance: 0}))
	require.Equal(t, time.Microsecond*100, AccuracyNSFromClockQuality(ClockQuality{ClockClass: ClockClass6, ClockAccuracy: ClockAccuracyMicrosecond100, OffsetScaledLogVariance: 0}))
	require.Equal(t, time.Microsecond*250, AccuracyNSFromClockQuality(ClockQuality{ClockClass: ClockClass6, ClockAccuracy: ClockAccuracyMicrosecond250, OffsetScaledLogVariance: 0}))
	require.Equal(t, time.Millisecond, AccuracyNSFromClockQuality(ClockQuality{ClockClass: ClockClass6, ClockAccuracy: ClockAccuracyMillisecond1, OffsetScaledLogVariance: 0}))
	require.Equal(t, time.Microsecond*2500, AccuracyNSFromClockQuality(ClockQuality{ClockClass: ClockClass6, ClockAccuracy: ClockAccuracyMillisecond2point5, OffsetScaledLogVariance: 0}))
	require.Equal(t, time.Millisecond*10, AccuracyNSFromClockQuality(ClockQuality{ClockClass: ClockClass6, ClockAccuracy: ClockAccuracyMillisecond10, OffsetScaledLogVariance: 0}))
	require.Equal(t, time.Millisecond*25, AccuracyNSFromClockQuality(ClockQuality{ClockClass: ClockClass6, ClockAccuracy: ClockAccuracyMillisecond25, OffsetScaledLogVariance: 0}))
	require.Equal(t, time.Millisecond*100, AccuracyNSFromClockQuality(ClockQuality{ClockClass: ClockClass6, ClockAccuracy: ClockAccuracyMillisecond100, OffsetScaledLogVariance: 0}))
	require.Equal(t, time.Millisecond*250, AccuracyNSFromClockQuality(ClockQuality{ClockClass: ClockClass6, ClockAccuracy: ClockAccuracyMillisecond250, OffsetScaledLogVariance: 0}))
	require.Equal(t, time.Second, AccuracyNSFromClockQuality(ClockQuality{ClockClass: ClockClass6, ClockAccuracy: ClockAccuracySecond1, OffsetScaledLogVariance: 0}))
	require.Equal(t, time.Second*10, AccuracyNSFromClockQuality(ClockQuality{ClockClass: ClockClass6, ClockAccuracy: ClockAccuracySecond10, OffsetScaledLogVariance: 0}))
	require.Equal(t, time.Second*25, AccuracyNSFromClockQuality(ClockQuality{ClockClass: ClockClass6, ClockAccuracy: ClockAccuracySecondGreater10, OffsetScaledLogVariance: 0}))
}
