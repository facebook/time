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
	require.Zero(t, s.Correct(gms, addrA))
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
		require.Zero(t, s.Correct(gms, addrA), "still within the grace period")
	}
	next := build()
	next[addrA] = selected
	require.Equal(t, 1, s.Correct(next, addrA))
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
	require.Zero(t, s.Correct(gms, addrA))
	require.Zero(t, gms[addrA].PortOffset)
}

func TestSimpleMissingSelected(t *testing.T) {
	s := &Simple{Config: Config{Threshold: time.Microsecond}}
	require.Zero(t, s.Correct(map[netip.Addr]*GM{addrB: gm(0, true)}, addrA))
}

func TestSimpleAloneWithSelected(t *testing.T) {
	s := &Simple{Config: Config{Threshold: time.Microsecond}}
	gms := map[netip.Addr]*GM{addrA: gm(9*time.Microsecond, true)}
	require.Zero(t, s.Correct(gms, addrA), "nothing to corroborate against")
}

func TestComplexMovesEachGMIndependently(t *testing.T) {
	c := &Complex{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 4}}
	gms := map[netip.Addr]*GM{
		addrA: gm(0, true),
		addrB: gm(5*time.Microsecond, true),
		addrC: gm(0, true),
	}
	require.Equal(t, 1, c.Correct(gms, addrA), "only B looks asymmetric")
	require.True(t, gms[addrB].Asymmetric)
	require.False(t, gms[addrC].Asymmetric)
	require.Zero(t, gms[addrA].PortOffset, "the selected GM is untouched")
}

func TestComplexBlamesSelectedAfterMaxPortChanges(t *testing.T) {
	c := &Complex{Config: Config{Threshold: time.Microsecond, MaxPortChanges: 2}}
	b := gm(5*time.Microsecond, true)
	b.PortOffset = 3 // already searched past the limit
	gms := map[netip.Addr]*GM{addrA: gm(0, true), addrB: b}

	c.Correct(gms, addrA)
	require.Equal(t, uint16(1), gms[addrA].PortOffset, "selected GM is blamed")
	require.Zero(t, b.PortOffset, "the others are reset so the search restarts")
}

func TestComplexResetsStreakWhenClean(t *testing.T) {
	c := &Complex{Config: Config{Threshold: time.Microsecond, MaxConsecutive: 5, MaxPortChanges: 9}}
	b := gm(10*time.Nanosecond, true)
	b.Streak = 4
	c.Correct(map[netip.Addr]*GM{addrA: gm(0, true), addrB: b}, addrA)
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

	c.Correct(map[netip.Addr]*GM{addrA: gm(0, true), addrB: b}, addrA)
	require.Zero(t, b.PortOffset)
	require.False(t, b.PortMoved, "the reset must not leave a pending move behind")
}
