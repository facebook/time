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
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var (
	addrA = netip.MustParseAddr("192.168.0.1")
	addrB = netip.MustParseAddr("192.168.0.2")
	addrC = netip.MustParseAddr("192.168.0.3")
)

// gm is a GM that answered with a judgeable measurement, or stayed silent
func gm(offset time.Duration, answered bool) *GM {
	return &GM{Offset: offset, Answered: answered, Judgeable: answered}
}

// unjudgeable answered, but with a rejected delay or a clock class we do not follow
func unjudgeable(offset time.Duration) *GM {
	return &GM{Offset: offset, Answered: true}
}

func TestGMSuspicious(t *testing.T) {
	threshold := time.Microsecond
	require.True(t, gm(2*time.Microsecond, true).suspicious(threshold))
	require.True(t, gm(-2*time.Microsecond, true).suspicious(threshold), "sign does not matter")
	require.False(t, gm(500*time.Nanosecond, true).suspicious(threshold))
	require.False(t, gm(threshold, true).suspicious(threshold), "boundary is not over")
	require.False(t, gm(9*time.Microsecond, false).suspicious(threshold), "silence is never evidence")
	require.False(t, unjudgeable(9*time.Microsecond).suspicious(threshold), "an unjudgeable answer is not suspicion")
}

func TestSimpleNeedsUnanimity(t *testing.T) {
	cfg := Config{Threshold: time.Microsecond}
	s := &Simple{Config: cfg}

	// B and C disagree about whether anything is wrong, so the selected GM is fine
	gms := map[netip.Addr]*GM{
		addrA: gm(0, true),
		addrB: gm(5*time.Microsecond, true),
		addrC: gm(10*time.Nanosecond, true),
	}
	require.Zero(t, s.Observe(Observation{GMs: gms, Best: addrA}))
	require.Zero(t, gms[addrA].PortOffset)
}

func TestSimpleMovesPortAfterStreak(t *testing.T) {
	s := &Simple{Config: Config{Threshold: time.Microsecond, MaxConsecutive: 2}}
	build := func() map[netip.Addr]*GM {
		return map[netip.Addr]*GM{
			addrA: gm(0, true),
			addrB: gm(5*time.Microsecond, true),
			addrC: gm(5*time.Microsecond, true),
		}
	}
	gms := build()
	selected := gms[addrA]
	// the streak lives on the selected GM, so carry it across ticks
	for range 3 {
		next := build()
		next[addrA] = selected
		gms = next
		require.Zero(t, s.Observe(Observation{GMs: gms, Best: addrA}), "still within the grace period")
	}
	next := build()
	next[addrA] = selected
	require.Equal(t, 1, s.Observe(Observation{GMs: next, Best: addrA}))
	require.Equal(t, uint16(1), selected.PortOffset)
}

func TestSimpleIgnoresSilentRack(t *testing.T) {
	s := &Simple{Config: Config{Threshold: time.Microsecond}}
	gms := map[netip.Addr]*GM{
		addrA: gm(0, true),
		addrB: gm(0, false),
		addrC: gm(0, false),
	}
	// every other GM being silent is not evidence our own path is bad
	require.Zero(t, s.Observe(Observation{GMs: gms, Best: addrA}))
	require.Zero(t, gms[addrA].PortOffset)
}

func TestSimpleMissingSelected(t *testing.T) {
	s := &Simple{Config: Config{Threshold: time.Microsecond}}
	require.Zero(t, s.Observe(Observation{GMs: map[netip.Addr]*GM{addrB: gm(0, true)}, Best: addrA}))
}

func TestSimpleAloneWithSelected(t *testing.T) {
	s := &Simple{Config: Config{Threshold: time.Microsecond}}
	gms := map[netip.Addr]*GM{addrA: gm(9*time.Microsecond, true)}
	require.Zero(t, s.Observe(Observation{GMs: gms, Best: addrA}), "nothing to corroborate against")
}

func TestCorrectorNames(t *testing.T) {
	require.Equal(t, "simple", (&Simple{}).Name())
}

// peersAt turns bare offsets into distinct responders, as a real probe would
func peersAt(at time.Time, offsets ...time.Duration) []Peer {
	peers := make([]Peer, 0, len(offsets))
	for i, o := range offsets {
		peers = append(peers, Peer{
			Addr:   netip.AddrFrom4([4]byte{10, 0, 0, byte(i + 1)}),
			Offset: o,
			At:     at,
		})
	}
	return peers
}

func rackOf(offsets ...time.Duration) *Rack {
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 4}}
	r.Observe(Observation{Peers: peersAt(time.Now(), offsets...)})
	return r
}

func TestRackMovesPortWhenPeersAgree(t *testing.T) {
	r := rackOf(3000, 3100, 3200)
	gms := map[netip.Addr]*GM{addrA: gm(0, true)}

	require.Equal(t, 1, r.Observe(Observation{GMs: gms, Best: addrA}))
	require.Equal(t, uint16(1), gms[addrA].PortOffset)
	require.True(t, gms[addrA].Asymmetric)
}

