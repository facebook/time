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

package server

import (
	"net"
	"time"

	"github.com/facebook/time/phc"
	"github.com/jsimonetti/rtnetlink/rtnl"
	errors "github.com/pkg/errors"
)

// bitsInBytes is a number of bits in byte
const bitsInBytes = 8

// ipv4Len is the IPv4 len in bits
const ipv4Len = net.IPv4len * bitsInBytes

// ipv6Len is the IPv6 len in bits
const ipv6Len = net.IPv6len * bitsInBytes

func addIfaceIP(iface *net.Interface, addr *net.IP) error {
	// Check if IP is assigned:
	assigned, err := checkIP(iface, addr)
	if err != nil {
		return err
	}
	if assigned {
		return nil
	}

	conn, err := rtnl.Dial(nil)
	if err != nil {
		return errors.Wrap(err, "can't establish netlink connection")
	}
	defer conn.Close()

	var mask net.IPMask
	if v4 := addr.To4(); v4 == nil {
		mask = net.CIDRMask(ipv6Mask, ipv6Len)
	} else {
		mask = net.CIDRMask(ipv4Mask, ipv4Len)
	}

	err = conn.AddrAdd(iface, &net.IPNet{IP: *addr, Mask: mask})
	if err != nil {
		return errors.Wrap(err, "can't add address")
	}
	return nil
}

func deleteIfaceIP(iface *net.Interface, addr *net.IP) error {
	// Check if IP is assigned:
	assigned, err := checkIP(iface, addr)
	if err != nil {
		return err
	}
	if !assigned {
		return nil
	}

	conn, err := rtnl.Dial(nil)
	if err != nil {
		return errors.Wrap(err, "can't establish netlink connection")
	}
	defer conn.Close()

	var mask net.IPMask
	if v4 := addr.To4(); v4 == nil {
		mask = net.CIDRMask(ipv6Mask, ipv6Len)
	} else {
		mask = net.CIDRMask(ipv4Mask, ipv4Len)
	}

	err = conn.AddrDel(iface, &net.IPNet{IP: *addr, Mask: mask})
	if err != nil {
		return errors.Wrap(err, "can't remove address")
	}

	return nil
}

// PHCOffset periodically checks for PHC-SYS offset and updates it in the config
func phcOffset(iface string) (time.Duration, error) {
	device, err := phc.DeviceFromIface(iface)
	if err != nil {
		return 0, err
	}

	res, err := phc.TimeAndOffsetFromDevice(device, phc.MethodSyscallClockGettime)
	if err != nil {
		return 0, err
	}
	return res.Offset, nil
}
