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
	// the probe is driven from outside on a five-minute cadence, and its reading
	// is stamped when the first response landed -- before the probe has finished
	// collecting. A window equal to the period therefore expires before its
	// replacement arrives, and the corrector reports healthy for part of every
	// cycle. Three periods rides out two missed probes; path asymmetry persists
	// for hours, so a reading that old is still worth acting on.
	rackMaxAge = 15 * time.Minute
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

	mu      sync.RWMutex
	reading reading
	// searched counts moves in the current episode, per grandmaster. The port
	// offset is monotonic for the life of the daemon, so budgeting against it
	// would let a host search once and sit inert for days, deaf to any later
	// fault. Keyed by GM because each is reached over its own path: failing over
	// to one nobody has searched must not inherit a spent budget.
	searched searchCounter
}

// reading is one rack measurement. What the peers said, when, which path it
// describes, and whether it has already been acted on -- all replaced together
// by observePeers, and meaningless apart, because a measurement of this host's
// clock only means something in the context of the path it was taken over.
type reading struct {
	median time.Duration
	spread time.Duration
	// peers is how many distinct responders it is drawn from; the median is only
	// as well determined as its sample is large
	peers int
	at    time.Time
	// under is the grandmaster whose path this measured. Failing over re-steers
	// the clock, so the reading then describes a route we are no longer on: not
	// enough to move the new GM's port, and not enough to call its search done.
	under netip.Addr
	// spent marks it as having already justified a move. Sync ticks outnumber
	// probes by orders of magnitude, so without this one measurement would move
	// the port on every tick until the next probe replaced it.
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

	// spread between symmetric quantiles, so a wild peer counts the same whether
	// it sits high or low. Integer division does the right thing at a thin quorum
	// on its own: below eight peers it trims less than a quartile, and at the
	// three-peer floor it trims nothing and reports the full span, which errs
	// towards holding rather than moving on three noisy peers.
	lo := len(sorted) / 4

	r.mu.Lock()
	defer r.mu.Unlock()
	// replaced whole: a measurement and the path it describes cannot be updated
	// apart, and the new one has not been acted on. under is carried over because
	// only correct() learns which GM we are following.
	r.reading = reading{
		median: sorted[len(sorted)/2],
		spread: sorted[len(sorted)-1-lo] - sorted[lo],
		peers:  len(sorted),
		at:     measured,
		under:  r.reading.under,
	}
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
	if r.reading.at.IsZero() || now.Sub(r.reading.at) > rackMaxAge {
		return 0, false, false, r.reading.at
	}
	quiet = r.reading.median.Abs() <= r.Config.Threshold || !r.decidedLocked()
	return r.reading.median, quiet, true, r.reading.at
}

// stderrLocked is the standard error of the stored median, with the trimmed span
// standing in for the deviation. Callers must hold at least the read lock.
func (r *Rack) stderrLocked() float64 {
	return medianStdErrFactor * float64(r.reading.spread) / math.Sqrt(float64(r.reading.peers))
}

// judge reads one reading once: settled is the threshold, convict also needs it
// unspent and the rack's own uncertainty under that threshold. Two calls would
// let a probe land between them and answer about different readings.
func (r *Rack) judge() (settled, convict bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.reading.peers < rackMinPeers {
		return false, false
	}
	settled = r.reading.median.Abs() <= r.Config.Threshold
	if !settled || r.reading.spent || rackSigmas*r.stderrLocked() > float64(r.Config.Threshold) {
		return settled, false
	}
	r.reading.spent = true
	return true, true
}

// decided reports whether the peers place the median far enough from zero to act
// on. Callers must hold at least the read lock; verdict is the only one today. Comparing the spread directly to the median ignores how many peers were
// asked, which both blocks a wide-spread rack where many peers still agree on a
// centre -- switches that do not correct residence time spread every offset by
// microseconds -- and acts on a bare quorum that happens to look tight.
func (r *Rack) decidedLocked() bool {
	if r.reading.peers < rackMinPeers {
		return false
	}
	// below eight peers the trimmed span is wider than a true IQR, so the estimate
	// errs high and the corrector holds rather than moves
	return math.Abs(float64(r.reading.median)) > rackSigmas*r.stderrLocked()
}

// takeBias returns the reading and marks it spent in one step, so a probe landing
// mid-decision cannot have its fresh reading consumed by the move this one made.
func (r *Rack) takeBias(gm netip.Addr, measured time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.reading.spent || !r.reading.at.Equal(measured) {
		return false
	}
	r.reading.spent = true
	r.searched.charge(gm)
	return true
}

