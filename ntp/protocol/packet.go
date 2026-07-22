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
	"encoding/binary"
	"fmt"
	"net"
)

// PacketSizeBytes sets the size of NTP packet
const PacketSizeBytes = 48

// ControlHeaderSizeBytes is a buffer to read packet header with Kernel timestamps
const ControlHeaderSizeBytes = 32

// maxUDPPacketSizeBytes bounds a single UDP read. Plain NTP is 48 octets, but an
// NTS-protected packet also carries extension fields, so reads are sized to a
// full 1500-octet MTU to avoid silently truncating them.
const maxUDPPacketSizeBytes = 1500

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
	Settings        uint8            // leap year indicator, version number and mode
	Stratum         uint8            // stratum
	Poll            int8             // poll. Power of 2
	Precision       int8             // precision. Power of 2
	RootDelay       uint32           // total delay to the reference clock
	RootDispersion  uint32           // total dispersion to the reference clock
	ReferenceID     uint32           // identifier of server or a reference clock
	RefTimeSec      uint32           // last time local clock was updated sec
	RefTimeFrac     uint32           // last time local clock was updated frac
	OrigTimeSec     uint32           // client time sec
	OrigTimeFrac    uint32           // client time frac
	RxTimeSec       uint32           // receive time sec
	RxTimeFrac      uint32           // receive time frac
	TxTimeSec       uint32           // transmit time sec
	TxTimeFrac      uint32           // transmit time frac
	ExtensionFields []ExtensionField // Empty for plain NTP
}

const (
	liNoWarning      = 0
	liAlarmCondition = 3
	vnFirst          = 1
	vnLast           = 4
	modeClient       = 3
)

// putHeader writes the fixed 48-octet NTP header (RFC 5905) into dst in wire
// order, big-endian. dst must be at least PacketSizeBytes long. This hand-rolled
// codec replaces reflection-based binary.Write over a []any field list, keeping
// the hot request/response path allocation-free.
func (p *Packet) putHeader(dst []byte) {
	_ = dst[PacketSizeBytes-1] // bounds-check hint: panic early if dst too short
	dst[0] = p.Settings
	dst[1] = p.Stratum
	dst[2] = byte(p.Poll)      //#nosec G115
	dst[3] = byte(p.Precision) //#nosec G115
	binary.BigEndian.PutUint32(dst[4:8], p.RootDelay)
	binary.BigEndian.PutUint32(dst[8:12], p.RootDispersion)
	binary.BigEndian.PutUint32(dst[12:16], p.ReferenceID)
	binary.BigEndian.PutUint32(dst[16:20], p.RefTimeSec)
	binary.BigEndian.PutUint32(dst[20:24], p.RefTimeFrac)
	binary.BigEndian.PutUint32(dst[24:28], p.OrigTimeSec)
	binary.BigEndian.PutUint32(dst[28:32], p.OrigTimeFrac)
	binary.BigEndian.PutUint32(dst[32:36], p.RxTimeSec)
	binary.BigEndian.PutUint32(dst[36:40], p.RxTimeFrac)
	binary.BigEndian.PutUint32(dst[40:44], p.TxTimeSec)
	binary.BigEndian.PutUint32(dst[44:48], p.TxTimeFrac)
}

// readHeader parses the fixed 48-octet NTP header from src, which must be at
// least PacketSizeBytes long, into p.
func (p *Packet) readHeader(src []byte) {
	_ = src[PacketSizeBytes-1] // bounds-check hint
	p.Settings = src[0]
	p.Stratum = src[1]
	p.Poll = int8(src[2])      //#nosec G115
	p.Precision = int8(src[3]) //#nosec G115
	p.RootDelay = binary.BigEndian.Uint32(src[4:8])
	p.RootDispersion = binary.BigEndian.Uint32(src[8:12])
	p.ReferenceID = binary.BigEndian.Uint32(src[12:16])
	p.RefTimeSec = binary.BigEndian.Uint32(src[16:20])
	p.RefTimeFrac = binary.BigEndian.Uint32(src[20:24])
	p.OrigTimeSec = binary.BigEndian.Uint32(src[24:28])
	p.OrigTimeFrac = binary.BigEndian.Uint32(src[28:32])
	p.RxTimeSec = binary.BigEndian.Uint32(src[32:36])
	p.RxTimeFrac = binary.BigEndian.Uint32(src[36:40])
	p.TxTimeSec = binary.BigEndian.Uint32(src[40:44])
	p.TxTimeFrac = binary.BigEndian.Uint32(src[44:48])
}

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

// encodedLen returns the packet's wire size: the 48-octet header plus every
// extension field (including padding).
func (p *Packet) encodedLen() int {
	n := PacketSizeBytes
	for _, ef := range p.ExtensionFields {
		n += ef.EncodedSize()
	}
	return n
}

// Bytes serializes the packet (header + extension fields) into a freshly
// allocated []byte. It uses explicit big-endian offsets (no reflection) and
// works for both plain NTP and NTS packets.
func (p *Packet) Bytes() ([]byte, error) {
	out := make([]byte, p.encodedLen())
	p.putHeader(out)
	if len(p.ExtensionFields) == 0 {
		return out, nil
	}
	if _, err := MarshalExtensionFieldsTo(p.ExtensionFields, out[PacketSizeBytes:]); err != nil {
		return nil, err
	}
	return out, nil
}

// UnmarshalBinary fills the Packet from []bytes
func (p *Packet) UnmarshalBinary(b []byte) error {
	if len(b) < PacketSizeBytes {
		return fmt.Errorf("ntp packet too short: %d bytes", len(b))
	}
	p.readHeader(b)
	if len(b) > PacketSizeBytes {
		efs, err := ParseExtensionFields(b[PacketSizeBytes:])
		if err != nil {
			return err
		}
		p.ExtensionFields = efs
	} else {
		p.ExtensionFields = nil
	}
	return nil
}

// AssociatedData returns the header plus the first n extension fields,
// re-serialized so unrecognized/reserved EFs are reproduced exactly (RFC 8915
// §5.4). Callers MUST NOT mutate ExtensionFields between parse and this call, or
// the reconstructed bytes will no longer match what the peer authenticated.
func (p *Packet) AssociatedData(n int) ([]byte, error) {
	if n < 0 || n > len(p.ExtensionFields) {
		return nil, fmt.Errorf("ntp: associated-data ef index %d out of range [0,%d]", n, len(p.ExtensionFields))
	}
	efs := p.ExtensionFields[:n]
	size := PacketSizeBytes
	for i := range efs {
		if err := efs[i].checkBodySize(); err != nil {
			return nil, err
		}
		size += efs[i].EncodedSize()
	}
	out := make([]byte, size)
	p.putHeader(out)
	writeExtensionFields(out[PacketSizeBytes:], efs)
	return out, nil
}

// BytesToPacket converts []bytes to Packet
func BytesToPacket(ntpPacketBytes []byte) (*Packet, error) {
	packet := &Packet{}
	return packet, packet.UnmarshalBinary(ntpPacketBytes)
}

// ReadNTPPacket reads an incoming NTP packet, including any NTS extension
// fields. The read buffer is a full 1500-octet MTU so extension fields are not
// silently truncated, and the packet is parsed from exactly the bytes received.
func ReadNTPPacket(conn *net.UDPConn) (ntp *Packet, remAddr net.Addr, err error) {
	buf := make([]byte, maxUDPPacketSizeBytes)
	n, remAddr, err := conn.ReadFromUDP(buf)
	if err != nil {
		return nil, nil, err
	}
	ntp, err = BytesToPacket(buf[:n])

	return ntp, remAddr, err
}
