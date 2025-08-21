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
	"encoding/binary"
	"fmt"
	"net"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// unix.Cmsghdr size differs depending on platform
var socketControlMessageHeaderOffset = binary.Size(unix.Cmsghdr{})

var timestamping = unix.SO_TIMESTAMP

// Here we have basic HW and SW timestamping support

// byteToTime converts bytes into a timestamp
func byteToTime(data []byte) (time.Time, error) {
	// freebsd supports only SO_TIMESTAMP mode, which returns timeval
	timeval := (*unix.Timeval)(unsafe.Pointer(&data[0]))
	return time.Unix(timeval.Unix()), nil
}

/*
scmDataToTime parses SocketControlMessage Data field into time.Time.
*/
func scmDataToTime(data []byte) (ts time.Time, err error) {
	size := binary.Size(unix.Timeval{})

	ts, err = byteToTime(data[0:size])
	if err != nil {
		return ts, err
	}
	if ts.UnixNano() == 0 {
		return ts, fmt.Errorf("got zero timestamp")
	}

	return ts, nil
}

// socketControlMessageTimestamp parses timestamp from control message
func socketControlMessageTimestamp(b []byte) (time.Time, error) {
	return scmDataToTime(b[unix.CmsgSpace(0):])
}

// EnableSWTimestampsRx enables SW RX timestamps on the socket
func EnableSWTimestampsRx(connFd int) error {
	// Allow reading of SW timestamps via socket
	return unix.SetsockoptInt(connFd, unix.SOL_SOCKET, timestamping, 1)
}

// EnableTimestamps enables timestamps on the socket based on requested type
func EnableTimestamps(ts Timestamp, connFd int, _ *net.Interface) error {
	switch ts {
	case SW:
		if err := EnableSWTimestampsRx(connFd); err != nil {
			return fmt.Errorf("Cannot enable software timestamps: %w", err)
		}
	default:
		return fmt.Errorf("Unrecognized timestamp type: %s", ts)
	}
	return nil
}