// exhausted reports whether this episode has used its whole search budget. A
// MaxPortChanges of N permits exactly N moves, so zero is a usable detect-only
// setting rather than one that still moves once.
func (r *Rack) exhausted(gm netip.Addr) bool {
	return r.searched.count(gm) >= r.Config.MaxPortChanges
}

// correctOthers searches the paths we are not following. Only reached from the
// quiet branch, so the rack has already said our own clock is inside the
// threshold and a bad local clock cannot convict all four.
func (r *Rack) correctOthers(gms map[netip.Addr]*GM, best netip.Addr) int {
	moved := 0
	for addr, gm := range gms {
		if addr == best || !gm.Answered || !gm.Judgeable {
			continue
		}
		if gm.Offset.Abs() <= r.Config.Threshold {
			// this path is good now, so its search is over and the next fault on it
			// starts with a full budget rather than inheriting an exhausted one
			gm.Asymmetric = false
			r.settle(addr)
			continue
		}
		gm.Asymmetric = true
		if r.exhausted(addr) {
			continue
		}
		r.searched.charge(addr)
		gm.MovePort()
		moved++
		log.Infof("GM %s off by %v against a quiet rack - new port offset: %d", addr, gm.Offset, gm.PortOffset)
	}
	return moved
}

// PortMoves implements Corrector.
func (r *Rack) PortMoves(gm netip.Addr) uint16 { return r.searched.count(gm) }

// settle ends the current episode for one grandmaster. A small median says the
// path we are following is fine; it is no evidence about the others, so clearing
// them would hand a known-bad path a fresh budget on every failover back to it.
func (r *Rack) settle(gm netip.Addr) { r.searched.clear(gm) }

// rebind drops the stored reading when the selected GM changes. The reading is
// evidence about one path: it can neither justify moving a different GM's port
// nor declare that GM's search finished.
// rebind reports whether the selected grandmaster changed.
func (r *Rack) rebind(best netip.Addr) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.reading.under == best {
		return false
	}
	if r.reading.under.IsValid() {
		// discard rather than re-label: the measurement belongs to the old route
		r.reading = reading{}
	}
	r.reading.under = best
	return true
}

func (r *Rack) correct(gms map[netip.Addr]*GM, best netip.Addr) int {
	// before anything reads the reading, make sure it belongs to this GM
	movedGM := r.rebind(best)
	r.searched.keepOnly(gms)
	// Asymmetric describes the path, not whether we acted on it, so a spent or
	// exhausted reading still reports the fault. Only a fresh reading showing a
	// quiet rack clears it.
	now := time.Now()
	median, quiet, known, measured := r.verdict(now)
	// the others carry across ticks and correctOthers owns them, so clearing them
	// every tick would flap them false whenever no fresh reading arrived. Failing
	// over is the exception: a verdict reached while we followed a GM says nothing
	// once we no longer do.
	for addr, gm := range gms {
		switch {
		case addr == best:
			gm.Asymmetric = known && !quiet
		case movedGM:
			gm.Asymmetric = false
		}
	}
	if !known {
		// an arm with no reading looks exactly like a quiet one from the outside;
		// Debug because sync ticks outnumber probes by orders of magnitude
		log.Debugf("no usable rack reading, last probe %v", measured)
		return 0
	}
	if quiet {
		// settled, not merely quiet: quiet is also true when the peers cannot agree,
		// and a clock we cannot vouch for turns every GM's offset back into a
		// statement about us. Only reached when the selected path needs nothing, so
		// our own clock comes first and the two never contend for one reading.
		others := 0
		settled, convict := r.judge()
		if settled {
			if convict {
				others = r.correctOthers(gms, best)
			}
			r.settle(best)
		}
		return others
	}
	selected := gms[best]
	if selected == nil {
		log.Errorf("selected GM %v is not in the GM list", best)
		return 0
	}
	// the offset is hashed to a send worker rather than indexing one, so a search
	// samples paths with replacement and never proves it has seen them all. The
	// budget is a bound on effort, not evidence of exhaustive coverage.
	if r.exhausted(best) {
		log.Debugf("selected GM %s exhausted the search, rack bias %v persists", best, median)
		return 0
	}
	if !r.takeBias(best, measured) {
		return 0
	}
	// the offset is not reset when an episode settles: it IS the path we landed
	// on, so zeroing it would put the host back on the one the search just moved
	// off. It only ever climbs, which is harmless -- ptp4u hashes it to pick a
	// worker rather than indexing, so every value is an equally valid path and a
	// wrap lands on one too.
	selected.MovePort()
	selected.Asymmetric = true
	log.Infof("rack says we are %v off - selected GM %s new port offset: %d", median, best, selected.PortOffset)
	return 1
}