func TestRackHoldsWhenPeersDisagree(t *testing.T) {
	// same median as above, but the peers do not agree with each other. Needs
	// more than the minimum quorum, since trimming leaves little to disagree over
	r := rackOf(-9000, -4000, 1000, 3000, 6000, 11000, 15000)
	gms := map[netip.Addr]*GM{addrA: gm(0, true)}

	require.Zero(t, r.Observe(Observation{GMs: gms, Best: addrA}), "spread exceeding the bias is noise")
	require.Zero(t, gms[addrA].PortOffset)
}

func TestRackIgnoresSmallBias(t *testing.T) {
	r := rackOf(100, 110, 120)
	gms := map[netip.Addr]*GM{addrA: gm(0, true)}
	require.Zero(t, r.Observe(Observation{GMs: gms, Best: addrA}))
}

func TestRackWithoutObservation(t *testing.T) {
	r := &Rack{Config: Config{Threshold: time.Microsecond}}
	require.Zero(t, r.Observe(Observation{GMs: map[netip.Addr]*GM{addrA: gm(0, true)}, Best: addrA}), "never probed")

	// an empty round must not overwrite nor create a reading
	r.Observe(Observation{Peers: nil})
	require.Zero(t, r.Observe(Observation{GMs: map[netip.Addr]*GM{addrA: gm(0, true)}, Best: addrA}))
}

func TestRackRejectsStaleAndThin(t *testing.T) {
	now := time.Now()

	r := &Rack{Config: Config{Threshold: time.Microsecond}}
	r.Observe(Observation{Peers: peersAt(now.Add(-2*rackMaxAge), 3000, 3100, 3200)})
	_, _, ok, _ := r.verdict(now)
	require.False(t, ok, "the probe runs outside sptp and can stop")

	thin := &Rack{Config: Config{Threshold: time.Microsecond}}
	thin.Observe(Observation{Peers: peersAt(now, 3000, 3100)})
	_, _, ok, _ = thin.verdict(now)
	require.False(t, ok, "two responders is not a rack")
}

func TestRackMissingSelected(t *testing.T) {
	r := rackOf(3000, 3100, 3200)
	require.Zero(t, r.Observe(Observation{GMs: map[netip.Addr]*GM{addrB: gm(0, true)}, Best: addrA}))
}

func TestRackName(t *testing.T) {
	require.Equal(t, "rack", (&Rack{}).Name())
}

// the two sources arrive on different cadences, so every corrector has to
// tolerate a round carrying only the one it does not weigh
func TestObserveToleratesEitherSource(t *testing.T) {
	// small enough that Rack has nothing to act on when the GM round follows
	peersOnly := Observation{Peers: peersAt(time.Now(), 10, 11, 12)}
	gmsOnly := Observation{GMs: map[netip.Addr]*GM{addrA: gm(0, true)}, Best: addrA, At: time.Now()}

	for _, c := range []Corrector{
		&Simple{Config: Config{Threshold: time.Microsecond}},
		&Rack{Config: Config{Threshold: time.Microsecond}},
	} {
		t.Run(c.Name(), func(t *testing.T) {
			require.Zero(t, c.Observe(Observation{At: time.Now()}), "an empty round")
			// peers alone carry no GM whose port could move, whatever they say
			require.Zero(t, c.Observe(peersOnly))
			require.Zero(t, c.Observe(gmsOnly), "a healthy GM and a quiet rack")
		})
	}
}

// Rack needs peer evidence from an earlier round to act on a later GM round
func TestRackActsAcrossRounds(t *testing.T) {
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 4}}
	now := time.Now()

	require.Zero(t, r.Observe(Observation{Peers: peersAt(now, 3000, 3100, 3200)}))

	gms := map[netip.Addr]*GM{addrA: gm(0, true)}
	require.Equal(t, 1, r.Observe(Observation{GMs: gms, Best: addrA, At: now}))
	require.Equal(t, uint16(1), gms[addrA].PortOffset)
}

// an answered but unjudgeable GM told us its path is fine, so it must not vote
// with the silent ones; one bad peer plus one unjudgeable peer is not unanimity
func TestSimpleUnjudgeableIsNotSuspicion(t *testing.T) {
	s := &Simple{Config: Config{Threshold: time.Microsecond}}
	gms := map[netip.Addr]*GM{
		addrA: gm(0, true),
		addrB: gm(5*time.Microsecond, true),
		addrC: unjudgeable(9 * time.Microsecond),
	}
	for range 10 {
		require.Zero(t, s.Observe(Observation{GMs: gms, Best: addrA}))
	}
	require.Zero(t, gms[addrA].PortOffset)
}

