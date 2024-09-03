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
	"time"

	log "github.com/sirupsen/logrus"
)

// NTPStats are metrics for upstream reporting
type NTPStats struct {
	PeerDelay             float64 `json:"ntp.peer.delay"`                          // sys.peer delay in ms
	PeerPoll              int     `json:"ntp.peer.poll"`                           // sys.peer poll in seconds
	PeerJitter            float64 `json:"ntp.peer.jitter"`                         // sys.peer jitter in ms
	PeerOffset            float64 `json:"ntp.peer.offset"`                         // sys.peer offset in ms
	PeerStratum           int     `json:"ntp.peer.stratum"`                        // sys.peer stratum
	Frequency             float64 `json:"ntp.sys.frequency"`                       // clock frequency in PPM
	Offset                float64 `json:"ntp.sys.offset"`                          // tracking clock offset in MS
	RootDelay             float64 `json:"ntp.sys.root_delay"`                      // tracking root delay in MS
	StatError             bool    `json:"ntp.stat.error"`                          // error reported in Leap Status
	Correction            float64 `json:"ntp.correction"`                          // current correction
	PeerCount             int     `json:"ntp.peer.count"`                          // number of upstream peers
	OffsetComparedToPeers float64 `json:"ntp.sys.offset_selected_vs_avg_peers_ms"` // sys peer offset vs avg peer offset in ms
}

type averages struct {
	delay   float64
	jitter  float64
	offset  float64
	poll    uint
	stratum int
}

func peersAverages(peers []*Peer) (*averages, error) {
	// calculate averages
	total := len(peers)
	if total == 0 {
		return nil, fmt.Errorf("no peers detected to output stats")
	}
	totalDelay := 0.0
	totalJitter := 0.0
	totalOffset := 0.0
	bestPPoll := 0
	bestHPoll := 0
	bestStratum := 0
	for _, p := range peers {
		totalDelay += p.Delay
		totalJitter += p.Jitter
		totalOffset += p.Offset
		if bestPPoll == 0 || p.PPoll < bestPPoll {
			bestPPoll = p.PPoll
		}
		if bestHPoll == 0 || p.HPoll < bestHPoll {
			bestHPoll = p.HPoll
		}
		if bestStratum == 0 || p.Stratum < bestStratum {
			bestStratum = p.Stratum
		}
	}
	return &averages{
		delay:   totalDelay / float64(total),
		poll:    uint(math.Min(float64(bestPPoll), float64(bestHPoll))),
		jitter:  totalJitter / float64(total),
		offset:  totalOffset / float64(total),
		stratum: bestStratum,
	}, nil
}

// NewNTPStats constructs NTPStats from NTPCheckResult
func NewNTPStats(r *NTPCheckResult) (*NTPStats, error) {
	if r.SysVars == nil {
		return nil, fmt.Errorf("no system variables to output stats")
	}
	var delay, jitter, offset float64
	var poll uint
	var stratum int
	var offsetComparedToPeers float64
	syspeer, err := r.FindSysPeer()
	if err != nil {
		log.Warningf("Can't get system peer: %v", err)
		goodPeers, err := r.FindGoodPeers()
		if err != nil {
			return nil, fmt.Errorf("nothing to calculate stats from: %w", err)
		}
		peerAvgs, err := peersAverages(goodPeers)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate stats from peers: %w", err)
		}

		delay = peerAvgs.delay
		jitter = peerAvgs.jitter
		poll = peerAvgs.poll
		stratum = peerAvgs.stratum
		offset = peerAvgs.offset
	} else {
		delay = syspeer.Delay
		jitter = syspeer.Jitter
		poll = uint(math.Min(float64(syspeer.PPoll), float64(syspeer.HPoll)))
		stratum = syspeer.Stratum
		offset = syspeer.Offset
	}

	// get averages from non-sys peers
	okPeers, err := r.FindAcceptableNonSysPeers()
	if err != nil {
		log.Debugf("Can't get any peers for avg calculations: %v", err)
	} else {
		peerAvgs, err := peersAverages(okPeers)
		if err == nil {
			log.Debugf("Sys Offset: %v, Avg Peer Offset: %v", time.Duration(offset*float64(time.Millisecond)), time.Duration(peerAvgs.offset*float64(time.Millisecond)))
			offsetComparedToPeers = math.Abs(math.Abs(offset) - math.Abs(peerAvgs.offset))
		}
	}
	output := NTPStats{
		PeerDelay:             delay,
		PeerPoll:              1 << poll, // hpoll and ppoll are stored in seconds as a power of two
		PeerJitter:            jitter,
		PeerOffset:            offset,
		Offset:                r.SysVars.Offset,
		RootDelay:             r.SysVars.RootDelay,
		PeerStratum:           stratum,
		Frequency:             r.SysVars.Frequency,
		Correction:            r.Correction,
		StatError:             r.LI == 3, // that's how ntpstat defines unsynchronized
		PeerCount:             len(r.Peers),
		OffsetComparedToPeers: offsetComparedToPeers,
	}
	return &output, nil
}
