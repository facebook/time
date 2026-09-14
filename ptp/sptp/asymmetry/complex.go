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

// Complex searches each GM's return path independently, then blames the selected
// GM once any other has been moved more than MaxPortChanges times.
type Complex struct{ Config Config }

// Name implements Corrector.
func (c *Complex) Name() string { return "complex" }

// Correct implements Corrector.
func (c *Complex) Correct(gms map[netip.Addr]*GM, best netip.Addr) int {
	c.correctOthers(gms, best)
	if c.selectedIsAsymmetric(gms) {
		c.correctSelected(gms, best)
	}
	n := 0
	for _, gm := range gms {
		if gm.Asymmetric {
			n++
		}
	}
	return n
}

func (c *Complex) correctOthers(gms map[netip.Addr]*GM, best netip.Addr) {
	for addr, gm := range gms {
		if !gm.Answered {
			// a GM that said nothing this tick keeps the streak it had earned
			continue
		}
		gm.Asymmetric = false
		if addr == best {
			continue
		}
		if !gm.suspicious(c.Config.Threshold) {
			gm.Streak = 0
			continue
		}
		gm.Asymmetric = true
		if gm.Streak > int(c.Config.MaxConsecutive) {
			gm.MovePort()
			gm.Streak = 0
			log.Infof("GM %v Asymmetric - new port offset: %d", addr, gm.PortOffset)
		} else {
			log.Debugf("GM %v asymmetric - grace %d/%d", addr, gm.Streak, c.Config.MaxConsecutive)
		}
		gm.Streak++
	}
}

// selectedIsAsymmetric reads exhausted search effort on any other GM as evidence
// that the shared leg, and so the selected GM, is the asymmetric one.
func (c *Complex) selectedIsAsymmetric(gms map[netip.Addr]*GM) bool {
	for _, gm := range gms {
		if gm.Asymmetric && gm.PortOffset > c.Config.MaxPortChanges {
			return true
		}
	}
	return false
}

func (c *Complex) correctSelected(gms map[netip.Addr]*GM, best netip.Addr) {
	for addr, gm := range gms {
		if addr == best {
			gm.MovePort()
			gm.Asymmetric = true
			log.Infof("Selected GM %s asymmetric - new port offset: %d", best, gm.PortOffset)
		} else {
			gm.ResetPort()
			gm.Asymmetric = false
		}
		gm.Streak = 0
	}
}
