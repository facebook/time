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

package timestamp

import (
	"fmt"
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func Test_byteToTime(t *testing.T) {
	timeb := []byte{63, 155, 21, 96, 0, 0, 0, 0, 52, 156, 191, 42, 0, 0, 0, 0}
	res, err := byteToTime(timeb)
	require.Nil(t, err)

	require.Equal(t, int64(1612028735717200436), res.UnixNano())
}

func Test_ReadTXtimestamp(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.Nil(t, err)
	defer conn.Close()

	connFd, err := ConnFd(conn)
	require.Nil(t, err)

	txts, attempts, err := ReadTXtimestamp(connFd)
	require.Equal(t, time.Time{}, txts)
	require.Equal(t, maxTXTS, attempts)
	require.Equal(t, fmt.Errorf("no TX timestamp found after %d tries", maxTXTS), err)

	err = EnableSWTimestamps(connFd)
	require.Nil(t, err)

	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
	_, err = conn.WriteTo([]byte{}, addr)
	require.Nil(t, err)
	txts, attempts, err = ReadTXtimestamp(connFd)

	require.NotEqual(t, time.Time{}, txts)
	require.Equal(t, 1, attempts)
	require.Nil(t, err)
}

func Test_scmDataToTime(t *testing.T) {
	hwData := []byte{
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		63, 155, 21, 96, 0, 0, 0, 0, 52, 156, 191, 42, 0, 0, 0, 0,
	}
	swData := []byte{
		63, 155, 21, 96, 0, 0, 0, 0, 52, 156, 191, 42, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	}
	noData := []byte{
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	}

	tests := []struct {
		name    string
		data    []byte
		want    int64
		wantErr bool
	}{
		{
			name:    "hardware timestamp",
			data:    hwData,
			want:    1612028735717200436,
			wantErr: false,
		},
		{
			name:    "software timestamp",
			data:    swData,
			want:    1612028735717200436,
			wantErr: false,
		},
		{
			name:    "zero timestamp",
			data:    noData,
			want:    0,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := scmDataToTime(tt.data)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.Nil(t, err)
				require.Equal(t, tt.want, res.UnixNano())
			}
		})
	}
}

func TestEnableSWTimestampsRx(t *testing.T) {
	// listen to incoming udp packets
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)
	defer conn.Close()

	connFd, err := ConnFd(conn)
	require.NoError(t, err)

	// Allow reading of kernel timestamps via socket
	err = EnableSWTimestampsRx(connFd)
	require.NoError(t, err)

	// Check that socket option is set
	timestampsEnabled, _ := unix.GetsockoptInt(connFd, unix.SOL_SOCKET, unix.SO_TIMESTAMPING)
	newTimestampsEnabled, _ := unix.GetsockoptInt(connFd, unix.SOL_SOCKET, unix.SO_TIMESTAMPING_NEW)

	// At least one of them should be set, which it > 0
	require.Greater(t, timestampsEnabled+newTimestampsEnabled, 0, "None of the socket options is set")
}

func TestEnableSWTimestamps(t *testing.T) {
	// listen to incoming udp packets
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)
	defer conn.Close()

	connFd, err := ConnFd(conn)
	require.NoError(t, err)

	// Allow reading of kernel timestamps via socket
	err = EnableSWTimestamps(connFd)
	require.NoError(t, err)

	// Check that socket option is set
	timestampsEnabled, _ := unix.GetsockoptInt(connFd, unix.SOL_SOCKET, unix.SO_TIMESTAMPING)
	newTimestampsEnabled, _ := unix.GetsockoptInt(connFd, unix.SOL_SOCKET, unix.SO_TIMESTAMPING_NEW)

	// At least one of them should be set, which it > 0
	require.Greater(t, timestampsEnabled+newTimestampsEnabled, 0, "None of the socket options is set")
}