// the decision is recorded, never applied; the caller owns the mutation
func TestPortActionsAreRecordedNotApplied(t *testing.T) {
	g := gm(0, true)
	require.False(t, g.PortMoved)

	g.MovePort()
	require.True(t, g.PortMoved)
	require.Equal(t, uint16(1), g.PortOffset, "the view advances so a later check sees it")
}

// sync ticks outnumber peer probes by orders of magnitude, so one reading must
// justify one move -- not a move on every tick until the next probe lands
func TestRackSpendsEachReadingOnce(t *testing.T) {
	r := rackOf(3000, 3100, 3200)
	gms := map[netip.Addr]*GM{addrA: gm(0, true)}

	require.Equal(t, 1, r.Observe(Observation{GMs: gms, Best: addrA}))
	for range 10 {
		require.Zero(t, r.Observe(Observation{GMs: gms, Best: addrA}), "the reading is spent")
	}
	require.Equal(t, uint16(1), gms[addrA].PortOffset)

	// a fresh probe re-arms it, so a path that is still bad gets moved again
	r.Observe(Observation{Peers: peersAt(time.Now(), 3000, 3100, 3200)})
	require.Equal(t, 1, r.Observe(Observation{GMs: gms, Best: addrA}))
	require.Equal(t, uint16(2), gms[addrA].PortOffset)
}

// a reading measures the path we were on when it was taken. Carrying it across a
// failover would let one GM's evidence finish another GM's search, which reopens
// the refund a flapping grandmaster could otherwise exploit.
func TestRackReadingDoesNotSurviveGrandmasterChange(t *testing.T) {
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 1}}
	gms := map[netip.Addr]*GM{addrA: gm(0, true), addrB: gm(0, true)}

	spend := func(best netip.Addr) int {
		moves := 0
		for range 6 {
			r.Observe(Observation{Peers: peersAt(time.Now(), 3000, 3100, 3200)})
			moves += r.Observe(Observation{GMs: gms, Best: best})
		}
		return moves
	}
	require.Equal(t, 1, spend(addrA), "addrA spends its budget on a bad path")

	// a healthy reading taken while following addrB
	r.Observe(Observation{Peers: peersAt(time.Now(), -10, 0, 10)})
	require.Zero(t, r.Observe(Observation{GMs: gms, Best: addrB}))

	// flip back to addrA before any new probe: addrB's reading must not settle it
	require.Zero(t, r.Observe(Observation{GMs: gms, Best: addrA}))
	require.Zero(t, spend(addrA), "addrA is still spent")
}

// and the converse: a rack that does agree on a small median must still settle,
// including a perfectly synchronised one, which is never "decided"
func TestRackTightlyAgreedSmallMedianSettles(t *testing.T) {
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 1}}
	gms := map[netip.Addr]*GM{addrA: gm(0, true)}

	spend := func() int {
		moves := 0
		for range 6 {
			r.Observe(Observation{Peers: peersAt(time.Now(), 3000, 3100, 3200)})
			moves += r.Observe(Observation{GMs: gms, Best: addrA})
		}
		return moves
	}
	require.Equal(t, 1, spend(), "first episode spends its budget")

	r.Observe(Observation{Peers: peersAt(time.Now(), -10, 0, 10)})
	require.Zero(t, r.Observe(Observation{GMs: gms, Best: addrA}))

	require.Equal(t, 1, spend(), "a rack that agrees we are fine ends the episode")
}

// the freshness window has to outlive the gap between probes. At exactly one
// period a reading expires before its replacement lands, so a genuinely bad path
// reports healthy for part of every cycle and correction stops.
func TestRackReadingSurvivesUntilTheNextProbe(t *testing.T) {
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 4}}
	now := time.Now()
	r.Observe(Observation{Peers: peersAt(now, 3000, 3100, 3200)})

	// one probe period plus collection time and scheduler jitter
	_, _, known, _ := r.verdict(now.Add(6 * time.Minute))
	require.True(t, known, "a reading must outlast the gap to its replacement")

	_, _, known, _ = r.verdict(now.Add(rackMaxAge + time.Second))
	require.False(t, known, "but a probe that stopped arriving must stop steering")
}

// the rack measures the path we are following and nothing else, so a GM we
// failed away from must not keep its old verdict for the life of the daemon
func TestRackClearsVerdictOnGrandmasterWeLeft(t *testing.T) {
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 4}}
	gms := map[netip.Addr]*GM{addrA: gm(0, true), addrB: gm(0, true)}

	r.Observe(Observation{Peers: peersAt(time.Now(), 3000, 3100, 3200)})
	require.Equal(t, 1, r.Observe(Observation{GMs: gms, Best: addrA}))
	require.True(t, gms[addrA].Asymmetric)

	// failing over drops the reading, so nothing is known about either path
	require.Zero(t, r.Observe(Observation{GMs: gms, Best: addrB}))
	require.False(t, gms[addrA].Asymmetric, "we know nothing about the one we left")

	// a fresh probe measures the path we are on now
	r.Observe(Observation{Peers: peersAt(time.Now(), 3000, 3100, 3200)})
	require.Equal(t, 1, r.Observe(Observation{GMs: gms, Best: addrB}))
	require.True(t, gms[addrB].Asymmetric, "the path we are on now is bad")
	require.False(t, gms[addrA].Asymmetric, "and the one we left stays unjudged")
}

