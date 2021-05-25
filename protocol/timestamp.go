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

// Here we have basic HW and SW timestamping support

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	// HWTSTAMP_TX_ON int 1
	hwtstampTXON = 0x00000001
	// HWTSTAMP_FILTER_ALL int 1
	hwtstampFilterAll = 0x00000001
	// Control is a socket control message containing TX/RX timestamp
	// If the read fails we may endup with multiple timestamps in the buffer
	// which is best to read right away
	controlSizeBytes = 128
	// ptp packets usually up to 66 bytes
	payloadSizeBytes = 128
	// look only for X sequential TS
	maxTXTS = 100
)

const (
	// HWTIMESTAMP is a hardware timestamp
	HWTIMESTAMP = "hardware"
	// SWTIMESTAMP is a software timestmap
	SWTIMESTAMP = "software"
)

// Ifreq is a struct for ioctl ethernet manipulation syscalls.
type ifreq struct {
	name [unix.IFNAMSIZ]byte
	data uintptr
}

type hwtstampСonfig struct {
	flags    int
	txType   int
	rxFilter int
}

// connFd returns file descriptor of a connection
func connFd(conn *net.UDPConn) (int, error) {
	sc, err := conn.SyscallConn()
	if err != nil {
		return -1, err
	}
	var intfd int
	err = sc.Control(func(fd uintptr) {
		intfd = int(fd)
	})
	if err != nil {
		return -1, err
	}
	return intfd, nil
}

func ioctlTimestamp(fd int, ifname string) error {
	hw := &hwtstampСonfig{
    flags:    0,
		txType:   hwtstampTXON,
		rxFilter: hwtstampFilterAll,
	}
	i := &ifreq{data: uintptr(unsafe.Pointer(hw))}
	copy(i.name[:unix.IFNAMSIZ-1], ifname)

	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.SIOCSHWTSTAMP, uintptr(unsafe.Pointer(i))); errno != 0 {
		return fmt.Errorf("failed to run ioctl SIOCSHWTSTAMP: %d", errno)
	}
	return nil
}

// EnableHWTimestampsSocket enables HW timestamps on the socket
func EnableHWTimestampsSocket(conn *net.UDPConn, iface string) error {
	// Get socket fd
	connfd, err := connFd(conn)
	if err != nil {
		return err
	}

	if err := ioctlTimestamp(connfd, iface); err != nil {
		return err
	}

	// Enable hardware timestamp capabilities on socket
	flags := unix.SOF_TIMESTAMPING_TX_HARDWARE |
		unix.SOF_TIMESTAMPING_RX_HARDWARE |
		unix.SOF_TIMESTAMPING_RAW_HARDWARE
	// Allow reading of HW timestamps via socket
	if err := unix.SetsockoptInt(connfd, unix.SOL_SOCKET, unix.SO_TIMESTAMPING, flags); err != nil {
		return err
	}

	if err := unix.SetsockoptInt(connfd, unix.SOL_SOCKET, unix.SO_SELECT_ERR_QUEUE, 1); err != nil {
		return err
	}
	return nil
}

// EnableSWTimestampsSocket enables SW timestamps on the socket
func EnableSWTimestampsSocket(conn *net.UDPConn) error {
	// Get socket fd
	connfd, err := connFd(conn)
	if err != nil {
		return err
	}

	flags := unix.SOF_TIMESTAMPING_TX_SOFTWARE |
		unix.SOF_TIMESTAMPING_RX_SOFTWARE |
		unix.SOF_TIMESTAMPING_SOFTWARE
	// Allow reading of SW timestamps via socket
	if err := unix.SetsockoptInt(connfd, unix.SOL_SOCKET, unix.SO_TIMESTAMPING, flags); err != nil {
		return err
	}

	if err := unix.SetsockoptInt(connfd, unix.SOL_SOCKET, unix.SO_SELECT_ERR_QUEUE, 1); err != nil {
		return err
	}
	return nil
}

// byteToTime converts LittleEndian bytes into a timestamp
func byteToTime(data []byte) (time.Time, error) {
	ts := &unix.Timespec{}
	b := bytes.NewReader(data)
	if err := binary.Read(b, binary.LittleEndian, ts); err != nil {
		return time.Time{}, err
	}
	return time.Unix(ts.Unix()), nil
}

