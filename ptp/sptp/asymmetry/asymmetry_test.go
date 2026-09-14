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

func TestComplexMovesEachGMIndependently(t *testing.T) {
	c := &Complex{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 4}}
	gms := map[netip.Addr]*GM{
		addrA: gm(0, true),
		addrB: gm(5*time.Microsecond, true),
		addrC: gm(0, true),
	}
	require.Equal(t, 1, c.Observe(Observation{GMs: gms, Best: addrA}), "only B looks asymmetric")
	require.True(t, gms[addrB].Asymmetric)
	require.False(t, gms[addrC].Asymmetric)
	require.Zero(t, gms[addrA].PortOffset, "the selected GM is untouched")
}

func TestComplexBlamesSelectedAfterMaxPortChanges(t *testing.T) {
	c := &Complex{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 2}}
	b := gm(5*time.Microsecond, true)
	b.PortOffset = 3 // already searched past the limit
	gms := map[netip.Addr]*GM{addrA: gm(0, true), addrB: b}

	c.Observe(Observation{GMs: gms, Best: addrA})
	require.Equal(t, uint16(1), gms[addrA].PortOffset, "selected GM is blamed")
	require.Zero(t, b.PortOffset, "the others are reset so the search restarts")
}

func TestComplexResetsStreakWhenClean(t *testing.T) {
	c := &Complex{Config: Config{Threshold: time.Microsecond, MaxConsecutive: 5, MaxPortChanges: 9}}
	b := gm(10*time.Nanosecond, true)
	b.Streak = 4
	c.Observe(Observation{GMs: map[netip.Addr]*GM{addrA: gm(0, true), addrB: b}, Best: addrA})
	require.Zero(t, b.Streak)
	require.False(t, b.Asymmetric)
}

func TestCorrectorNames(t *testing.T) {
	require.Equal(t, "simple", (&Simple{}).Name())
	require.Equal(t, "complex", (&Complex{}).Name())
}

// the complex path can move a GM's port and then, in the same round, stop
// searching it and reset; the reset has to win or the port lands on 1 not 0
func TestResetPortCancelsMove(t *testing.T) {
	g := gm(0, true)
	g.MovePort()
	g.ResetPort()

	require.True(t, g.PortReset)
	require.False(t, g.PortMoved, "a reset in the same round cancels the move")
	require.Zero(t, g.PortOffset)
}

func TestComplexResetLandsOnZero(t *testing.T) {
	c := &Complex{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 2}}
	// suspicious and already past the search limit, so it is moved then blamed away
	b := gm(5*time.Microsecond, true)
	b.PortOffset = 3
	b.Streak = 99

	c.Observe(Observation{GMs: map[netip.Addr]*GM{addrA: gm(0, true), addrB: b}, Best: addrA})
	require.Zero(t, b.PortOffset)
	require.False(t, b.PortMoved, "the reset must not leave a pending move behind")
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
		&Complex{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 4}},
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
	r := &Rack{Config: Config{Threshold: time.Microsecond}}
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

// a GM that says nothing keeps the streak it earned, so an intermittent GM can
// still reach the grace limit
func TestComplexSilentGMKeepsStreak(t *testing.T) {
	c := &Complex{Config: Config{Threshold: time.Microsecond, MaxConsecutive: 2, MaxPortChanges: 9}}
	b := gm(5*time.Microsecond, true)
	b.Streak = 2

	silent := gm(0, false)
	silent.Streak = 2
	c.Observe(Observation{GMs: map[netip.Addr]*GM{addrA: gm(0, true), addrC: silent}, Best: addrA})
	require.Equal(t, 2, silent.Streak, "silence must not clear the streak")

	// and an answered clean measurement still does
	clean := gm(10*time.Nanosecond, true)
	clean.Streak = 2
	c.Observe(Observation{GMs: map[netip.Addr]*GM{addrA: gm(0, true), addrC: clean}, Best: addrA})
	require.Zero(t, clean.Streak)
}

// the decision is recorded, never applied; the caller owns the mutation
func TestPortActionsAreRecordedNotApplied(t *testing.T) {
	g := gm(0, true)
	require.False(t, g.PortMoved)

	g.MovePort()
	require.True(t, g.PortMoved)
	require.Equal(t, uint16(1), g.PortOffset, "the view advances so a later check sees it")

	g.ResetPort()
	require.True(t, g.PortReset)
	require.Zero(t, g.PortOffset)
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

// a port offset only picks a send worker, so past MaxPortChanges the search has
// seen every distinct return path and moving again achieves nothing
func TestRackStopsAtMaxPortChanges(t *testing.T) {
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 2}}
	gms := map[netip.Addr]*GM{addrA: gm(0, true)}

	moves := 0
	for range 10 {
		r.Observe(Observation{Peers: peersAt(time.Now(), 3000, 3100, 3200)})
		moves += r.Observe(Observation{GMs: gms, Best: addrA})
	}
	require.Equal(t, 3, moves, "offsets 1..3, then the budget is spent")
	require.Equal(t, uint16(3), gms[addrA].PortOffset)
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
	require.False(t, r.takeBias(stale), "a reading that is no longer current cannot be spent")

	r.mu.RLock()
	current := r.measured
	r.mu.RUnlock()
	require.True(t, r.takeBias(current))
	require.False(t, r.takeBias(current), "and only once")
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
	r := &Rack{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 0}}
	r.Observe(Observation{Peers: peersAt(time.Now(), 3000, 3100, 3200)})

	gms := map[netip.Addr]*GM{addrA: gm(0, true)}
	gms[addrA].PortOffset = 5

	require.Zero(t, r.Observe(Observation{GMs: gms, Best: addrA}), "budget spent")
	require.True(t, gms[addrA].Asymmetric, "but the path is still bad")
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
	r.measured = time.Now().Add(-2 * rackMaxAge)
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
