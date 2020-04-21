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
	"fmt"
	"net"

	"github.com/jsimonetti/rtnetlink/rtnl"
	errors "github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const bitsInBytes = 8

// ipv4Mask is a mask we will be assigning to the IPv4 address in interface
const ipv4Mask = 32

// ipv6Mask is a mask we will be assigning to the IPv4 address in interface
const ipv6Mask = 64

// ipv4Len is the IPv4 len in bits
const ipv4Len = net.IPv4len * bitsInBytes

// ipv6Len is the IPv6 len in bits
const ipv6Len = net.IPv6len * bitsInBytes

// AddIPOnInterface adds ip to interface
func (s *Server) addIPToInterface(vip net.IP) error {
	log.Debugf("Adding %s to %s", vip, s.ListenConfig.Iface)
	// Add IPs to the interface
	iface, err := net.InterfaceByName(s.ListenConfig.Iface)
	if err != nil {
		return fmt.Errorf("failed to add IP to the %s interface: %w", iface, err)
	}

	return addIfaceIP(iface, &vip)
}

// deleteIPFromInterface deletes ip from interface
func (s *Server) deleteIPFromInterface(vip net.IP) error {
	log.Debugf("Deleting %s to %s", vip, s.ListenConfig.Iface)
	// Delete IPs to the interface
	iface, err := net.InterfaceByName(s.ListenConfig.Iface)
	if err != nil {
		return err
	}

	return deleteIfaceIP(iface, &vip)
}

// DeleteAllIPs deletes all IPs from interface specified in config
func (s *Server) DeleteAllIPs() {
	for _, vip := range s.ListenConfig.IPs {
		if err := s.deleteIPFromInterface(vip); err != nil {
			// Don't return error. Continue deleting
			log.Errorf("[server]: %v", err)
		}
	}
}

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

// checkIP checks if IP is assigned to the interface already
func checkIP(iface *net.Interface, addr *net.IP) (bool, error) {
	iaddrs, err := iface.Addrs()
	if err != nil {
		return false, err
	}
	for _, iaddr := range iaddrs {
		var ip net.IP
		switch v := iaddr.(type) {
		case *net.IPAddr:
			ip = v.IP
		case *net.IPNet:
			ip = v.IP
		default:
			continue
		}

		if ip.Equal(*addr) {
			return true, nil
		}
	}
	return false, nil
}
