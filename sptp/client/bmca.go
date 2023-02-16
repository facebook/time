package client

import (
	ptp "github.com/facebook/time/ptp/protocol"

	"github.com/facebook/time/sptp/bmc"
)

// FIXME: use priority3
func bmca(msgs []*ptp.Announce, prios map[ptp.ClockIdentity]int) *ptp.Announce {
	if len(msgs) == 0 {
		return nil
	}
	best := msgs[0]
	for _, msg := range msgs[1:] {
		a := best
		b := msg
		localPrioA := prios[a.AnnounceBody.GrandmasterIdentity]
		localPrioB := prios[b.AnnounceBody.GrandmasterIdentity]
		if bmc.TelcoDscmp(a, b, localPrioA, localPrioB) < 0 {
			best = b
		}
	}
	return best
}
