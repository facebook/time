package announce

import (
	"net"
)

// NoopAnnounce is a noop implementation of Announce interface
// Use it if no IP advertisement is required
type NoopAnnounce struct{}

// Advertise is implementing Advertise interface. Doing nothing
func (n *NoopAnnounce) Advertise([]net.IP) error {
	return nil
}

// Withdraw is implementing Withdraw interface. Doing nothing
func (n *NoopAnnounce) Withdraw() error {
	return nil
}
