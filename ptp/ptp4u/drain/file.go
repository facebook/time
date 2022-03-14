package drain

import (
	"os"
)

// FileDrain implements the check interface
type FileDrain struct {
	FileName string
}

// Check checks the existance of a file
func (f *FileDrain) Check() bool {
	if _, err := os.Stat(f.FileName); err == nil {
		return true
	}

	return false
}
