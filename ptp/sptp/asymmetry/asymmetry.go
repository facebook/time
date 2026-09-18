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

// Package asymmetry decides when a grandmaster's response port should move.
//
// A path whose two directions differ leaves the servo steering to the wrong
// time while reporting a healthy offset, so the fault is invisible in the
// offset alone. Moving the response port lands the return traffic on a
// different ptp4u send worker, and so a different ECMP hash.
//
// The package holds no PTP state. Callers pass a GM per grandmaster and the
// correction records its decision through that interface.
package asymmetry

import (
	"maps"
	"net/netip"
	"sync"
	"time"
)

// GM is one grandmaster as a correction sees it: a measurement to judge, the
// response-port offset to move, and the streak bookkeeping between ticks.
//
// Answered and unusable is not the same as silent. A GM that replied with a
// rejected delay or a clock class we do not follow has told us its path is fine;
// a GM that said nothing has told us nothing, which the unanimity vote reads as
// suspicion. Collapsing the two lets one silent peer plus one bad peer move a
// port that neither alone would.
type GM struct {
	// Offset measured against this GM. Meaningful only when Answered is set.
	Offset time.Duration
	// Answered is false when the GM produced no measurement this tick.
	Answered bool
	// Judgeable is false when the measurement cannot be weighed: the delay was
	// rejected, or the GM is not a clock source we follow.
	Judgeable bool

	// PortOffset is the current response-port offset.
	PortOffset uint16
	// PortMoved is whether this correction asked the port to advance. The caller
	// applies it, so the decision stays separate from the mutation.
	PortMoved bool

	// Asymmetric marks this GM as suspected during the current tick.
	Asymmetric bool
	// Streak counts consecutive asymmetric measurements.
	Streak int
}

// MovePort records that this GM's response port should advance. A tick decides
// at most one move per GM.
func (g *GM) MovePort() {
	g.PortMoved = true
	g.PortOffset++
}

// Config tunes the corrections.
type Config struct {
	// Threshold above which an offset is suspicious. Simple reads it
	// against a grandmaster offset, Rack against the in-rack peer median; the two
	// cannot be tuned apart.
	Threshold time.Duration
	// MaxConsecutive measurements a GM may look asymmetric before its port moves.
	MaxConsecutive uint16
	// MaxPortChanges is how many moves one search may spend; zero spends none.
	// Rack only -- simple moves the selected port whenever the grandmasters are
	// unanimous, without a budget.
	MaxPortChanges uint16
}

// Observation is one round of evidence about this host's clock. Each source
// arrives on its own cadence -- grandmasters every sync tick, in-rack peers
// whenever the peer delay probe runs -- so a corrector is given whichever
// arrived and keeps what it weighs.
type Observation struct {
	// GMs is what each grandmaster reported, keyed by address. Set on a sync tick.
	GMs map[netip.Addr]*GM
	// Best is the currently selected grandmaster. Set alongside GMs.
	Best netip.Addr
	// Peers is what each in-rack peer reported. Set when a peer delay probe
	// completes; an unusable exchange is left out rather than counted as
	// agreement.
	Peers []Peer
	// At is when this round was measured.
	At time.Time
}

// Peer is one in-rack measurement. Identity and measurement time are kept
// separate from the offset so a corrector can drop duplicate or stale peers
// before it counts a quorum.
type Peer struct {
	// Addr is the responder, which identifies the peer across probes.
	Addr netip.Addr
	// Offset is this host against that peer.
	Offset time.Duration
	// At is when the exchange was measured, not when it was handed over.
	At time.Time
}

// Corrector decides whether any response port should move. Name identifies the
// implementation in stats, so an experiment can attribute a host to its arm.
type Corrector interface {
	// Observe takes one round of evidence and returns how many GMs had their
	// port moved. A round carrying nothing the corrector weighs moves none.
	Observe(obs Observation) int
	// ports tried for gm in the current search; zero means settled
	PortMoves(gm netip.Addr) uint16
	Name() string
}

// searchCounter counts ports tried per grandmaster since its path last looked
// good. Every corrector needs exactly this and each hand-rolled copy grew its
// own way of stranding a count, so there is one.
type searchCounter struct {
	mu sync.RWMutex
	n  map[netip.Addr]uint16
}

func (c *searchCounter) count(gm netip.Addr) uint16 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.n[gm]
}

func (c *searchCounter) charge(gm netip.Addr) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.n == nil {
		c.n = make(map[netip.Addr]uint16, 1)
	}
	c.n[gm]++
}

func (c *searchCounter) clear(gm netip.Addr) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.n, gm)
}

// keepOnly drops grandmasters that are no longer configured, so a GM we stopped
// selecting cannot strand a count for the life of the daemon.
func (c *searchCounter) keepOnly(gms map[netip.Addr]*GM) {
	c.mu.Lock()
	defer c.mu.Unlock()
	maps.DeleteFunc(c.n, func(addr netip.Addr, _ uint16) bool {
		_, ok := gms[addr]
		return !ok
	})
}

// suspicious is a judgeable measurement that is too far off. A GM that answered
// with something unjudgeable is evidence of a good path, exactly as before.
func (g *GM) suspicious(threshold time.Duration) bool {
	return g.Answered && g.Judgeable && g.Offset.Abs() > threshold
}
