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

package control

import (
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	readStatus    = 1
	readVariables = 2
)

// NormalizeData turns bytes that contain kv ASCII string info a map[string]string
func NormalizeData(data []byte) (map[string]string, error) {
	result := map[string]string{}
	pairs := strings.Split(string(data), ",")
	for _, pair := range pairs {
		split := strings.Split(pair, "=")
		if len(split) != 2 {
			log.Debugf("WARNING: Malformed packet, bad k=v pair '%s'", pair)
			continue
		}
		k := strings.TrimSpace(split[0])
		v := strings.TrimSpace(strings.Trim(split[1], `"`))
		result[k] = v
	}
	if len(result) == 0 {
		return result, errors.Errorf("Malformed packet, no k=v pairs decoded")
	}
	return result, nil
}

// NTPControlMsgHead structure is described in NTPv3 RFC-1119 Appendix B. NTP Control Messages
// for some reason it's missing from more recent NTPv4 RFC-5905.
// We don't have Data defined here as data size is variable and binary package
// simply doesn't support reading or writing structs with non-fixed fields.
type NTPControlMsgHead struct {
	// 0: 00 Version(3bit) Mode(3bit)
	VnMode uint8
	// 1: Response Error More Operation(5bit)
	REMOp uint8
	// 2-3: Sequence (16bit)
	Sequence uint16
	// 4-5: Status (16bit)
	Status uint16
	// 6-7: Association ID (16bit)
	AssociationID uint16
	// 8-9: Offset (16bit)
	Offset uint16
	// 10-11: Count (16bit)
	Count uint16
	// 12+: Data (up to 468 bits)
	// then goes [468]uint8 of data that we have in NTPControlMsg
}

// NTPControlMsg is just a NTPControlMsgHead with data
type NTPControlMsg struct {
	NTPControlMsgHead
	Data []uint8
}

// LeapDesc stores human-readable descriptions of LI (leap indicator) field
var LeapDesc = [4]string{"none", "add_sec", "del_sec", "alarm"}

// ClockSourceDesc stores human-readable descriptions of ClockSource field
var ClockSourceDesc = [10]string{
	"unspec",     // 00
	"pps",        // 01
	"lf_radio",   // 02
	"hf_radio",   // 03
	"uhf_radio",  // 04
	"local",      // 05
	"ntp",        // 06
	"other",      // 07
	"wristwatch", // 08
	"telephone",  // 09
}

// SystemEventDesc stores human-readable descriptions of SystemEvent field
var SystemEventDesc = [17]string{
	"unspecified",             // 00
	"freq_not_set",            // 01
	"freq_set",                // 02
	"spike_detect",            // 03
	"freq_mode",               // 04
	"clock_sync",              // 05
	"restart",                 // 06
	"panic_stop",              // 07
	"no_system_peer",          // 08
	"leap_armed",              // 09
	"leap_disarmed",           // 0a
	"leap_event",              // 0b
	"clock_step",              // 0c
	"kern",                    // 0d
	"TAI...",                  // 0e
	"stale leapsecond values", // 0f
	"clockhop",                // 10
}

// FlashDescMap maps bit mask with corresponding flash status
var FlashDescMap = map[uint16]string{
	0x0001: "pkt_dup",
	0x0002: "pkt_bogus",
	0x0004: "pkt_unsync",
	0x0008: "pkt_denied",
	0x0010: "pkt_auth",
	0x0020: "pkt_stratum",
	0x0040: "pkt_header",
	0x0080: "pkt_autokey",
	0x0100: "pkt_crypto",
	0x0200: "peer_stratum",
	0x0400: "peer_dist",
	0x0800: "peer_loop",
	0x1000: "peer_unreach",
}

// ReadFlashStatusWord returns list of flashers (as strings) decoded from flash status word
func ReadFlashStatusWord(flash uint16) []string {
	flashers := []string{}
	for mask, message := range FlashDescMap {
		if flash&mask > 0 {
			flashers = append(flashers, message)
		}
	}
	return flashers
}

// SystemStatusWord stores parsed SystemStatus 16bit word.
type SystemStatusWord struct {
	LI                 uint8
	ClockSource        uint8
	SystemEventCounter uint8
	SystemEventCode    uint8
}

// ReadSystemStatusWord transforms SystemStatus 16bit word into usable struct.
func ReadSystemStatusWord(b uint16) *SystemStatusWord {
	return &SystemStatusWord{
		LI:                 uint8((b & 0xc000) >> 14), // first 2 bits
		ClockSource:        uint8((b & 0x3f00) >> 8),  // next 6 bits
		SystemEventCounter: uint8((b & 0xf0) >> 4),    // 4 bits
		SystemEventCode:    uint8((b & 0xf)),          // last 4 bits
	}
}

