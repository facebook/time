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

package chrony

import (
	"fmt"
	"math"
	"net"
	"strconv"
	"time"
)

// ChronySocketPath is the default path to chronyd socket
const ChronySocketPath = "/var/run/chrony/chronyd.sock"

// ChronyPortV6Regexp is a regexp to find anything that listens on port 323
// hex(323) = '0x143'
const ChronyPortV6Regexp = "[0-9]+: [0-9A-Z]+:0143 .*"

// This is used in timeSpec.SecHigh for 32-bit timestamps
const noHighSec uint32 = 0x7fffffff

// IPAddr family constants - corresponds to IPAddr_Family in chrony's addressing.h
// https://gitlab.com/chrony/chrony/-/blob/master/addressing.h
const (
	IPAddrUnspec uint16 = 0
	IPAddrInet4  uint16 = 1
	IPAddrInet6  uint16 = 2
	IPAddrID     uint16 = 3
)

// magic numbers to convert chronyFloat to normal float
const (
	floatExpBits  = 7
	floatCoefBits = (4*8 - floatExpBits)
)

// IPAddr represents a chrony IP address which can be IPv4, IPv6, or an
// unresolved ID (for sources that haven't been resolved yet).
// The struct layout matches chrony's wire format for binary serialization.
type IPAddr struct {
	IP     [16]uint8
	Family uint16
	Pad    uint16
}

// ToNetIP returns the underlying net.IP for resolved addresses.
// Returns nil for unresolved addresses (IPAddrID family) or unspecified addresses.
func (ip *IPAddr) ToNetIP() net.IP {
	switch ip.Family {
	case IPAddrInet4:
		return net.IP(ip.IP[:4])
	case IPAddrInet6:
		return net.IP(ip.IP[:])
	default:
		// IPAddrUnspec (0) and IPAddrID (3) don't represent actual IP addresses.
		// IPAddrID is used for unresolved addresses identified by an ID number.
		return nil
	}
}

// String returns a human-readable representation of the IP address.
// For IPAddrID family (unresolved addresses), it returns "ID#XXXXXXXXXX" format
// similar to chronyc's output with the -a flag.
func (ip *IPAddr) String() string {
	switch ip.Family {
	case IPAddrInet4:
		return net.IP(ip.IP[:4]).String()
	case IPAddrInet6:
		return net.IP(ip.IP[:]).String()
	case IPAddrID:
		// Format like chronyc: "ID#XXXXXXXXXX" (10 hex digits)
		// The ID is stored in the first 4 bytes of the IP field as big-endian uint32
		id := uint32(ip.IP[0])<<24 | uint32(ip.IP[1])<<16 | uint32(ip.IP[2])<<8 | uint32(ip.IP[3])
		return fmt.Sprintf("ID#%010X", id)
	default:
		return ""
	}
}

type timeSpec struct {
	SecHigh uint32
	SecLow  uint32
	Nsec    uint32
}

func (t *timeSpec) ToTime() time.Time {
	highU64 := uint64(t.SecHigh)
	if t.SecHigh == noHighSec {
		highU64 = 0
	}
	lowU64 := uint64(t.SecLow)
	return time.Unix(int64(highU64<<32|lowU64), int64(t.Nsec))
}

/*
32-bit floating-point format consisting of 7-bit signed exponent
and 25-bit signed coefficient without hidden bit.
The result is calculated as: 2^(exp - 25) * coef
*/
type chronyFloat int32

// ToFloat does magic to decode float from int32.
// Code is copied and translated to Go from original C sources.
func (f chronyFloat) ToFloat() float64 {
	var exp, coef int32

	x := uint32(f)

	exp = int32(x >> floatCoefBits)
	if exp >= 1<<(floatExpBits-1) {
		exp -= 1 << floatExpBits
	}
	exp -= floatCoefBits

	coef = int32(x % (1 << floatCoefBits))
	if coef >= 1<<(floatCoefBits-1) {
		coef -= 1 << floatCoefBits
	}

	return float64(coef) * math.Pow(2.0, float64(exp))
}

// RefidAsHEX prints ref id as hex
func RefidAsHEX(refID uint32) string {
	return fmt.Sprintf("%08X", refID)
}

// RefidToString decodes ASCII string encoded as uint32
func RefidToString(refID uint32) string {
	result := []rune{}

	for i := range 4 {
		c := rune((refID >> (24 - uint(i)*8)) & 0xff)
		if c == 0 {
			continue
		}
		if strconv.IsPrint(c) {
			result = append(result, c)
		} else {
			return RefidAsHEX(refID)
		}
	}

	return string(result)
}

/* NTP tests from RFC 5905:
   +--------------------------+----------------------------------------+
   | Packet Type              | Description                            |
   +--------------------------+----------------------------------------+
   | 1 duplicate packet       | The packet is at best an old duplicate |
   |                          | or at worst a replay by a hacker.      |
   |                          | This can happen in symmetric modes if  |
   |                          | the poll intervals are uneven.         |
   | 2 bogus packet           |                                        |
   | 3 invalid                | One or more timestamp fields are       |
   |                          | invalid. This normally happens in      |
   |                          | symmetric modes when one peer sends    |
   |                          | the first packet to the other and      |
   |                          | before the other has received its      |
   |                          | first reply.                           |
   | 4 access denied          | The access controls have blacklisted   |
   |                          | the source.                            |
   | 5 authentication failure | The cryptographic message digest does  |
   |                          | not match the MAC.                     |
   | 6 unsynchronized         | The server is not synchronized to a    |
   |                          | valid source.                          |
   | 7 bad header data        | One or more header fields are invalid. |
   +--------------------------+----------------------------------------+

chrony doesn't do test #4, but adds four extra tests:
* maximum delay
* delay ratio
* delay dev ratio
* synchronisation loop.

Those tests are roughly equivalent to ntpd 'flashers'
*/

// NTPTestDescMap maps bit mask with corresponding flash status
var NTPTestDescMap = map[uint16]string{
	0x0001: "pkt_dup",
	0x0002: "pkt_bogus",
	0x0004: "pkt_invalid",
	0x0008: "pkt_auth",
	0x0010: "pkt_stratum",
	0x0020: "pkt_header",
	0x0040: "tst_max_delay",
	0x0080: "tst_delay_ratio",
	0x0100: "tst_delay_dev_ration",
	0x0200: "tst_sync_loop",
}

// ReadNTPTestFlags returns list of failed ntp test flags (as strings)
func ReadNTPTestFlags(flags uint16) []string {
	testFlags := flags & NTPFlagsTests
	results := []string{}
	for mask, message := range NTPTestDescMap {
		if testFlags&mask == 0 {
			results = append(results, message)
		}
	}
	return results
}

// IPToBytes converts a net.IP to a [16]uint8 array for use in IPAddr structs.
// For IPv4, it stores the 4 bytes at the beginning of the array.
// For IPv6, it uses all 16 bytes.
func IPToBytes(ip net.IP) [16]uint8 {
	var result [16]uint8
	if ip4 := ip.To4(); ip4 != nil {
		copy(result[:], ip4)
	} else {
		copy(result[:], ip)
	}
	return result
}
