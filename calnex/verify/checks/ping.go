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

package checks

import (
	"context"
	"fmt"
	"net"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv6"
)

const ipv6ICMP = 58

// Ping check
type Ping struct {
	Remediation Remediation
}

// Name returns the name of the check
func (p *Ping) Name() string {
	return "Ping"
}

// Run executes the check
func (p *Ping) Run(target string, _ bool) error {
	ip, err := net.ResolveIPAddr("ip", target)
	if err != nil {
		return err
	}
	dst := &net.UDPAddr{IP: ip.IP, Zone: ip.Zone}

	conn, err := icmp.ListenPacket("udp6", "::")
	if err != nil {
		return err
	}

	response := make([]byte, 32)
	request := make([]byte, 32)

	for i := 0; i < 3; i++ {
		// Write
		msg := &icmp.Message{
			Type: ipv6.ICMPTypeEchoRequest,
			Code: 0,
			Body: &icmp.Echo{
				ID:   i,
				Seq:  i,
				Data: request,
			},
		}
		b, err := msg.Marshal(nil)
		if err != nil {
			return err
		}

		if _, err = conn.WriteTo(b, dst); err != nil {
			return err
		}

		// Read
		if err = conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			return err
		}
		_, _, err = conn.ReadFrom(response)
		if err != nil {
			return err
		}

		r, err := icmp.ParseMessage(ipv6ICMP, response)
		if err != nil {
			return err
		}

		if r.Type != ipv6.ICMPTypeEchoReply {
			// Not an echo response!
			return fmt.Errorf("malformed response")
		}
	}

	return nil
}

// Remediate the check
func (p *Ping) Remediate(ctx context.Context) (string, error) {
	return p.Remediation.Remediate(ctx)
}

// PingRemediation is an open source action for Ping check
type PingRemediation struct{}

// Remediate remediates the Ping check failure
func (a PingRemediation) Remediate(_ context.Context) (string, error) {
	return "Restart the device", nil
}