// the Go zero value has to be the safe one: an operator reaching for zero to
// freeze a fleet, or a config that simply omits the field, must get no moves
func TestRackZeroBudgetDisablesTheSearch(t *testing.T) {
	r := &Rack{Config: Config{Threshold: time.Microsecond}}
	gms := map[netip.Addr]*GM{addrA: gm(0, true)}

	moves := 0
	for range 5 {
		r.Observe(Observation{Peers: peersAt(time.Now(), 3000, 3100, 3200)})
		moves += r.Observe(Observation{GMs: gms, Best: addrA})
	}
	require.Zero(t, moves, "a zero budget moves nothing")
	require.Zero(t, gms[addrA].PortOffset)
	require.True(t, gms[addrA].Asymmetric, "but the bad path is still reported")
}

// the budget bounds effort, not coverage: offsets are hashed to send workers, so
// a search samples with replacement and never proves it tried them all and moving again achieves nothing
func TestRackStopsAtMaxPortChanges(t *testing.T) {
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 2}}
	gms := map[netip.Addr]*GM{addrA: gm(0, true)}

	moves := 0
	for range 10 {
		r.Observe(Observation{Peers: peersAt(time.Now(), 3000, 3100, 3200)})
		moves += r.Observe(Observation{GMs: gms, Best: addrA})
	}
	require.Equal(t, 2, moves, "MaxPortChanges of 2 buys exactly two moves")
	require.Equal(t, uint16(2), gms[addrA].PortOffset)
}

// a probe below quorum must be dropped on arrival, not stored and rejected
// later: storing it would clear spent and discard a reading that was actionable
func TestRackThinProbeKeepsLastReading(t *testing.T) {
	r := rackOf(3000, 3100, 3200)
	now := time.Now()

	r.Observe(Observation{Peers: peersAt(now, 9000, 9100)})

	median, _, ok, _ := r.verdict(now)
	require.True(t, ok, "the earlier full reading is still actionable")
	require.Equal(t, 3100*time.Nanosecond, median, "and was not overwritten")

	gms := map[netip.Addr]*GM{addrA: gm(0, true)}
	require.Equal(t, 1, r.Observe(Observation{GMs: gms, Best: addrA}))
}

// the flag says "asymmetric right now", so a rack that has gone quiet must clear
// it -- every path in correct can return early, including the clean one
func TestRackClearsAsymmetricWhenRackIsQuiet(t *testing.T) {
	r := rackOf(3000, 3100, 3200)
	gms := map[netip.Addr]*GM{addrA: gm(0, true)}
	require.Equal(t, 1, r.Observe(Observation{GMs: gms, Best: addrA}))
	require.True(t, gms[addrA].Asymmetric)

	// the path is fixed and peers now agree we are fine
	r.Observe(Observation{Peers: peersAt(time.Now(), 10, 11, 12)})
	require.Zero(t, r.Observe(Observation{GMs: gms, Best: addrA}))
	require.False(t, gms[addrA].Asymmetric, "a quiet rack must clear the flag")
}

// one wild peer must count the same whether it sits above or below the median,
// so the same outlier either side yields the same verdict
func TestRackSpreadIsDirectionSymmetric(t *testing.T) {
	now := time.Now()
	cfg := Config{Threshold: time.Microsecond, MaxPortChanges: 4}

	high := &Rack{Config: cfg}
	high.Observe(Observation{Peers: peersAt(now, 3000, 3100, 3200, 90000)})
	_, highQuiet, ok, _ := high.verdict(now)
	require.True(t, ok)

	low := &Rack{Config: cfg}
	low.Observe(Observation{Peers: peersAt(now, -90000, 3000, 3100, 3200)})
	_, lowQuiet, ok, _ := low.verdict(now)
	require.True(t, ok)

	require.Equal(t, highQuiet, lowQuiet)
}

// a probe landing while a decision is in flight must not have its fresh reading
// consumed by the move the previous one justified
func TestRackTakeBiasRejectsReplacedReading(t *testing.T) {
	r := rackOf(3000, 3100, 3200)
	stale := time.Now().Add(-time.Second)
	require.False(t, r.takeBias(addrA, stale), "a reading that is no longer current cannot be spent")

	r.mu.RLock()
	current := r.reading.at
	r.mu.RUnlock()
	require.True(t, r.takeBias(addrA, current))
	require.False(t, r.takeBias(addrA, current), "and only once")
}

