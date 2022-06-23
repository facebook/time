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
	"math"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSdoIDAndMsgType(t *testing.T) {
	sdoIDAndMsgType := NewSdoIDAndMsgType(MessageSignaling, 123)
	require.Equal(t, MessageSignaling, sdoIDAndMsgType.MsgType())
}

func TestProbeMsgType(t *testing.T) {
	tests := []struct {
		in      []byte
		want    MessageType
		wantErr bool
	}{
		{
			in:      []byte{},
			wantErr: true,
		},
		{
			in:   []byte{0x0},
			want: MessageSync,
		},
		{
			in:   []byte{0xC},
			want: MessageSignaling,
		},
		{
			in:   []byte{0xBC},
			want: MessageSignaling,
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("ProbeMsgType in=%d", tt.in), func(t *testing.T) {
			got, err := ProbeMsgType(tt.in)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestTLVHeadType(t *testing.T) {
	head := &TLVHead{
		TLVType:     TLVRequestUnicastTransmission,
		LengthField: 10,
	}
	require.Equal(t, TLVRequestUnicastTransmission, head.Type())
}

func TestMessageTypeString(t *testing.T) {
	require.Equal(t, "SYNC", MessageSync.String())
	require.Equal(t, "DELAY_REQ", MessageDelayReq.String())
	require.Equal(t, "PDELAY_REQ", MessagePDelayReq.String())
	require.Equal(t, "PDELAY_RES", MessagePDelayResp.String())
	require.Equal(t, "FOLLOW_UP", MessageFollowUp.String())
	require.Equal(t, "DELAY_RESP", MessageDelayResp.String())
	require.Equal(t, "PDELAY_RESP_FOLLOW_UP", MessagePDelayRespFollowUp.String())
	require.Equal(t, "ANNOUNCE", MessageAnnounce.String())
	require.Equal(t, "SIGNALING", MessageSignaling.String())
	require.Equal(t, "MANAGEMENT", MessageManagement.String())
}

func TestTLVTypeString(t *testing.T) {
	require.Equal(t, "MANAGEMENT", TLVManagement.String())
	require.Equal(t, "MANAGEMENT_ERROR_STATUS", TLVManagementErrorStatus.String())
	require.Equal(t, "ORGANIZATION_EXTENSION", TLVOrganizationExtension.String())
	require.Equal(t, "REQUEST_UNICAST_TRANSMISSION", TLVRequestUnicastTransmission.String())
	require.Equal(t, "GRANT_UNICAST_TRANSMISSION", TLVGrantUnicastTransmission.String())
	require.Equal(t, "CANCEL_UNICAST_TRANSMISSION", TLVCancelUnicastTransmission.String())
	require.Equal(t, "ACKNOWLEDGE_CANCEL_UNICAST_TRANSMISSION", TLVAcknowledgeCancelUnicastTransmission.String())
	require.Equal(t, "PATH_TRACE", TLVPathTrace.String())
	require.Equal(t, "ALTERNATE_TIME_OFFSET_INDICATOR", TLVAlternateTimeOffsetIndicator.String())
}

func TestTimeSourceString(t *testing.T) {
	require.Equal(t, "ATOMIC_CLOCK", TimeSourceAtomicClock.String())
	require.Equal(t, "GNSS", TimeSourceGNSS.String())
	require.Equal(t, "TERRESTRIAL_RADIO", TimeSourceTerrestrialRadio.String())
	require.Equal(t, "SERIAL_TIME_CODE", TimeSourceSerialTimeCode.String())
	require.Equal(t, "PTP", TimeSourcePTP.String())
	require.Equal(t, "NTP", TimeSourceNTP.String())
	require.Equal(t, "HAND_SET", TimeSourceHandSet.String())
	require.Equal(t, "OTHER", TimeSourceOther.String())
	require.Equal(t, "INTERNAL_OSCILLATOR", TimeSourceInternalOscillator.String())
}

func TestPortStateString(t *testing.T) {
	require.Equal(t, "INITIALIZING", PortStateInitializing.String())
	require.Equal(t, "FAULTY", PortStateFaulty.String())
	require.Equal(t, "DISABLED", PortStateDisabled.String())
	require.Equal(t, "LISTENING", PortStateListening.String())
	require.Equal(t, "PRE_MASTER", PortStatePreMaster.String())
	require.Equal(t, "MASTER", PortStateMaster.String())
	require.Equal(t, "PASSIVE", PortStatePassive.String())
	require.Equal(t, "UNCALIBRATED", PortStateUncalibrated.String())
	require.Equal(t, "SLAVE", PortStateSlave.String())
	require.Equal(t, "GRAND_MASTER", PortStateGrandMaster.String())
}

func TestPortIdentityString(t *testing.T) {
	pi := PortIdentity{}
	require.Equal(t, "000000.0000.000000-0", pi.String())
	pi = PortIdentity{
		ClockIdentity: 5212879185253000328,
		PortNumber:    1,
	}
	require.Equal(t, "4857dd.fffe.086488-1", pi.String())
}

func TestTimeIntervalNanoseconds(t *testing.T) {
	tests := []struct {
		in      TimeInterval
		want    float64
		wantStr string
	}{
		{
			in:      13697024,
			want:    209,
			wantStr: "TimeInterval(209.000ns)",
		},
		{
			in:      0x0000000000028000,
			want:    2.5,
			wantStr: "TimeInterval(2.500ns)",
		},
		{
			in:      -9240576,
			want:    -141,
			wantStr: "TimeInterval(-141.000ns)",
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("TimeInterval.Nanoseconds t=%d", tt.in), func(t *testing.T) {
			// first, convert from TimeInterval to time.Time
			got := tt.in.Nanoseconds()
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.wantStr, tt.in.String())
			// then convert time.Time we just got back to Timestamp
			gotTI := NewTimeInterval(got)
			assert.Equal(t, tt.in, gotTI)
		})
	}
}

func TestTimestamp(t *testing.T) {
	tests := []struct {
		in      Timestamp
		want    time.Time
		wantStr string
	}{
		{
			in: Timestamp{
				Seconds:     [6]byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x02},
				Nanoseconds: 1,
			},
			want:    time.Unix(2, 1),
			wantStr: fmt.Sprintf("Timestamp(%s)", time.Unix(2, 1)),
		},
		{
			in: Timestamp{
				Seconds:     [6]byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
				Nanoseconds: 0,
			},
			want:    time.Time{},
			wantStr: "Timestamp(empty)",
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("Timestamp t=%d", tt.in), func(t *testing.T) {
			// first, convert from Timestamp to time.Time
			got := tt.in.Time()
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.wantStr, tt.in.String())
			// then convert time.Time we just got back to Timestamp
			gotTS := NewTimestamp(got)
			assert.Equal(t, tt.in, gotTS)
		})
	}
}

func TestCorrection(t *testing.T) {
	tests := []struct {
		in         time.Duration
		want       Correction
		wantTooBig bool
		wantStr    string
	}{
		{
			in:      time.Millisecond,
			want:    Correction(65536000000),
			wantStr: "Correction(1000000.000ns)",
		},
		{
			in:         50 * time.Hour,
			want:       Correction(0x7fffffffffffffff),
			wantTooBig: true,
			wantStr:    "Correction(Too big)",
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("Correction of %v", tt.in), func(t *testing.T) {
			// first, convert from time.Duration to Correction
			got := NewCorrection(float64(tt.in))
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.wantStr, got.String())
			// then convert Correction we just got back to time.Duration
			gotNS := got.Nanoseconds()
			if tt.wantTooBig {
				require.True(t, math.IsInf(gotNS, 1))
			} else {
				require.Equal(t, tt.in, time.Duration(gotNS))
			}
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
		{
			name:    "non-ascii",
			in:      []byte{3, 120, 255, 200, 0},
			want:    "x\xff\xc8",
			wantErr: false,
		},
		{
			name:    "too short",
			in:      []byte{20, 120, 255, 200, 0},
			want:    "",
			wantErr: true,
		},
		{
			name:    "single",
			in:      []byte{1, 65, 0},
			want:    "A",
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
			name:      "ipv4 too long",
			in:        []byte{0x00, 0x01, 0x00, 0x05, 192, 168, 0, 1, 0},
			want:      nil,
			wantErr:   false,
			wantIPErr: true,
		},
		{
			name:      "ipv4 too short",
			in:        []byte{0x00, 0x01, 0x00, 0x04, 192, 168, 0},
			want:      nil,
			wantErr:   true,
			wantIPErr: false,
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
			name:      "ipv6 too short",
			in:        []byte{0x00, 0x02, 0x00, 0x10, 0x24, 0x01, 0xdb, 0x00, 0xff, 0xfe, 0x01, 0x23, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want:      nil,
			wantErr:   true,
			wantIPErr: false,
		},
		{
			name:      "ipv6 too long",
			in:        []byte{0x00, 0x02, 0x00, 0x11, 0x24, 0x01, 0xdb, 0x00, 0xff, 0xfe, 0x01, 0x23, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want:      nil,
			wantErr:   false,
			wantIPErr: true,
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

func TestClockAccuracyToDuration(t *testing.T) {
	require.Equal(t, time.Nanosecond*25, ClockAccuracyNanosecond25.Duration())
	require.Equal(t, time.Nanosecond*100, ClockAccuracyNanosecond100.Duration())
	require.Equal(t, time.Nanosecond*250, ClockAccuracyNanosecond250.Duration())
	require.Equal(t, time.Microsecond, ClockAccuracyMicrosecond1.Duration())
	require.Equal(t, time.Nanosecond*2500, ClockAccuracyMicrosecond2point5.Duration())
	require.Equal(t, time.Microsecond*10, ClockAccuracyMicrosecond10.Duration())
	require.Equal(t, time.Microsecond*25, ClockAccuracyMicrosecond25.Duration())
	require.Equal(t, time.Microsecond*100, ClockAccuracyMicrosecond100.Duration())
	require.Equal(t, time.Microsecond*250, ClockAccuracyMicrosecond250.Duration())
	require.Equal(t, time.Millisecond, ClockAccuracyMillisecond1.Duration())
	require.Equal(t, time.Microsecond*2500, ClockAccuracyMillisecond2point5.Duration())
	require.Equal(t, time.Millisecond*10, ClockAccuracyMillisecond10.Duration())
	require.Equal(t, time.Millisecond*25, ClockAccuracyMillisecond25.Duration())
	require.Equal(t, time.Millisecond*100, ClockAccuracyMillisecond100.Duration())
	require.Equal(t, time.Millisecond*250, ClockAccuracyMillisecond250.Duration())
	require.Equal(t, time.Second, ClockAccuracySecond1.Duration())
	require.Equal(t, time.Second*10, ClockAccuracySecond10.Duration())
	require.Equal(t, time.Second*25, ClockAccuracySecondGreater10.Duration())
}
