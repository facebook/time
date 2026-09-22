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
	"cmp"
	"maps"
	"math"
	"net/netip"
	"slices"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/facebook/time/ptp/sptp/client/measurement"
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
	// the grandmaster path's window, but fed by every peer of every probe, so it
	// fills on the first round instead of over minutes
	pathDelayFilterLength = 59
	// pathDelayFloorPercentile takes the running estimate from the low end of the
	// window rather than its middle, so contamination cannot define the baseline
	pathDelayFloorPercentile = 0.1
	// pathDelayDiscardMultiplier bounds a peer against the rack's filtered
	// transit: far above it is its own timestamping, not the rack.
	pathDelayDiscardMultiplier = 2
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
	// delays is the path delay window across probes, and pathDelay the running
	// estimate taken from it, exactly as the grandmaster path keeps per server
	delays    *measurement.Window
	pathDelay time.Duration

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
	// consecutive probes that claimed the same thing; counts probes, not the sync
	// ticks that vastly outnumber them
	agreed uint16
}

// claim is what one probe concluded about our own clock.
type claim uint8

const (
	// over the threshold, but the peers do not agree closely enough to say so
	unsure claim = iota
	innocent
	biasedHigh
	biasedLow
)

func (c claim) accuses() bool { return c == biasedHigh || c == biasedLow }