// quorum counts distinct peers, not exchanges: one chatty peer answering three
// times is still one opinion about our path
func TestRackQuorumCountsDistinctPeers(t *testing.T) {
	now := time.Now()
	one := netip.MustParseAddr("10.0.0.1")
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 4}}
	r.Observe(Observation{Peers: []Peer{
		{Addr: one, Offset: 3000, At: now},
		{Addr: one, Offset: 3100, At: now.Add(time.Second)},
		{Addr: one, Offset: 3200, At: now.Add(2 * time.Second)},
	}})

	_, _, known, _ := r.verdict(now.Add(2 * time.Second))
	require.False(t, known, "three exchanges with one peer is not a quorum")
}

// a reading ages from when the exchange happened, not when it was handed over
func TestRackAgesByMeasurementTime(t *testing.T) {
	now := time.Now()
	r := &Rack{Config: Config{Threshold: time.Microsecond}}
	// handed over now, but measured well beyond the staleness window
	r.Observe(Observation{Peers: peersAt(now.Add(-2*rackMaxAge), 3000, 3100, 3200)})

	_, _, known, _ := r.verdict(now)
	require.False(t, known, "an old exchange is stale however recently it arrived")
}

// the flag describes the path, so a reading already spent on a move still
// reports the fault rather than looking healthy on the next tick
func TestRackKeepsAsymmetricWhileBiasPersists(t *testing.T) {
	r := rackOf(3000, 3100, 3200)
	gms := map[netip.Addr]*GM{addrA: gm(0, true)}

	require.Equal(t, 1, r.Observe(Observation{GMs: gms, Best: addrA}))
	require.True(t, gms[addrA].Asymmetric)

	// same reading, now spent: no further move, but the path is still bad
	require.Zero(t, r.Observe(Observation{GMs: gms, Best: addrA}))
	require.True(t, gms[addrA].Asymmetric, "a spent reading still describes a bad path")
}

// and an exhausted search budget must not report healthy either
func TestRackKeepsAsymmetricWhenBudgetExhausted(t *testing.T) {
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 1}}
	gms := map[netip.Addr]*GM{addrA: gm(0, true)}

	// each probe buys one move, so spend the budget through the real path
	moves := 0
	for range 6 {
		r.Observe(Observation{Peers: peersAt(time.Now(), 3000, 3100, 3200)})
		moves += r.Observe(Observation{GMs: gms, Best: addrA})
	}
	require.Equal(t, 1, moves, "MaxPortChanges of 1 buys exactly one move")
	require.True(t, gms[addrA].Asymmetric, "the path is still bad after the search gives up")
}

// the budget bounds one search, not the daemon's life. Without a reset a host
// that searched this morning would sit deaf to a fault appearing tonight.
func TestRackBudgetResetsWhenRackGoesQuiet(t *testing.T) {
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 1}}
	gms := map[netip.Addr]*GM{addrA: gm(0, true)}

	spend := func() int {
		moves := 0
		for range 6 {
			r.Observe(Observation{Peers: peersAt(time.Now(), 3000, 3100, 3200)})
			moves += r.Observe(Observation{GMs: gms, Best: addrA})
		}
		return moves
	}
	require.Equal(t, 1, spend(), "first episode spends its budget")
	require.Zero(t, spend(), "and stays spent while the rack still objects")

	// the rack goes quiet: the search worked, or the fault moved on
	r.Observe(Observation{Peers: peersAt(time.Now(), 10, 11, 12)})
	require.Zero(t, r.Observe(Observation{GMs: gms, Best: addrA}))

	require.Equal(t, 1, spend(), "a later fault gets a full budget again")
}

// quiet is also true when the peers cannot agree. Treating that as the end of an
// episode would let one noisy probe refill the budget of a host that never
// settles, which is the whole population the budget exists to bound.
func TestRackUndecidedProbeDoesNotRefillBudget(t *testing.T) {
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 1}}
	gms := map[netip.Addr]*GM{addrA: gm(0, true)}

	spend := func() int {
		moves := 0
		for range 6 {
			r.Observe(Observation{Peers: peersAt(time.Now(), 3000, 3100, 3200)})
			moves += r.Observe(Observation{GMs: gms, Best: addrA})
		}
		return moves
	}
	require.Equal(t, 1, spend(), "first episode spends its budget")

	// a wide probe: still biased, but the peers no longer place the median
	r.Observe(Observation{Peers: peersAt(time.Now(), -4000, 3000, 9000)})
	require.Zero(t, r.Observe(Observation{GMs: gms, Best: addrA}))

	require.Zero(t, spend(), "an undecided reading is not a settled one")
}

