package server

import (
	"net"
)

// Stats is a metric collection interface
type Stats interface {
	// Start starts a stat reporter
	// Use this for passive reporters
	Start(int)

	// Report reports metrics
	// Use this for active reporters
	Report() error

	// SetPrefix sets custom metric prefix
	// For passive reporters this needs to be set before Start()
	SetPrefix(prefix string)

	// IncInvalidFormat atomically add 1 to the counter
	IncInvalidFormat()
	// IncRequests atomically add 1 to the counter
	IncRequests()
	// IncResponses atomically add 1 to the counter
	IncResponses()
	// IncListeners atomically add 1 to the counter
	IncListeners()
	// IncWorkers atomically add 1 to the counter
	IncWorkers()

	// DecListeners atomically removes 1 from the counter
	DecListeners()
	// DecWorkers atomically removes 1 from the counter
	DecWorkers()

	// SetAnnounce atomically sets counter to 1
	SetAnnounce()
	// ResetAnnounce atomically sets counter to 0
	ResetAnnounce()
}

// Announce is an announce interface
type Announce interface {
	// Do the announcement
	// Usually here advertise config is renewed
	// Unblocking. Run periodically
	Advertise([]net.IP) error
	// Stop announcement
	// Usually here advertise config is deleted
	Withdraw() error
}

// Checker is an internal healthcheck interface
type Checker interface {
	// Check is a method which performs basic validations that responder is alive
	Check() error

	// IncListeners atomically add 1 to the counter
	IncListeners()
	// IncWorkers atomically add 1 to the counter
	IncWorkers()

	// DecListeners atomically removes 1 from the counter
	DecListeners()
	// DecWorkers atomically removes 1 from the counter
	DecWorkers()
}
