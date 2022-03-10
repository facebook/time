package drain

import (
	"github.com/facebook/time/ptp/ptp4u/server"
)

// Drain is a drain check interface
type Drain interface {
	// Start starts the drain check
	Start(s *server.Server)
}
