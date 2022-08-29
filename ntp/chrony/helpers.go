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
	"time"
	"unicode"
)

// ChronySocketPath is the default path to chronyd socket
const ChronySocketPath = "/var/run/chrony/chronyd.sock"

// ChronyPortV6Regexp is a regexp to find anything that listens on port 323
// hex(323) = '0x143'
const ChronyPortV6Regexp = "[0-9]+: [0-9A-Z]+:0143 .*"

// This is used in timeSpec.SecHigh for 32-bit timestamps
const noHighSec uint32 = 0x7fffffff

// ip stuff
const (
	ipAddrInet4 uint16 = 1
	ipAddrInet6 uint16 = 2
)

// magic numbers to convert chronyFloat to normal float
const (
	floatExpBits  = 7
	floatCoefBits = (4*8 - floatExpBits)
)

type ipAddr struct {
	IP     [16]uint8
	Family uint16
	Pad    uint16
}

func (ip *ipAddr) ToNetIP() net.IP {
	if ip.Family == ipAddrInet4 {
		return net.IP(ip.IP[:4])
	}
	return net.IP(ip.IP[:])
}

func newIPAddr(ip net.IP) *ipAddr {
	family := ipAddrInet6
	if ip.To4() != nil {
		family = ipAddrInet4
	}
	var nIP [16]byte
	copy(nIP[:], ip)
	return &ipAddr{
		IP:     nIP,
		Family: family,
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

	for i := 0; i < 4 && i < 64-1; i++ {
		c := rune((refID >> (24 - uint(i)*8)) & 0xff)
		if unicode.IsPrint(c) {
			result = append(result, c)
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