// PeerStatus word decoded. Sadly values used by ntpd are different from RFC for v2 and v3 of NTP.
// Actual values are from http://doc.ntp.org/4.2.6/decode.html#peer
type PeerStatus struct {
	Broadcast   bool
	Reachable   bool
	AuthEnabled bool
	AuthOK      bool
	Configured  bool
}

// PeerSelect maps PeerSelection uint8 to human-readable string taken from http://doc.ntp.org/4.2.6/decode.html#peer
var PeerSelect = [8]string{"reject", "falsetick", "excess", "outlyer", "candidate", "backup", "sys.peer", "pps.peer"}

// ReadPeerStatus transforms PeerStatus 8bit flag into usable struct
func ReadPeerStatus(b uint8) PeerStatus {
	// 5 bit code with bits assigned different meanings
	return PeerStatus{
		Configured:  b&0x10 != 0,
		AuthOK:      b&0x8 != 0,
		AuthEnabled: b&0x4 != 0,
		Reachable:   b&0x2 != 0,
		Broadcast:   b&0x1 != 0,
	}
}

// PeerStatusWord stores parsed PeerStatus 16bit word.
type PeerStatusWord struct {
	PeerStatus       PeerStatus
	PeerSelection    uint8
	PeerEventCounter uint8
	PeerEventCode    uint8
}

// ReadPeerStatusWord transforms PeerStatus 16bit word into usable struct
func ReadPeerStatusWord(b uint16) *PeerStatusWord {
	status := uint8((b & 0xf800) >> 11) // first 5 bits
	return &PeerStatusWord{
		PeerStatus:       ReadPeerStatus(status),
		PeerSelection:    uint8((b & 0x700) >> 8), // 3 bits
		PeerEventCounter: uint8((b & 0xf0) >> 4),  // 4 bits
		PeerEventCode:    uint8((b & 0xf)),        // last 4 bits
	}
}

// GetVersion gets int version from Version+Mode 8bit word
func (n NTPControlMsgHead) GetVersion() int {
	return int((n.VnMode & 0x38) >> 3) // get 3 bits offset by 3 bits
}

// GetMode gets int mode from Version+Mode 8bit word
func (n NTPControlMsgHead) GetMode() int {
	return int(n.VnMode & 0x7) // get last 3 bits
}

// IsResponse returns true if packet is a response
func (n NTPControlMsgHead) IsResponse() bool {
	return n.REMOp&0x80 != 0 // response, bit 7
}

// HasError returns true if packet has error flag set
func (n NTPControlMsgHead) HasError() bool {
	return n.REMOp&0x40 != 0 // error flag, bit 6
}

// HasMore returns true if packet has More flag set
func (n NTPControlMsgHead) HasMore() bool {
	return n.REMOp&0x20 != 0 // more flag, bit 5
}

// GetOperation returns int operation extracted from REMOp 8bit word
func (n NTPControlMsgHead) GetOperation() uint8 {
	return uint8(n.REMOp & 0x1f) // last 5 bits
}

// GetSystemStatus returns parsed SystemStatusWord struct if present
func (n NTPControlMsg) GetSystemStatus() (*SystemStatusWord, error) {
	if n.GetOperation() != readStatus {
		return nil, errors.Errorf("no System Status Word supported for operation=%d", n.GetOperation())
	}
	return ReadSystemStatusWord(n.Status), nil
}

// GetPeerStatus returns parsed PeerStatusWord struct if present
func (n NTPControlMsg) GetPeerStatus() (*PeerStatusWord, error) {
	if n.GetOperation() != readVariables {
		return nil, errors.Errorf("no Peer Status Word supported for operation=%d", n.GetOperation())
	}
	return ReadPeerStatusWord(n.Status), nil
}

// GetAssociations returns map of PeerStatusWord, basically peer information.
func (n NTPControlMsg) GetAssociations() (map[uint16]*PeerStatusWord, error) {
	result := map[uint16]*PeerStatusWord{}
	if n.GetOperation() != readStatus {
		return result, errors.Errorf("no peer list supported for operation=%d", n.GetOperation())
	}
	for i := 0; i < int(n.Count/4); i++ {
		assoc := n.Data[i*4 : i*4+4]                         // 2 uint16 encoded as 4 bytes
		id := uint16(assoc[0])<<8 | uint16(assoc[1])         // uint16 from 2 uint8
		peerStatus := uint16(assoc[2])<<8 | uint16(assoc[3]) // ditto
		result[id] = ReadPeerStatusWord(peerStatus)
	}
	return result, nil
}

// GetAssociationInfo returns parsed normalized variables if present
func (n NTPControlMsg) GetAssociationInfo() (map[string]string, error) {
	result := map[string]string{}
	if n.GetOperation() != readVariables {
		return result, errors.Errorf("no variables supported for operation=%d", n.GetOperation())
	}
	data, err := NormalizeData(n.Data)
	if err != nil {
		return result, err
	}
	return data, nil
}
