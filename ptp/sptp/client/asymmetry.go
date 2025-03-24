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
	"math"
	"net/netip"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	log "github.com/sirupsen/logrus"
)

// Returns number of port changes requested (which equals the number of GMS assumed to be asymmetric)
func correctAsymmetrySimple(clients map[netip.Addr]*Client, results map[netip.Addr]*RunResult, bestAddr netip.Addr, config AsymmetryConfig) int {
	if simpleSelectedGMAsymmetric(results, bestAddr, config) {
		correctSelectedGMAsymmetry(clients, bestAddr)
		return 1
	}

	return 0
}

// correctAsymmetry adjusts client AlternateResponsePortTLV Offset to correct path asymmetry based on the asymmetry configs provided.
// Returns number of port changes requested (which equals the number of GMS assumed to be asymmetric)
func correctAsymmetry(clients map[netip.Addr]*Client, results map[netip.Addr]*RunResult, bestAddr netip.Addr, config AsymmetryConfig) int {
	correctNonSelectedGMsAsymmetry(clients, results, bestAddr, config)

	if selectedGMAsymmetric(clients, config) {
		correctSelectedGMAsymmetry(clients, bestAddr)
	}

	return countAsymmetric(clients)
}

// correctNonSelectedGMsAsymmetry Increases AlternateResponsePortTLV Offset if clock offset is above a threshold after a configured period
func correctNonSelectedGMsAsymmetry(clients map[netip.Addr]*Client, results map[netip.Addr]*RunResult, bestAddr netip.Addr, config AsymmetryConfig) {
	for _, result := range results {
		client := clients[result.Server]
		client.asymmetric = false
		if result.Server == bestAddr {
			continue
		}
		if isAsymmetric(result, config.AsymmetryThreshold) {
			client.asymmetric = true
			if client.asymmetryCounter > config.MaxConsecutiveAsymmetry {
				alternateResponsePortTlv := getAlternateResponsePortTLV(client)
				if alternateResponsePortTlv != nil {
					alternateResponsePortTlv.Offset++
				}
				client.asymmetryCounter = 0
				log.Infof("GM %v Asymmetric - new port offset: %d", result.Server, client.delayRequest.TLVs[0].(*ptp.AlternateResponsePortTLV).Offset)
			} else {
				log.Debugf("GM %v asymmetric - grace %d/%d", result.Server, client.asymmetryCounter, config.MaxConsecutiveAsymmetry)
			}
			client.asymmetryCounter++
		} else {
			log.Debugf("GM %v not asymmetric, lowering grace", result.Server)
			client.asymmetryCounter = max(client.asymmetryCounter-1, 0)
		}
	}
}

// simpleSelectedGMAsymmetric checks if currently selected GM is asymmetric based on how many non-selected GMs are asymmetric
func simpleSelectedGMAsymmetric(results map[netip.Addr]*RunResult, bestAddr netip.Addr, config AsymmetryConfig) bool {
	var count int
	for addr, result := range results {
		if addr == bestAddr {
			continue
		}
		if isAsymmetric(result, config.AsymmetryThreshold) {
			count++
		}
	}
	return count == len(results)-1
}

// selectedGMAsymmetric verifies if we have attempted enough ports on any client to the point where we assume the currently selected GM is using a bad path
func selectedGMAsymmetric(clients map[netip.Addr]*Client, config AsymmetryConfig) bool {
	for _, c := range clients {
		if c.asymmetric {
			tlv := getAlternateResponsePortTLV(c)
			if tlv != nil && tlv.Offset > config.MaxPortChanges {
				return true
			}
		}
	}
	return false
}

func countAsymmetric(clients map[netip.Addr]*Client) int {
	count := 0
	for _, clients := range clients {
		if clients.asymmetric {
			count++
		}
	}
	return count
}

// correctSelectedGMAsymmetry requests a port change for the current selected GM and resets asymmetry status for all other clients.
// It performs no checks, and assumes the selected GM is asymmetric.
func correctSelectedGMAsymmetry(clients map[netip.Addr]*Client, bestAddr netip.Addr) {
	for addr, client := range clients {
		if addr == bestAddr {
			alternateResponsePortTlv := getAlternateResponsePortTLV(client)
			if alternateResponsePortTlv != nil {
				alternateResponsePortTlv.Offset++
			}
			client.asymmetric = true
			log.Infof("Selected GM %s asymmetric - new port offset: %d", bestAddr, alternateResponsePortTlv.Offset)
		} else {
			alternateResponsePortTlv := getAlternateResponsePortTLV(client)
			if alternateResponsePortTlv != nil {
				alternateResponsePortTlv.Offset = 0
			}
			client.asymmetric = false
			client.asymmetryCounter = 0
		}
	}
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

// isAsymmetric checks if a GM run result used an asymmetric path
func isAsymmetric(result *RunResult, asymmetryThreshold time.Duration) bool {
	// TODO: Threshold calculation could consider best GM as reference, in case all GMs fluctuate together (100ns fluctuation seen on tests)
	return result.Measurement.Announce.AnnounceBody.GrandmasterClockQuality.ClockClass == ptp.ClockClass6 && math.Abs(float64(result.Measurement.Offset)) > float64(asymmetryThreshold)
}