func TestSocketControlMessageTimestamp(t *testing.T) {
	if timestamping != unix.SO_TIMESTAMPING_NEW {
		t.Skip("This test supports SO_TIMESTAMPING_NEW only. No sample of SO_TIMESTAMPING")
	}

	var b []byte

	// unix.Cmsghdr used in socketControlMessageTimestamp differs depending on platform
	switch runtime.GOARCH {
	case "amd64":
		b = []byte{60, 0, 0, 0, 0, 0, 0, 0, 41, 0, 0, 0, 25, 0, 0, 0, 42, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 64, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 65, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 230, 180, 10, 97, 0, 0, 0, 0, 239, 83, 199, 39, 0, 0, 0, 0}
	case "386":
		b = []byte{56, 0, 0, 0, 41, 0, 0, 0, 25, 0, 0, 0, 42, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 60, 0, 0, 0, 1, 0, 0, 0, 65, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 230, 180, 10, 97, 0, 0, 0, 0, 239, 83, 199, 39, 0, 0, 0, 0}
	default:
		t.Skip("This test supports amd64/386 platforms only")
	}

	ts, err := socketControlMessageTimestamp(b)
	require.NoError(t, err)
	require.Equal(t, int64(1628091622667374575), ts.UnixNano())
}

func TestReadPacketWithRXTimestamp(t *testing.T) {
	request := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 42}
	// listen to incoming udp packets
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("localhost"), Port: 0})
	require.NoError(t, err)
	defer conn.Close()

	// get connection file descriptor
	connFd, err := ConnFd(conn)
	require.NoError(t, err)

	// Allow reading of kernel timestamps via socket
	err = EnableSWTimestampsRx(connFd)
	require.NoError(t, err)

	err = unix.SetNonblock(connFd, false)
	require.NoError(t, err)

	// Send a client request
	timeout := 1 * time.Second
	cconn, err := net.DialTimeout("udp", conn.LocalAddr().String(), timeout)
	require.NoError(t, err)
	defer cconn.Close()
	_, err = cconn.Write(request)
	require.NoError(t, err)

	// read kernel timestamp from incoming packet
	data, returnaddr, nowKernelTimestamp, err := ReadPacketWithRXTimestamp(connFd)
	require.Equal(t, request, data, "We should have the same request arriving on the server")
	require.Equal(t, time.Now().Unix()/10, nowKernelTimestamp.Unix()/10, "kernel timestamps should be within 10s")
	requireEqualNetAddrSockAddr(t, cconn.LocalAddr(), returnaddr)
	require.NoError(t, err)
}

func TestReadPacketWithRXTXTimestamp(t *testing.T) {
	request := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 42}
	// listen to incoming udp packets
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("localhost"), Port: 0})
	require.NoError(t, err)
	defer conn.Close()

	// get connection file descriptor
	connFd, err := ConnFd(conn)
	require.NoError(t, err)

	// Allow reading of kernel timestamps via socket
	err = EnableSWTimestamps(connFd)
	require.NoError(t, err)

	err = unix.SetNonblock(connFd, false)
	require.NoError(t, err)

	// Send a client request
	timeout := 1 * time.Second
	cconn, err := net.DialTimeout("udp", conn.LocalAddr().String(), timeout)
	require.NoError(t, err)
	defer cconn.Close()
	_, err = cconn.Write(request)
	require.NoError(t, err)

	// read kernel timestamp from incoming packet
	data, returnaddr, nowKernelTimestamp, err := ReadPacketWithRXTimestamp(connFd)
	require.NoError(t, err)
	require.Equal(t, request, data, "We should have the same request arriving on the server")
	require.Equal(t, time.Now().Unix()/10, nowKernelTimestamp.Unix()/10, "kernel timestamps should be within 10s")
	requireEqualNetAddrSockAddr(t, cconn.LocalAddr(), returnaddr)

	// send packet and read TX timestamp
	_, err = conn.WriteTo(request, cconn.LocalAddr())
	require.NoError(t, err)
	txts, attempts, err := ReadTXtimestamp(connFd)
	require.NotEqual(t, time.Time{}, txts)
	require.Equal(t, 1, attempts)
	require.Nil(t, err)
}
