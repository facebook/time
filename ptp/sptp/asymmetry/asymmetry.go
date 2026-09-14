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
	"net/netip"
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
	// PortMoved is whether this correction asked the port to advance, and
	// PortReset whether it asked for the offset to be cleared. The caller applies
	// them, so the decision stays separate from the mutation.
	PortMoved bool
	PortReset bool

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

// ResetPort records that this GM's response port should return to zero. It
// cancels a move recorded earlier in the same round, which the complex path does
// when it stops searching a GM and blames the selected one instead.
func (g *GM) ResetPort() {
	g.PortReset = true
	g.PortMoved = false
	g.PortOffset = 0
}

// Config tunes the corrections.
type Config struct {
	// Threshold above which an offset is suspicious.
	Threshold time.Duration
	// MaxConsecutive measurements a GM may look asymmetric before its port moves.
	MaxConsecutive uint16
	// MaxPortChanges a GM may accumulate before the selected GM is blamed instead.
	MaxPortChanges uint16
}

// Corrector decides whether any response port should move. Name identifies the
// implementation in stats, so an experiment can attribute a host to its arm.
type Corrector interface {
	// Correct returns how many GMs had their port moved.
	Correct(gms map[netip.Addr]*GM, best netip.Addr) int
	Name() string
}

// suspicious is a judgeable measurement that is too far off. A GM that answered
// with something unjudgeable is evidence of a good path, exactly as before.
func (g *GM) suspicious(threshold time.Duration) bool {
	return g.Answered && g.Judgeable && g.Offset.Abs() > threshold
}
