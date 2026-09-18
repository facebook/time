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

package client

import (
	"net/netip"
	"time"

	"github.com/facebook/time/ptp/pdelay"
	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/sptp/asymmetry"
)

// newCorrector returns nil when asymmetry correction is disabled.
func newCorrector(config AsymmetryConfig) asymmetry.Corrector {
	if !config.AsymmetryCorrectionEnabled {
		return nil
	}
	c := asymmetry.Config{
		Threshold:      config.AsymmetryThreshold,
		MaxConsecutive: config.MaxConsecutiveAsymmetry,
		MaxPortChanges: config.MaxPortChanges,
	}
	if config.Rack {
		return &asymmetry.Rack{Config: c}
	}
	return &asymmetry.Simple{Config: c}
}

// observePeers hands a completed multicast probe to the corrector.
func (p *SPTP) observePeers(results pdelay.Results) {
	peers := make([]asymmetry.Peer, 0, len(results))
	for _, r := range results {
		if r == nil || r.Error != nil || !r.Valid() {
			continue
		}
		peers = append(peers, asymmetry.Peer{Addr: r.Responder, Offset: r.Offset(), At: r.Timestamp})
	}
	p.corrector.Observe(asymmetry.Observation{Peers: peers, At: time.Now()})
}

// correctAsymmetry hands the tick's measurements to the corrector and writes any
// decision back onto the clients.
func (p *SPTP) correctAsymmetry(results map[netip.Addr]*RunResult, bestAddr netip.Addr) int {
	// every configured GM, not just the ones that answered: a corrector can only
	// clear stale state on a GM it can see. A result naming an address we never
	// configured, which a malformed packet can produce, has no GM to carry it.
	gms := make(map[netip.Addr]*asymmetry.GM, len(p.clients))
	for addr, client := range p.clients {
		gms[addr] = newGM(client, results[addr])
	}
	n := p.corrector.Observe(asymmetry.Observation{GMs: gms, Best: bestAddr, At: time.Now()})
	for addr, gm := range gms {
		client := p.clients[addr]
		client.asymmetric = gm.Asymmetric
		client.asymmetryCounter = gm.Streak
		applyPortActions(client, gm)
	}
	return n
}

// applyPortActions performs what the correction decided. Keeping the mutation
// here means a corrector never reaches into a live delay request.
func applyPortActions(client *Client, gm *asymmetry.GM) {
	tlv := getAlternateResponsePortTLV(client)
	if tlv == nil {
		return
	}
	if gm.PortMoved {
		tlv.Offset++
	}
}

func newGM(client *Client, result *RunResult) *asymmetry.GM {
	gm := &asymmetry.GM{
		Asymmetric: client.asymmetric,
		Streak:     client.asymmetryCounter,
	}
	if tlv := getAlternateResponsePortTLV(client); tlv != nil {
		gm.PortOffset = tlv.Offset
	}
	if result == nil {
		return gm
	}
	// a backoff or errored run still answers for this tick, it just carries
	// nothing to judge
	gm.Answered = true
	if result.Measurement == nil {
		return gm
	}
	// a rejected delay or a clock class we do not follow cannot be judged either
	gm.Judgeable = !result.Measurement.BadDelay &&
		result.Measurement.Announce.GrandmasterClockQuality.ClockClass == ptp.ClockClass6
	gm.Offset = result.Measurement.Offset
	return gm
}

func getAlternateResponsePortTLV(client *Client) *ptp.AlternateResponsePortTLV {
	if client == nil || client.delayRequest == nil {
		return nil
	}
	for _, tlv := range client.delayRequest.TLVs {
		if alternateResponsePortTlv, ok := tlv.(*ptp.AlternateResponsePortTLV); ok {
			return alternateResponsePortTlv
		}
	}
	return nil
}
