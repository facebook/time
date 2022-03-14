package drain

// Drain is a drain check interface
type Drain interface {
	// Check returns true if the service needs to be drained
	Check() bool
}
