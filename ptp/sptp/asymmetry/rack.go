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

package asymmetry

import (
	"math"
	"net/netip"
	"slices"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	// the probe is driven from outside, so a reading that stopped arriving must
	// not keep steering corrections
	rackMaxAge = 5 * time.Minute
	// one or two responders is not a rack
	rackMinPeers = 3
	// 1.2533/1.349: the median's standard error in deviations, over the IQR's
	// width in the same, so the deviation itself never has to be estimated
	medianStdErrFactor = 0.9291
	// how far from zero the median must sit before a port moves
	rackSigmas = 3.0
)

// Rack moves the selected GM's port when in-rack peers agree this host's clock
// is off.
//
// The other correctors read the offset to a grandmaster, which a locked servo
// drives to ~0 whatever the path asymmetry is -- the fault is invisible there.
// In-rack cabling is identical, so a peer measurement carries no asymmetry term
// and disagreement with the rack is the error itself.
//
// The peer delay probe is driven from outside; Rack neither schedules nor logs it.
type Rack struct {
	Config Config

	mu       sync.RWMutex
	median   time.Duration
	spread   time.Duration
	measured time.Time
	// peers is how many distinct responders the reading is drawn from; the median
	// is only as well determined as its sample is large
	peers int
	// spent marks a reading already used to move a port. Sync ticks are far more
	// frequent than probes, so without this one observation would move the port
	// again on every tick until the next probe replaced it.
	spent bool
}

// Name implements Corrector.
func (r *Rack) Name() string { return "rack" }

// Observe implements Corrector. A peer round updates what the rack reports, a GM
// round is when Rack can act on it; the two arrive on different cadences.
func (r *Rack) Observe(obs Observation) int {
	r.observePeers(obs.Peers)
	if len(obs.GMs) == 0 {
		return 0
	}
	return r.correct(obs.GMs, obs.Best)
}

func (r *Rack) observePeers(peers []Peer) {
	// one reading per responder: a peer answering twice is still one opinion, and
	// counting both would let a single chatty peer reach quorum on its own
	newest := make(map[netip.Addr]Peer, len(peers))
	for _, p := range peers {
		if prev, seen := newest[p.Addr]; !seen || p.At.After(prev.At) {
			newest[p.Addr] = p
		}
	}
	// a probe below quorum is not a reading, and must not clobber the last one
	// that was: bias would then reject the thin result and leave a still-bad path
	// uncorrected until the next full probe
	if len(newest) < rackMinPeers {
		return
	}
	sorted := make([]time.Duration, 0, len(newest))
	measured := time.Time{}
	for _, p := range newest {
		sorted = append(sorted, p.Offset)
		// the round is only as fresh as its newest exchange
		if p.At.After(measured) {
			measured = p.At
		}
	}
	slices.Sort(sorted)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.median = sorted[len(sorted)/2]
	// spread between symmetric quantiles, so a wild peer counts the same whether
	// it sits high or low. Trimming is dropped when it would leave nothing to
	// compare: at a thin quorum the full span is the honest answer, and it errs
	// towards holding rather than moving a port on three noisy peers.
	lo := len(sorted) / 4
	if len(sorted)-1-lo <= lo {
		lo = 0
	}
	r.spread = sorted[len(sorted)-1-lo] - sorted[lo]
	r.peers = len(sorted)
	r.measured = measured
	r.spent = false
}

// verdict describes the path the rack currently sees. known is false when there
// is nothing recent to judge; quiet is true when the rack agrees we are fine,
// either because the bias is small or because the peers do not agree with each
// other. measured identifies the reading, so spending it later is atomic.
//
// Whether the reading was already spent is deliberately not part of this: a spent
// reading still describes a bad path, it just cannot justify another move.
func (r *Rack) verdict(now time.Time) (median time.Duration, quiet, known bool, measured time.Time) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	// a stored reading always cleared quorum; observePeers is the only writer.
	// measured is returned even when unusable, so a stale arm can say how stale.
	if r.measured.IsZero() || now.Sub(r.measured) > rackMaxAge {
		return 0, false, false, r.measured
	}
	quiet = r.median.Abs() <= r.Config.Threshold || !r.decided()
	return r.median, quiet, true, r.measured
}

// decided reports whether the peers place the median far enough from zero to act
// on. Comparing the spread directly to the median ignores how many peers were
// asked, which both blocks a wide-spread rack where many peers still agree on a
// centre -- switches that do not correct residence time spread every offset by
// microseconds -- and acts on a bare quorum that happens to look tight.
func (r *Rack) decided() bool {
	if r.peers < rackMinPeers {
		return false
	}
	// standard error of a median, with the IQR standing in for the deviation
	stderr := medianStdErrFactor * float64(r.spread) / math.Sqrt(float64(r.peers))
	return math.Abs(float64(r.median)) > rackSigmas*stderr
}

// takeBias returns the reading and marks it spent in one step, so a probe landing
// mid-decision cannot have its fresh reading consumed by the move this one made.
func (r *Rack) takeBias(measured time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.spent || !r.measured.Equal(measured) {
		return false
	}
	r.spent = true
	return true
}

func (r *Rack) correct(gms map[netip.Addr]*GM, best netip.Addr) int {
	// Asymmetric describes the path, not whether we acted on it, so a spent or
	// exhausted reading still reports the fault. Only a fresh reading showing a
	// quiet rack clears it.
	now := time.Now()
	median, quiet, known, measured := r.verdict(now)
	if selected := gms[best]; selected != nil {
		// evidence of a bad path, not a record of having acted: a spent or
		// budget-exhausted reading still reports it, and no evidence is not evidence
		selected.Asymmetric = known && !quiet
	}
	if !known {
		// an arm with no reading looks exactly like a quiet one from the outside;
		// Debug because sync ticks outnumber probes by orders of magnitude
		log.Debugf("no usable rack reading, last probe %v", measured)
		return 0
	}
	if quiet {
		return 0
	}
	selected := gms[best]
	if selected == nil {
		log.Errorf("selected GM %v is not in the GM list", best)
		return 0
	}
	// a port offset only picks a ptp4u send worker, so past MaxPortChanges every
	// distinct return path has been tried and moving again just reshuffles among
	// paths already known to be bad
	if selected.PortOffset > r.Config.MaxPortChanges {
		log.Debugf("selected GM %s exhausted %d port changes, rack bias %v persists",
			best, selected.PortOffset, median)
		return 0
	}
	if !r.takeBias(measured) {
		return 0
	}
	selected.MovePort()
	selected.Asymmetric = true
	log.Infof("rack says we are %v off - selected GM %s new port offset: %d", median, best, selected.PortOffset)
	return 1
}