// each grandmaster is reached over its own path, so failing over to one nobody
// has searched must not inherit the spent budget of the one we left
func TestRackBudgetIsPerGrandmaster(t *testing.T) {
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 1}}
	gms := map[netip.Addr]*GM{addrA: gm(0, true), addrB: gm(0, true)}

	spend := func(best netip.Addr) int {
		moves := 0
		for range 6 {
			r.Observe(Observation{Peers: peersAt(time.Now(), 3000, 3100, 3200)})
			moves += r.Observe(Observation{GMs: gms, Best: best})
		}
		return moves
	}
	require.Equal(t, 1, spend(addrA), "addrA spends its budget")
	require.Zero(t, spend(addrA), "and stays spent")
	require.Equal(t, 1, spend(addrB), "addrB has its own path and its own budget")
}

// settling while following one GM says nothing about the others. Clearing them
// too would give a known-bad path a fresh budget every time the host fails back
// to it, so a flapping grandmaster would fund an unbounded search.
func TestRackSettleDoesNotRefundOtherGrandmasters(t *testing.T) {
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 1}}
	gms := map[netip.Addr]*GM{addrA: gm(0, true), addrB: gm(0, true)}

	spend := func(best netip.Addr) int {
		moves := 0
		for range 6 {
			r.Observe(Observation{Peers: peersAt(time.Now(), 3000, 3100, 3200)})
			moves += r.Observe(Observation{GMs: gms, Best: best})
		}
		return moves
	}
	require.Equal(t, 1, spend(addrA), "addrA spends its budget on a bad path")

	// fail over to addrB, whose path turns out to be fine
	r.Observe(Observation{Peers: peersAt(time.Now(), 10, 11, 12)})
	require.Zero(t, r.Observe(Observation{GMs: gms, Best: addrB}))

	require.Zero(t, spend(addrA), "failing back finds addrA still spent")
}

// at the minimum quorum there is nothing left to compare once both sides are
// trimmed, so the spread has to fall back to the full span: three peers that do
// not agree must hold, not move a port on a median that only looks convincing
func TestRackMinimumQuorumSpreadIsNotDegenerate(t *testing.T) {
	noisy := rackOf(3000, 3100, 12000)
	gms := map[netip.Addr]*GM{addrA: gm(0, true)}
	require.Zero(t, noisy.Observe(Observation{GMs: gms, Best: addrA}),
		"one wild peer in three is disagreement, not consensus")
	require.Zero(t, gms[addrA].PortOffset)

	// three peers that do agree still act
	tight := rackOf(3000, 3050, 3100)
	gms = map[netip.Addr]*GM{addrA: gm(0, true)}
	require.Equal(t, 1, tight.Observe(Observation{GMs: gms, Best: addrA}))
}

// the flag is written back to the client every GM round, so evidence that aged
// out must clear it rather than leave a bad path latched forever
func TestRackClearsAsymmetricWhenEvidenceAgesOut(t *testing.T) {
	r := rackOf(3000, 3100, 3200)
	gms := map[netip.Addr]*GM{addrA: gm(0, true)}
	require.Equal(t, 1, r.Observe(Observation{GMs: gms, Best: addrA}))
	require.True(t, gms[addrA].Asymmetric)

	// the probe stops; the reading ages past rackMaxAge
	r.mu.Lock()
	r.reading.at = time.Now().Add(-2 * rackMaxAge)
	r.mu.Unlock()

	require.Zero(t, r.Observe(Observation{GMs: gms, Best: addrA}))
	require.False(t, gms[addrA].Asymmetric, "no evidence is not evidence of a bad path")
}

// an unusable reading still reports when it was measured, so a stale arm can be
// told apart from one that never probed
func TestRackVerdictKeepsMeasuredWhenUnusable(t *testing.T) {
	now := time.Now()
	stale := now.Add(-2 * rackMaxAge)
	r := &Rack{Config: Config{Threshold: time.Microsecond}}
	r.Observe(Observation{Peers: peersAt(stale, 3000, 3100, 3200)})

	_, _, known, measured := r.verdict(now)
	require.False(t, known)
	require.Equal(t, stale.Unix(), measured.Unix(), "how stale, not just that it is stale")

	_, _, _, never := (&Rack{}).verdict(now)
	require.True(t, never.IsZero(), "and zero when nothing ever probed")
}

// a switch that does not correct residence time spreads every peer offset by
// microseconds, which the old spread-against-median guard read as disagreement
// and refused to act on however many peers agreed on a centre
func TestRackActsOnWideSpreadWithManyPeers(t *testing.T) {
	now := time.Now()
	offsets := make([]time.Duration, 0, 45)
	for i := range 45 {
		offsets = append(offsets, 2000+time.Duration((i%9)*600-2400))
	}
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 4}}
	r.Observe(Observation{Peers: peersAt(now, offsets...)})

	_, _, known, _ := r.verdict(now)
	require.True(t, known)
	gms := map[netip.Addr]*GM{addrA: gm(0, true)}
	require.Equal(t, 1, r.Observe(Observation{GMs: gms, Best: addrA}),
		"45 peers centred on 2us is a verdict even when the spread exceeds it")
}

