package drain

import (
	"os"
	"time"

	"github.com/facebook/time/ptp/ptp4u/server"
	log "github.com/sirupsen/logrus"
)

const (
	looptime   = 30 * time.Second
	killswitch = "/var/tmp/kill_ptp4u"
)

// FileDrain drains the server if the file exists
type FileDrain struct {
	Time time.Duration
	File string
}

// NewFileDraim returns a new FileDrain
func NewFileDrain() *FileDrain {
	return &FileDrain{
		Time: looptime,
		File: killswitch,
	}
}

// Start drain check
func (f *FileDrain) Start(s *server.Server) {
	for {
		if _, err := os.Stat(f.File); err == nil {
			s.Drain()
			log.Warningf("killswitch engaged shifting traffic")
		} else {
			s.Undrain()
		}

		time.Sleep(f.Time)
	}
}
