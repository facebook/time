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

	log "github.com/sirupsen/logrus"
)

// Simple moves the selected GM's port only when every other GM agrees something
// is wrong. One GM looking bad says something about that GM; all of them looking
// bad says something about us.
type Simple struct{ Config Config }

// Name implements Corrector.
func (s *Simple) Name() string { return "simple" }

// Correct implements Corrector.
func (s *Simple) Correct(gms map[netip.Addr]*GM, best netip.Addr) int {
	selected := gms[best]
	if selected == nil {
		log.Errorf("selected GM %v is not in the GM list", best)
		return 0
	}
	if !s.selectedIsAsymmetric(gms, best, selected) {
		return 0
	}
	selected.Streak = 0
	selected.MovePort()
	log.Infof("Selected GM %s asymmetric - new port offset: %d", best, selected.PortOffset)
	return 1
}

func (s *Simple) selectedIsAsymmetric(gms map[netip.Addr]*GM, best netip.Addr, selected *GM) bool {
	var asymmetric, silent, others int
	for addr, gm := range gms {
		if addr == best {
			continue
		}
		others++
		// reset on every pass, in case the selected GM changed
		gm.Streak = 0
		if !gm.Answered {
			// silence is not evidence of a good path; an unjudgeable answer is
			asymmetric++
			silent++
			continue
		}
		if gm.suspicious(s.Config.Threshold) {
			asymmetric++
		}
	}
	// with only the selected GM answering there is nothing to corroborate against
	if others == 0 || silent == others {
		return false
	}
	if asymmetric != others {
		selected.Streak = max(selected.Streak-1, 0)
		return false
	}
	if selected.Streak <= int(s.Config.MaxConsecutive) {
		selected.Streak++
		return false
	}
	return true
}