func (r reading) claim(threshold time.Duration) claim {
	if r.peers < rackMinPeers {
		return unsure
	}
	// scaled by how many peers were asked, so a wide rack where many still agree
	// on a centre is usable and a bare quorum that looks tight is not
	doubt := rackSigmas * medianStdErrFactor * float64(r.spread) / math.Sqrt(float64(r.peers))
	switch {
	case r.median.Abs() <= threshold:
		// clearing our own clock needs the rack tighter than the threshold itself;
		// accusing it only needs the median told apart from zero
		if doubt > float64(threshold) {
			return unsure
		}
		return innocent
	case math.Abs(float64(r.median)) <= doubt:
		return unsure
	case r.median > 0:
		return biasedHigh
	}
	return biasedLow
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

// delay screens one peer against the running estimate and folds it in when
// usable, as measurements.delay and applyDelay do per grandmaster.
func (r *Rack) delay(newDelay time.Duration) bool {
	if r.delays == nil {
		r.delays = measurement.NewWindow(pathDelayFilterLength)
	}
	// transit is never zero or less, so it is not a measurement of anything
	if newDelay <= 0 {
		return false
	}
	// keep the first real sample whatever it is, or the estimate never starts
	if !math.IsNaN(r.delays.LastSample()) &&
		!measurement.PathDelayInRange(newDelay, pathDelayDiscardMultiplier*r.pathDelay, 0, r.pathDelay, r.delays.Full()) {
		log.Debugf("path delay %v outside (0, %dx %v] - filtering out", newDelay, pathDelayDiscardMultiplier, r.pathDelay)
		return false
	}
	r.delays.Add(float64(newDelay))
	// the low end, not the middle: a rack contaminates in whole probes, so a
	// median moves with the bad peers and raises the bound meant to reject them
	r.pathDelay = time.Duration(r.delays.Percentile(pathDelayFloorPercentile))
	return true
}

// screen folds a probe into the estimate and returns the peers that survived it.
func (r *Rack) screen(byDelay []Peer) []Peer {
	return slices.DeleteFunc(slices.Clone(byDelay), func(p Peer) bool { return !r.delay(p.PathDelay) })
}

func (r *Rack) observePeers(peers []Peer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// one reading per responder: a peer answering twice is still one opinion, and
	// counting both would let a single chatty peer reach quorum on its own
	newest := make(map[netip.Addr]Peer, len(peers))
	probed := time.Time{}
	for _, p := range peers {
		if p.At.After(probed) {
			probed = p.At
		}
		if prev, seen := newest[p.Addr]; !seen || p.At.After(prev.At) {
			newest[p.Addr] = p
		}
	}
	// shortest first: the window takes its first sample unconditionally, so the
	// lowest seeds it and the kept set does not depend on map order
	byDelay := slices.SortedFunc(maps.Values(newest), func(a, b Peer) int {
		return cmp.Compare(a.PathDelay, b.PathDelay)
	})
	kept := r.screen(byDelay)
	// a full probe screened below quorum is one rack-wide bad round if the last
	// reading still stands, and the transit having moved once that has aged out
	if len(kept) < rackMinPeers && len(byDelay) >= rackMinPeers {
		log.Warningf("rack screened %d of %d peers below quorum against floor %v",
			len(byDelay)-len(kept), len(byDelay), r.pathDelay)
		if r.delays == nil || !r.delays.Full() || probed.Sub(r.reading.at) <= rackMaxAge {
			return
		}
		log.Warningf("rack path delay floor %v stale for longer than %v - relearning", r.pathDelay, rackMaxAge)
		r.delays, r.pathDelay = nil, 0
		kept = r.screen(byDelay)
	}
	// a probe below quorum is not a reading, and must not clobber the last one
	// that was: bias would then reject the thin result and leave a still-bad path
	// uncorrected until the next full probe
	if len(kept) < rackMinPeers {
		return
	}
	sorted := make([]time.Duration, 0, len(kept))
	measured := time.Time{}
	for _, p := range kept {
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

	median := sorted[len(sorted)/2]
	spread := sorted[len(sorted)-1-lo] - sorted[lo]
	next := reading{
		median: median,
		spread: spread,
		peers:  len(sorted),
		at:     measured,
		// only correct() learns which GM we are following
		under: r.reading.under,
		// one claim made repeatedly, not several separate crossings
		agreed: 1,
	}
	// After, not just within the window: a replayed or equal-stamped batch is the
	// same probe again, and must not count as another agreement
	if next.claim(r.Config.Threshold) == r.reading.claim(r.Config.Threshold) &&
		measured.After(r.reading.at) && measured.Sub(r.reading.at) <= rackMaxAge {
		next.agreed = min(r.reading.agreed+1, r.Config.confirmations())
	}
	// replaced whole: a measurement and the path it describes cannot be updated
	// apart, and the new one has not been acted on
	r.reading = next
}

// verdict describes the path the rack currently sees. known is false when there
// is nothing recent to judge; quiet is true when the rack agrees we are fine,
// either because the bias is small or because the peers do not agree with each
// other. measured identifies the reading, so spending it later is atomic.
//
// Whether the reading was already spent is deliberately not part of this: a spent
// reading still describes a bad path, it just cannot justify another move.
func (r *Rack) verdict(now time.Time) (median time.Duration, quiet, confirmed, known bool, measured time.Time) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	// a stored reading always cleared quorum; observePeers is the only writer.
	// measured is returned even when unusable, so a stale arm can say how stale.
	if r.reading.at.IsZero() || now.Sub(r.reading.at) > rackMaxAge {
		return 0, false, false, false, r.reading.at
	}
	return r.reading.median, !r.reading.claim(r.Config.Threshold).accuses(), r.confirmedLocked(), true, r.reading.at
}

// judge reads one reading once: settled is the threshold, convict also needs it
// unspent and confirmed. Two calls would let a probe land between them and
// answer about different readings.
func (r *Rack) judge() (settled, convict bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.reading.peers < rackMinPeers {
		return false, false
	}
	settled = r.reading.median.Abs() <= r.Config.Threshold
	if !settled || r.reading.spent || r.reading.claim(r.Config.Threshold) != innocent || !r.confirmedLocked() {
		return settled, false
	}
	r.reading.spent = true
	return true, true
}

func (r *Rack) confirmedLocked() bool {
	return r.reading.agreed >= r.Config.confirmations()
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
	median, quiet, confirmed, known, measured := r.verdict(now)
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
	// mid-search a sign flip is the search sampling, not the bias going away
	if !confirmed && r.searched.count(best) == 0 {
		log.Debugf("rack bias %v on %s not yet confirmed, holding", median, best)
		return 0
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
