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

package checker

import (
	"fmt"
	"math"
	"net"
	"strings"
)

// NewNTPPeerStats constructs NTPStats from NTPCheckResult
func NewNTPPeerStats(r *NTPCheckResult, noDNS bool) (map[string]any, error) {
	if r.SysVars == nil {
		return nil, fmt.Errorf("no system variables to output stats")
	}

	result := make(map[string]any, len(r.Peers))

	// Then add stats for all the peers
	for _, peer := range r.Peers {
		// skip invalid IPs
		ip := net.ParseIP(peer.SRCAdr)
		if ip == nil || ip.IsUnspecified() {
			continue
		}
		hostnames := peerName(peer, noDNS)
		// Replace "." and ":" for "_"
		hostname := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSuffix(hostnames[0], "."), ".", "_"), ":", "_")

		result[fmt.Sprintf("ntp.peers.%s.delay", hostname)] = peer.Delay
		result[fmt.Sprintf("ntp.peers.%s.poll", hostname)] = 1 << uint(math.Min(float64(peer.PPoll), float64(peer.HPoll)))
		result[fmt.Sprintf("ntp.peers.%s.jitter", hostname)] = peer.Jitter
		result[fmt.Sprintf("ntp.peers.%s.offset", hostname)] = peer.Offset
		result[fmt.Sprintf("ntp.peers.%s.stratum", hostname)] = peer.Stratum
	}

	return result, nil
}

func peerName(p *Peer, noDNS bool) []string {
	if noDNS {
		if len(p.Hostname) != 0 {
			return []string{p.Hostname}
		}
		return []string{p.SRCAdr}
	}
	hostnames, err := net.LookupAddr(p.SRCAdr)
	if err != nil || len(hostnames) == 0 {
		hostnames = []string{p.SRCAdr}
	}
	return hostnames
}
