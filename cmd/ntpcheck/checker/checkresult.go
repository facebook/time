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
	"errors"
	"slices"

	"github.com/facebook/time/ntp/control"
)

// NTPCheckResult represents result of NTPCheck run populated with information about the server and its peers.
type NTPCheckResult struct {
	// parsed from SystemStatusWord
	LI          uint8
	LIDesc      string
	ClockSource string
	Correction  float64
	Event       string
	EventCount  uint8
	// data parsed from System Variables
	SysVars *SystemVariables
	// map of peers with data from PeerStatusWord and Peer Variables
	Peers map[uint16]*Peer
}

// FindSysPeer returns sys.peer (main source of NTP information for server)
func (r *NTPCheckResult) FindSysPeer() (*Peer, error) {
	if len(r.Peers) == 0 {
		return nil, errors.New("no peers present")
	}
	for _, peer := range r.Peers {
		if peer.Selection == control.SelSYSPeer {
			return peer, nil
		}
	}
	return nil, errors.New("no sys.peer present")
}

// FindGoodPeers returns list of peers suitable for synchronization
func (r *NTPCheckResult) FindGoodPeers() ([]*Peer, error) {
	results := []*Peer{}
	// see http://doc.ntp.org/current-stable/decode.html#peer for reference
	goodStatusesMap := map[uint8]bool{
		control.SelCandidate: true,
		control.SelBackup:    true,
		control.SelSYSPeer:   true,
		control.SelPPSPeer:   true,
	}
	if len(r.Peers) == 0 {
		return results, errors.New("no peers present")
	}
	for _, peer := range r.Peers {
		if goodStatusesMap[peer.Selection] {
			results = append(results, peer)
		}
	}
	if len(results) == 0 {
		return results, errors.New("no good peers present")
	}
	return results, nil
}

// FindNoSelectPeers returns list of peers in noselect state
func (r *NTPCheckResult) FindNoSelectPeers() ([]*Peer, error) {
	results := []*Peer{}
	if len(r.Peers) == 0 {
		return results, errors.New("no peers present")
	}
	for _, peer := range r.Peers {
		if peer.NoSelect {
			results = append(results, peer)
		}
	}
	return results, nil
}

// FindAcceptableNonSysPeers returns list of peers suitable for checking time with excluding sys.peer
func (r *NTPCheckResult) FindAcceptableNonSysPeers() ([]*Peer, error) {
	results, err := r.FindGoodPeers()
	if err != nil || len(results) == 1 {
		results, err = r.FindNoSelectPeers()
	}
	return slices.DeleteFunc(results, func(p *Peer) bool {
		return p.Selection == control.SelSYSPeer
	}), err
}

// NewNTPCheckResult returns new instance of NewNTPCheckResult
func NewNTPCheckResult() *NTPCheckResult {
	return &NTPCheckResult{
		Peers: map[uint16]*Peer{},
	}
}
