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
	"bytes"
	"encoding/binary"
	"net"
	"time"
	"unsafe"

	syscall "golang.org/x/sys/unix"
)

// PacketSizeBytes sets the size of NTP packet
const PacketSizeBytes = 48

// ControlHeaderSizeBytes is a buffer to read packet header with Kernel timestamps
const ControlHeaderSizeBytes = 32

// Packet is an NTPv4 packet
/*
http://seriot.ch/ntp.php
https://tools.ietf.org/html/rfc958
   0                   1                   2                   3
   0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
0 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
  |LI | VN  |Mode |    Stratum     |     Poll      |  Precision   |
4 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
  |                         Root Delay                            |
8 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
  |                         Root Dispersion                       |
12+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
  |                          Reference ID                         |
16+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
  |                                                               |
  +                     Reference Timestamp (64)                  +
  |                                                               |
24+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
  |                                                               |
  +                      Origin Timestamp (64)                    +
  |                                                               |
32+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
  |                                                               |
  +                      Receive Timestamp (64)                   +
  |                                                               |
40+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
  |                                                               |
  +                      Transmit Timestamp (64)                  +
  |                                                               |
48+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

 0 1 2 3 4 5 6 7
+-+-+-+-+-+-+-+-+
|LI | VN  |Mode |
+-+-+-+-+-+-+-+-+
 0 1 1 0 0 0 1 1

Setting = LI | VN  |Mode. Client request example:
00 011 011 (or 0x1B)
|  |   +-- client mode (3)
|  + ----- version (3)
+ -------- leap year indicator, 0 no warning
*/
type Packet struct {
	Settings       uint8  // leap year indicator, version number and mode
	Stratum        uint8  // stratum
	Poll           int8   // poll. Power of 2
	Precision      int8   // precision. Power of 2
	RootDelay      uint32 // total delay to the reference clock
	RootDispersion uint32 // total dispersion to the reference clock
	ReferenceID    uint32 // identifier of server or a reference clock
	RefTimeSec     uint32 // last time local clock was updated sec
	RefTimeFrac    uint32 // last time local clock was updated frac
	OrigTimeSec    uint32 // client time sec
	OrigTimeFrac   uint32 // client time frac
	RxTimeSec      uint32 // receive time sec
	RxTimeFrac     uint32 // receive time frac
	TxTimeSec      uint32 // transmit time sec
	TxTimeFrac     uint32 // transmit time frac
}

const (
	liNoWarning      = 0
	liAlarmCondition = 3
	vnFirst          = 1
	vnLast           = 4
	modeClient       = 3
)

// ValidSettingsFormat verifies that LI | VN  |Mode fields are set correctly
// check the first byte,include:
// LN:must be 0 or 3
// VN:must be 1,2,3 or 4
// Mode:must be 3
func (p *Packet) ValidSettingsFormat() bool {
	settings := p.Settings
	var l = settings >> 6
	var v = (settings << 2) >> 5
	var m = (settings << 5) >> 5
	if (l == liNoWarning) || (l == liAlarmCondition) {
		if (v >= vnFirst) && (v <= vnLast) {
			if m == modeClient {
				return true
			}
		}
	}
	return false
}

// Bytes converts Packet to []bytes
func (p *Packet) Bytes() ([]byte, error) {
	var bytes bytes.Buffer
	err := binary.Write(&bytes, binary.BigEndian, p)
	return bytes.Bytes(), err
}

// BytesToPacket converts []bytes to Packet
func BytesToPacket(ntpPacketBytes []byte) (*Packet, error) {
	packet := &Packet{}
	reader := bytes.NewReader(ntpPacketBytes)
	err := binary.Read(reader, binary.BigEndian, packet)
	return packet, err
}

// ReadNTPPacket reads incoming NTP packet
func ReadNTPPacket(conn *net.UDPConn) (ntp *Packet, remAddr net.Addr, err error) {
	buf := make([]byte, PacketSizeBytes)
	_, remAddr, err = conn.ReadFromUDP(buf)
	if err != nil {
		return nil, nil, err
	}
	ntp, err = BytesToPacket(buf)

	return ntp, remAddr, err
}

// ReadPacketWithKernelTimestamp reads kernel timestamp from incoming packet
func ReadPacketWithKernelTimestamp(conn *net.UDPConn) (ntp *Packet, kernelRxTime time.Time, remAddr net.Addr, err error) {
	buf := make([]byte, PacketSizeBytes)
	oob := make([]byte, ControlHeaderSizeBytes)

	// Receive message + control struct from the socket
	// https://linux.die.net/man/2/recvmsg
	// This is a low-level way of getting the message (NTP packet content)
	// Additionally we receive control headers, one of which is kernel timestamp
	_, _, _, sa, err := conn.ReadMsgUDP(buf, oob)
	if err != nil {
		return nil, time.Time{}, nil, err
	}
	// Extract kernel timestamp from control fields
	ts := (*syscall.Timespec)(unsafe.Pointer(&oob[syscall.CmsgSpace(0)]))
	kernelRxTime = time.Unix(ts.Unix())

	packet, err := BytesToPacket(buf)
	return packet, kernelRxTime, sa, err
}
