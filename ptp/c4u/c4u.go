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

package c4u

import (
	"time"

	"github.com/coreos/go-systemd/daemon"
	"github.com/facebook/time/ptp/c4u/clock"
	"github.com/facebook/time/ptp/c4u/stats"
	"github.com/facebook/time/ptp/c4u/utcoffset"
	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/ptp4u/server"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// SdNotify notifies systemd about service successful start
func SdNotify() error {
	// daemon.SdNotify returns one of the following:
	// (false, nil) - notification not supported (i.e. NOTIFY_SOCKET is unset)
	// (false, err) - notification supported, but failure happened (e.g. error connecting to NOTIFY_SOCKET or while sending data)
	// (true, nil) - notification supported, data has been sent
	supported, err := daemon.SdNotify(false, daemon.SdNotifyReady)
	if !supported && err != nil {
		return err
	} else if !supported {
		log.Warning("sd_notify not supported")
	} else {
		log.Info("successfully sent sd_notify event")
	}
	return nil
}

// Config is a struct representing the config of the c4u
type Config struct {
	Apply               bool
	Path                string
	Pid                 string
	Sample              int
	AccuracyExpr        string
	ClassExpr           string
	LockBaseLine        ptp.ClockAccuracy
	CalibratingBaseLine ptp.ClockAccuracy
	HoldoverBaseLine    ptp.ClockAccuracy
}

var defaultConfig = &server.DynamicConfig{
	DrainInterval:  30 * time.Second,
	MaxSubDuration: 1 * time.Hour,
	MetricInterval: 1 * time.Minute,
	MinSubInterval: 1 * time.Second,
}

func evaluateClockQuality(config *Config, q *ptp.ClockQuality) *ptp.ClockQuality {
	w := q

	// If all data in ring buffer is nil we have to give up and pronounce
	// clock as uncalibrated with unknown accuracy
	if w == nil {
		w = &ptp.ClockQuality{
			ClockClass:    clock.ClockClassUncalibrated,
			ClockAccuracy: ptp.ClockAccuracyUnknown,
		}
	}

	// Some override logic to represent the situation better:
	// * In locked/calibrating states we may fallback to nearly 0ns. Keep the baseline or worse
	// * In holdover we don't know the offset. Let's fallback to Baseline or worse
	// * In uncalibrated state we don't know the offset - let's say it
	if w.ClockClass == clock.ClockClassLock && w.ClockAccuracy < config.LockBaseLine {
		w.ClockAccuracy = config.LockBaseLine
	} else if w.ClockClass == clock.ClockClassHoldover && w.ClockAccuracy < config.HoldoverBaseLine {
		w.ClockAccuracy = config.HoldoverBaseLine
	} else if w.ClockClass == clock.ClockClassCalibrating && w.ClockAccuracy < config.CalibratingBaseLine {
		w.ClockAccuracy = config.CalibratingBaseLine
	} else if w.ClockClass == clock.ClockClassUncalibrated {
		w.ClockAccuracy = ptp.ClockAccuracyUnknown
	}

	return w
}

// Run config generation once
func Run(config *Config, rb *clock.RingBuffer, st stats.Stats) error {
	defer st.Snapshot()
	dataError := false
	dp, err := clock.Run()
	if err != nil {
		log.Warningf("Failed to collect clock data: %v", err)
		dataError = true
	}

	// If DP is missing - assume the worst
	if dp == nil {
		dp = &clock.DataPoint{
			OscillatorClockClass: clock.ClockClassUncalibrated,
		}
	}

	rb.Write(dp)
	// stats
	st.SetPHCOffsetNS(int64(dp.PHCOffset))
	st.SetOscillatorOffsetNS(int64(dp.OscillatorOffset))

	w, err := clock.Worst(rb.Data(), config.AccuracyExpr, config.ClassExpr)
	if err != nil {
		return err
	}
	st.SetClockAccuracyWorst(int64(w.ClockAccuracy))

	// Evaluate and override if needed
	q := evaluateClockQuality(config, w)

	// UTC data
	u, err := utcoffset.Run()
	if err != nil {
		log.Errorf("Failed to collect UTC offset data: %v", err)
		dataError = true
		// Keep going. UTC offset will be 0.
		// Clock data needs to be updated anyway as higher priority
	}

	if dataError {
		st.IncDataError()
	} else {
		st.ResetDataError()
	}

	current, err := server.ReadDynamicConfig(config.Path)
	if err != nil {
		log.Warningf("Failed read current ptp4u config: %v. Using defaults", err)
		current = defaultConfig
	}
	pending := &server.DynamicConfig{}
	*pending = *current

	pending.ClockClass = q.ClockClass
	pending.ClockAccuracy = q.ClockAccuracy
	pending.UTCOffset = u

	st.SetClockClass(int64(pending.ClockClass))
	st.SetClockAccuracy(int64(pending.ClockAccuracy))
	st.SetUTCOffsetSec(int64(pending.UTCOffset.Seconds()))

	if *current != *pending {
		log.Infof("Current: %+v", current)
		log.Infof("Pending: %+v", pending)

		if config.Apply {
			log.Infof("Saving a pending config to %s", config.Path)
			err := pending.Write(config.Path)
			if err != nil {
				log.Errorf("Failed save the ptp4u config: %v", err)
				return nil
			}

			pid, err := server.ReadPidFile(config.Pid)
			if err != nil {
				log.Warningf("Failed to read ptp4u pid: %v", err)
				return nil
			}

			err = unix.Kill(pid, unix.SIGHUP)
			if err != nil {
				log.Warningf("Failed to send SIGHUP to ptp4u %d: %v", pid, err)
				return nil
			}
			log.Infof("SIGHUP is sent to ptp4u pid: %d", pid)
			st.IncReload()
		}
	} else {
		// set reload to 0
		st.ResetReload()
	}

	return nil
}