// and the converse: a bare quorum is weak evidence however tidy it looks
func TestRackHoldsOnThinQuorumWithLooseSpread(t *testing.T) {
	now := time.Now()
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 4}}
	r.Observe(Observation{Peers: peersAt(now, 1000, 1500, 2000)})

	gms := map[netip.Addr]*GM{addrA: gm(0, true)}
	require.Zero(t, r.Observe(Observation{GMs: gms, Best: addrA}),
		"three peers spanning 1us do not place the median")
	require.Zero(t, gms[addrA].PortOffset)
}

// sample size is what separates them: the same spread decides with enough peers
func TestRackDecidedScalesWithPeerCount(t *testing.T) {
	now := time.Now()
	thin := &Rack{Config: Config{Threshold: time.Microsecond}}
	thin.Observe(Observation{Peers: peersAt(now, 1000, 1500, 2000)})
	_, thinQuiet, _, _ := thin.verdict(now)
	require.True(t, thinQuiet)

	wide := make([]time.Duration, 0, 40)
	for i := range 40 {
		wide = append(wide, 1500+time.Duration((i%5)*250-500))
	}
	many := &Rack{Config: Config{Threshold: time.Microsecond}}
	many.Observe(Observation{Peers: peersAt(now, wide...)})
	_, manyQuiet, _, _ := many.verdict(now)
	require.False(t, manyQuiet, "same centre, same spread, more peers")
}

// One threshold. Under it the host is settled and the search count clears;
// over it the corrector keeps working.
func TestRackSettlesAtThreshold(t *testing.T) {
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 10}}
	gm := netip.MustParseAddr("2401:db00::1")
	gms := map[netip.Addr]*GM{gm: {Answered: true, Judgeable: true}}

	r.Observe(Observation{Peers: peersAt(time.Now(), 3000, 3100, 3200), GMs: gms, Best: gm})
	require.Equal(t, uint16(1), r.PortMoves(gm), "over threshold, the search advances")

	r.Observe(Observation{Peers: peersAt(time.Now(), 400, 500, 600), GMs: gms, Best: gm})
	require.Zero(t, r.PortMoves(gm), "under threshold is settled, so the count clears")
}

// One threshold, the same one everywhere: a path over 1us gets searched.
func TestRackConvictsOtherGMsOverThreshold(t *testing.T) {
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 10}}
	best := netip.MustParseAddr("2401:db00::1")
	bad := netip.MustParseAddr("2401:db00::2")
	near := netip.MustParseAddr("2401:db00::3")
	gms := map[netip.Addr]*GM{
		best: {Answered: true, Judgeable: true, Offset: 17 * time.Nanosecond},
		bad:  {Answered: true, Judgeable: true, Offset: 3962 * time.Nanosecond},
		near: {Answered: true, Judgeable: true, Offset: 1500 * time.Nanosecond},
	}

	// our own clock sits at ~890ns against the rack, well short of perfect
	r.Observe(Observation{Peers: peersAt(time.Now(), 700, 890, 1000), GMs: gms, Best: best})

	require.Equal(t, uint16(1), gms[bad].PortOffset, "over threshold")
	require.Equal(t, uint16(1), gms[near].PortOffset, "1.5us is over 1us too")
	require.Zero(t, gms[best].PortOffset, "the selected path is fine")
}

// The ports move once per probe, not once per sync tick. Ticks outnumber probes
// by orders of magnitude, so without this a GM burns its whole budget in seconds.
func TestRackCorrectsOthersOncePerReading(t *testing.T) {
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 10}}
	best := netip.MustParseAddr("2401:db00::1")
	bad := netip.MustParseAddr("2401:db00::2")
	gms := map[netip.Addr]*GM{
		best: {Answered: true, Judgeable: true, Offset: 17 * time.Nanosecond},
		bad:  {Answered: true, Judgeable: true, Offset: 3962 * time.Nanosecond},
	}

	r.Observe(Observation{Peers: peersAt(time.Now(), 10, 20, 30)})
	for range 20 {
		r.Observe(Observation{GMs: gms, Best: best})
	}

	require.Equal(t, uint16(1), gms[bad].PortOffset, "twenty ticks, one reading, one move")
}

