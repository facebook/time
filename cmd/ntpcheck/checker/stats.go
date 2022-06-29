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
	"math"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// NTPStats is what we want to report as stats for FBAgent to put into ODS
type NTPStats struct {
	PeerDelay   float64 `json:"ntp.peer.delay"`    // sys.peer delay in ms
	PeerPoll    int     `json:"ntp.peer.poll"`     // sys.peer poll in seconds
	PeerJitter  float64 `json:"ntp.peer.jitter"`   // sys.peer jitter in ms
	PeerOffset  float64 `json:"ntp.peer.offset"`   // sys.peer offset in ms
	PeerStratum int     `json:"ntp.peer.stratum"`  // sys.peer stratum
	Frequency   float64 `json:"ntp.sys.frequency"` // clock frequency in PPM
	StatError   bool    `json:"ntp.stat.error"`    // error reported in Leap Status
	Correction  float64 `json:"ntp.correction"`    // current correction
}

// NewNTPStats constructs NTPStats from NTPCheckResult
func NewNTPStats(r *NTPCheckResult) (*NTPStats, error) {
	if r.SysVars == nil {
		return nil, errors.New("no system variables to output stats")
	}
	syspeer, err := r.FindSysPeer()
	var delay, jitter, offset float64
	var poll uint
	var stratum int
	if err != nil {
		log.Warningf("Can't get system peer: %v", err)
		// calculate averages like ntp_stats_ods.py did
		total := len(r.Peers)
		if total == 0 {
			return nil, errors.New("no peers detected to output stats")
		}
		goodPeers, err := r.FindGoodPeers()
		if err != nil {
			return nil, errors.Wrap(err, "nothing to calculate stats from")
		}
		totalDelay := 0.0
		totalJitter := 0.0
		totalOffset := 0.0
		bestPPoll := 0
		bestHPoll := 0
		bestStratum := 0
		for _, p := range goodPeers {
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
		delay = totalDelay / float64(total)
		jitter = totalJitter / float64(total)
		offset = totalOffset / float64(total)
		stratum = bestStratum
		poll = uint(math.Min(float64(bestPPoll), float64(bestHPoll)))
	} else {
		delay = syspeer.Delay
		jitter = syspeer.Jitter
		poll = uint(math.Min(float64(syspeer.PPoll), float64(syspeer.HPoll)))
		stratum = syspeer.Stratum
		offset = syspeer.Offset
	}
	output := NTPStats{
		PeerDelay:   delay,
		PeerPoll:    1 << poll, // hpoll and ppoll are stored in seconds as a power of two
		PeerJitter:  jitter,
		PeerOffset:  offset,
		PeerStratum: stratum,
		Frequency:   r.SysVars.Frequency,
		Correction:  r.Correction,
		// that's how ntpstat defines unsynchronized
		StatError: r.LI == 3,
	}
	return &output, nil
}
