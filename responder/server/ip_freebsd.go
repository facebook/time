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
	"os/exec"

	errors "github.com/pkg/errors"
)

func addIfaceIP(iface *net.Interface, addr *net.IP) error {
	// Check if IP is assigned:
	assigned, err := checkIP(iface, addr)
	if err != nil {
		return err
	}
	if assigned {
		return nil
	}

	var mask int
	var proto string
	if v4 := addr.To4(); v4 == nil {
		proto = "inet6"
		mask = ipv6Mask
	} else {
		proto = "inet"
		mask = ipv4Mask
	}

	cmd := exec.Command("ifconfig", iface.Name, proto, "alias", fmt.Sprintf("%s/%d", addr.String(), mask))
	fmt.Println(cmd.Args)
	if err := cmd.Run(); err != nil {
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

	var proto string
	if v4 := addr.To4(); v4 == nil {
		proto = "inet6"
	} else {
		proto = "inet"
	}

	cmd := exec.Command("ifconfig", iface.Name, proto, "-alias", addr.String())
	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "can't remove address")
	}

	return nil
}