// A secondary GM's budget is refunded when its path comes good, the same way the
// selected GM's is. Without it a GM that exhausts its search is deaf to every
// later fault, which is the whole reason the budget is per episode.
func TestRackRefundsOtherGMWhenItsPathComesGood(t *testing.T) {
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 2}}
	best := netip.MustParseAddr("2401:db00::1")
	bad := netip.MustParseAddr("2401:db00::2")
	gms := map[netip.Addr]*GM{
		best: {Answered: true, Judgeable: true, Offset: 17 * time.Nanosecond},
		bad:  {Answered: true, Judgeable: true, Offset: 4 * time.Microsecond},
	}

	for range 2 {
		r.Observe(Observation{Peers: peersAt(time.Now(), 10, 20, 30)})
		r.Observe(Observation{GMs: gms, Best: best})
	}
	require.Equal(t, uint16(2), r.PortMoves(bad), "budget spent")

	gms[bad].Offset = 100 * time.Nanosecond
	r.Observe(Observation{Peers: peersAt(time.Now(), 10, 20, 30)})
	r.Observe(Observation{GMs: gms, Best: best})
	require.Zero(t, r.PortMoves(bad), "a good path ends the search")

	gms[bad].Offset = 4 * time.Microsecond
	r.Observe(Observation{Peers: peersAt(time.Now(), 10, 20, 30)})
	r.Observe(Observation{GMs: gms, Best: best})
	require.Equal(t, uint16(1), r.PortMoves(bad), "a later fault gets a fresh budget")
}

// A GM that drops out of the list has no search left to report. Leaving the
// count behind would strand it non-zero for as long as the daemon runs.
func TestSimpleClearsWhenSelectedLeavesTheList(t *testing.T) {
	s := &Simple{Config: Config{Threshold: time.Microsecond, MaxConsecutive: 0}}
	best := netip.MustParseAddr("2401:db00::1")
	other := netip.MustParseAddr("2401:db00::2")
	gms := map[netip.Addr]*GM{
		best:  {Answered: true, Judgeable: true, Offset: 50 * time.Nanosecond},
		other: {Answered: true, Judgeable: true, Offset: 4 * time.Microsecond},
	}

	for range 3 {
		s.Observe(Observation{GMs: gms, Best: best})
	}
	require.NotZero(t, s.PortMoves(best), "searching")

	delete(gms, best)
	s.Observe(Observation{GMs: gms, Best: best})
	require.Zero(t, s.PortMoves(best), "gone from the list, so no search to report")
}

// A convicted GM stays flagged between probes. The flag carries across ticks, so
// clearing it on every tick would flap it false whenever there is no new reading.
func TestRackKeepsOtherGMFlaggedBetweenProbes(t *testing.T) {
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 10}}
	best := netip.MustParseAddr("2401:db00::1")
	bad := netip.MustParseAddr("2401:db00::2")
	gms := map[netip.Addr]*GM{
		best: {Answered: true, Judgeable: true, Offset: 17 * time.Nanosecond},
		bad:  {Answered: true, Judgeable: true, Offset: 4 * time.Microsecond},
	}

	r.Observe(Observation{Peers: peersAt(time.Now(), 10, 20, 30)})
	r.Observe(Observation{GMs: gms, Best: best})
	require.True(t, gms[bad].Asymmetric, "convicted")

	// ticks with no new probe must not clear it
	for range 5 {
		r.Observe(Observation{GMs: gms, Best: best})
	}
	require.True(t, gms[bad].Asymmetric, "still bad, still flagged")
}

// A rack that cannot agree is quiet but says nothing about our clock, so it is
// no grounds to spend another GM's budget.
func TestRackDoesNotConvictOthersWhenPeersDisagree(t *testing.T) {
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 10}}
	best := netip.MustParseAddr("2401:db00::1")
	bad := netip.MustParseAddr("2401:db00::2")
	gms := map[netip.Addr]*GM{
		best: {Answered: true, Judgeable: true, Offset: 17 * time.Nanosecond},
		bad:  {Answered: true, Judgeable: true, Offset: 4 * time.Microsecond},
	}

	// peers scattered so wide the median means nothing, yet it lands near zero
	r.Observe(Observation{Peers: peersAt(time.Now(), -40000, 100, 40000), GMs: gms, Best: best})

	require.Zero(t, gms[bad].PortOffset, "an unmeasurable rack convicts nobody")
}

// Peer probes reach every corrector, grandmaster map empty. Simple must not read
// that as "these GMs are gone" and clear a search it is still running.
func TestSimpleKeepsSearchAcrossPeerObservations(t *testing.T) {
	s := &Simple{Config: Config{Threshold: time.Microsecond, MaxConsecutive: 0}}
	best := netip.MustParseAddr("2401:db00::1")
	other := netip.MustParseAddr("2401:db00::2")
	gms := map[netip.Addr]*GM{
		best:  {Answered: true, Judgeable: true, Offset: 50 * time.Nanosecond},
		other: {Answered: true, Judgeable: true, Offset: 4 * time.Microsecond},
	}

	for range 3 {
		s.Observe(Observation{GMs: gms, Best: best})
	}
	searching := s.PortMoves(best)
	require.NotZero(t, searching, "searching")

	s.Observe(Observation{Peers: peersAt(time.Now(), 10, 20, 30)})
	require.Equal(t, searching, s.PortMoves(best), "a peer probe must not end the search")
}
