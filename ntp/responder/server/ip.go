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

	log "github.com/sirupsen/logrus"
)

// ipv4Mask is a mask we will be assigning to the IPv4 address in interface
const ipv4Mask = 32

// ipv6Mask is a mask we will be assigning to the IPv4 address in interface
const ipv6Mask = 64

// AddIPOnInterface adds ip to interface
func (s *Server) addIPToInterface(vip net.IP) error {
	if !s.Config.ManageLoopback {
		return nil
	}
	log.Infof("Adding %s to %s", vip, s.Config.Iface)
	// Add IPs to the interface
	iface, err := net.InterfaceByName(s.Config.Iface)
	if err != nil {
		return fmt.Errorf("failed to add IP to the %s interface: %w", s.Config.Iface, err)
	}

	return addIfaceIP(iface, &vip)
}

// deleteIPFromInterface deletes ip from interface
func (s *Server) deleteIPFromInterface(vip net.IP) error {
	if !s.Config.ManageLoopback {
		return nil
	}
	log.Infof("Deleting %s to %s", vip, s.Config.Iface)
	// Delete IPs to the interface
	iface, err := net.InterfaceByName(s.Config.Iface)
	if err != nil {
		return err
	}

	return deleteIfaceIP(iface, &vip)
}

// DeleteAllIPs deletes all IPs from interface specified in config
func (s *Server) DeleteAllIPs() {
	for _, vip := range s.Config.IPs {
		if err := s.deleteIPFromInterface(vip); err != nil {
			// Don't return error. Continue deleting
			log.Errorf("[server]: %v", err)
		}
	}
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