/*
scmDataToTime parses SocketControlMessage Data field into time.Time.
The structure can return up to three timestamps. This is a legacy
feature. Only one field is non-zero at any time. Most timestamps
are passed in ts[0]. Hardware timestamps are passed in ts[2].
*/
func scmDataToTime(data []byte) (ts time.Time, err error) {
	// first, try to use hardware timestamps
	ts, err = byteToTime(data[32:48])
	if err != nil {
		return ts, err
	}
	// if hw timestamps aren't present, use software timestamps
	// we can't use ts.IsZero because for some crazy reason timestamp parsed using time.Unix()
	// reports IsZero() == false, even if seconds and nanoseconds are zero.
	if ts.UnixNano() == 0 {
		ts, err = byteToTime(data[0:16])
		if err != nil {
			return ts, err
		}
		if ts.UnixNano() == 0 {
			return ts, fmt.Errorf("got zero timestamp")
		}
	}

	return ts, nil
}

func waitForHWTS(conn *net.UDPConn) error {
	// Get socket fd
	connfd, err := connFd(conn)
	if err != nil {
		return err
	}

	// Wait until TX timestamp is ready
	fds := []unix.PollFd{{Fd: int32(connfd), Events: unix.POLLPRI, Revents: 0}}
	if _, err = unix.Poll(fds, 1); err != nil { // Timeout is 1 ms
		return err
	}
	return nil
}

// ReadTXtimestamp returns HW TX timestamp
func ReadTXtimestamp(conn *net.UDPConn) (time.Time, int, error) {
	// Get socket fd
	connfd, err := connFd(conn)
	if err != nil {
		return time.Time{}, 0, err
	}

	// Accessing hw timestamp
	buf := make([]byte, payloadSizeBytes)
	oob := make([]byte, controlSizeBytes)
	var boob int

	// TMP buffers
	tbuf := make([]byte, payloadSizeBytes)
	toob := make([]byte, controlSizeBytes)

	txfound := false

	// Sometimes we end up with more than 1 TX TS in the buffer.
	// We need to empty it and completely otherwise we end up with a shifted queue read:
	// Sync is out -> read TS from the previous Sync
	// Because we always perform at least 2 tries we start with 0 so on success we are at 1.
	attempts := 0
	for ; attempts < maxTXTS; attempts++ {
		if !txfound {
			// Wait for the poll event, ignore the error
      _ = waitForHWTS(conn)
		}

		_, tboob, _, _, err := unix.Recvmsg(connfd, tbuf, toob, unix.MSG_ERRQUEUE)
		if err != nil {
			// We've already seen the valid TX TS and now we have an empty queue.
			// All good
			if txfound {
				break
			}
			// Keep looking for a valid TX TS
			continue
		}
		// We found a valid TX TS. Still check more if there is a newer one
		txfound = true
		boob = tboob
		copy(buf, tbuf)
		copy(oob, toob)
	}

	if !txfound {
		return time.Time{}, attempts, fmt.Errorf("no TX timestamp found after %d tries", maxTXTS)
	}

	ms, err := unix.ParseSocketControlMessage(oob[:boob])
	if err != nil {
		return time.Time{}, 0, err
	}

	for _, m := range ms {
		// There are different socket control messages in the queue from time to time
		if m.Header.Level == unix.SOL_SOCKET && m.Header.Type == unix.SO_TIMESTAMPING {
			timestamp, err := scmDataToTime(m.Data)
			if err != nil {
				return time.Time{}, 0, err
			}

			return timestamp, attempts, nil
		}
	}
	return time.Time{}, 0, fmt.Errorf("failed to find HW timestamp in MSG_ERRQUEUE")
}

// ReadPacketWithRXTimestamp returns byte packet and HW RX timestamp
func ReadPacketWithRXTimestamp(conn *net.UDPConn) ([]byte, net.IP, time.Time, error) {
	// Accessing hw timestamp
	buf := make([]byte, payloadSizeBytes)
	oob := make([]byte, controlSizeBytes)

	bbuf, boob, _, addr, err := conn.ReadMsgUDP(buf, oob)
	if err != nil {
		return nil, nil, time.Time{}, fmt.Errorf("failed to read timestamp: %v", err)
	}

	ms, err := unix.ParseSocketControlMessage(oob[:boob])
	if err != nil {
		return nil, nil, time.Time{}, err
	}

	for _, m := range ms {
		// There are different socket control messages in the queue from time to time
		if m.Header.Level == unix.SOL_SOCKET && m.Header.Type == unix.SO_TIMESTAMPING {
			timestamp, err := scmDataToTime(m.Data)
			if err != nil {
				return nil, addr.IP, time.Time{}, err
			}

			return buf[:bbuf], addr.IP, timestamp, nil
		}
	}
	return nil, nil, time.Time{}, fmt.Errorf("failed to find HW timestamp in MSG_ERRQUEUE")
}
