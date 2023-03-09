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

package servo

import (
	"container/ring"
	"math"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	kpScale = 0.7
	kiScale = 0.3

	maxKpNormMax = 1.0
	maxKiNormMax = 2.0

	freqEstMargin = 0.001
)

type filterState uint8

const (
	filterNoSpike filterState = iota
	filterSpike
	filterReset
)

// PiServoCfg is an integral servo config
type PiServoCfg struct {
	PiKp         float64
	PiKi         float64
	PiKpScale    float64
	PiKpExponent float64
	PiKpNormMax  float64
	PiKiScale    float64
	PiKiExponent float64
	PiKiNormMax  float64
}

// PiServoFilterCfg is a filter configuration
type PiServoFilterCfg struct {
	minOffsetLocked   int64   // The minimum offset value to treat servo is locked
	maxFreqChange     int64   // The amount of ppb the oscillator can drift per 1s
	maxSkipCount      int     // The amount of samples to skip via filter
	maxOffsetInit     int64   // The initial value above which sample is treated as outlier
	offsetStdevFactor float64 // Standard deviation factor for offset stddev calculations
	freqStdevFactor   float64 // Standard deviation factor for frequency stddev calculations
	ringSize          int     // The amount of samples we have to collect to activate filter
}

// PiServoFilterSample is a structure of offset and frequency
type PiServoFilterSample struct {
	offset int64
	freq   float64
}

// PiServoFilter is a filter state structure
type PiServoFilter struct {
	offsetStdev   int64
	offsetSigmaSq int64
	offsetMean    int64
	freqStdev     float64
	freqSigmaSq   float64
	freqMean      float64
	skippedCount  int
	samples       *ring.Ring
	samplesCount  int
	cfg           *PiServoFilterCfg
}

// PiServo is an integral servo
type PiServo struct {
	Servo
	offset             [2]int64
	local              [2]uint64
	drift              float64
	kp                 float64
	ki                 float64
	lastFreq           float64
	count              int
	lastCorrectionTime time.Time
	filter             *PiServoFilter
	/* configuration: */
	cfg *PiServoCfg
}

func max(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// SetLastFreq function to reset last freq
func (s *PiServo) SetLastFreq(freq float64) {
	s.lastFreq = freq
	s.drift = freq
}

// SetMaxFreq is to adjust frequency range supported by PHC
func (s *PiServo) SetMaxFreq(freq float64) {
	s.maxFreq = freq
}

func (s *PiServo) isSpike(offset int64, lastCorrection time.Time) filterState {
	if s.filter == nil {
		return filterNoSpike
	}
	return s.filter.isSpike(offset, lastCorrection)
}

// Sample function to calculate frequency based on the offset
func (s *PiServo) Sample(offset int64, localTs uint64) (float64, State) {
	var kiTerm, freqEstInterval, localDiff float64
	state := StateInit
	ppb := s.lastFreq
	sOffset := offset
	if sOffset < 0 {
		sOffset = -sOffset
	}

	switch s.count {
	case 0:
		s.offset[0] = offset
		s.local[0] = localTs
		s.count = 1
	case 1:
		s.offset[1] = offset
		s.local[1] = localTs

		if s.local[0] >= s.local[1] {
			s.count = 0
			break
		}

		localDiff = (float64)(s.local[1]-s.local[0]) / math.Pow10(9)
		localDiff += localDiff * freqEstMargin
		freqEstInterval = 0.016 / s.ki
		if freqEstInterval > 1000.0 {
			freqEstInterval = 1000.0
		}
		if localDiff < freqEstInterval {
			log.Warningf("servo Sample is called too often, not enough time passed since first sample")
			break
		}

		/* Adjust drift by the measured frequency offset. */
		s.drift += (math.Pow10(9) - s.drift) * float64(s.offset[1]-s.offset[0]) /
			float64(s.local[1]-s.local[0])

		if s.drift < -s.maxFreq {
			s.drift = -s.maxFreq
		} else if s.drift > s.maxFreq {
			s.drift = s.maxFreq
		}

		if (s.FirstUpdate && s.FirstStepThreshold > 0 &&
			s.FirstStepThreshold < sOffset) ||
			(s.StepThreshold > 0 && s.StepThreshold < sOffset) {
			state = StateJump
		} else {
			state = StateLocked
		}
		ppb = s.drift
		s.count = 2
	case 2:
		/*
		 * reset the clock servo when offset is greater than the max
		 * offset value. Note that the clock jump will be performed in
		 * step 1, so it is not necessary to have clock jump
		 * immediately. This allows re-calculating drift as in initial
		 * clock startup.
		 */
		if s.StepThreshold != 0 &&
			s.StepThreshold < sOffset {
			s.count = 0
			state = StateInit
			if s.filter != nil {
				s.filter.Reset()
			}
			break
		}
		fState := s.isSpike(offset, s.lastCorrectionTime)
		if fState == filterSpike {
			ppb = s.MeanFreq()
			state = StateFilter
			s.filter.skippedCount++ // it's safe because fState can only be filterNoSpike without filter
			log.Warningf("servo filtered out offset %d", offset)
			break
		}
		// if there were too many outstanding offsets, reset the filter and the servo
		if fState == filterReset {
			s.count = 0
			s.drift = 0
			s.filter.Reset() // it's safe because fState can only be filterNoSpike without filter
			state = StateInit
			log.Warning("servo was reset")
			break
		}
		state = StateLocked
		kiTerm = s.ki * float64(offset)
		ppb = s.kp*float64(offset) + s.drift + kiTerm
		if ppb < -s.maxFreq {
			ppb = -s.maxFreq
		} else if ppb > s.maxFreq {
			ppb = s.maxFreq
		} else {
			s.drift += kiTerm
		}
	}
	s.lastFreq = ppb
	if state == StateLocked && s.filter != nil {
		s.filter.Sample(&PiServoFilterSample{offset: offset, freq: ppb})
		s.filter.skippedCount = 0
		s.lastCorrectionTime = time.Now()
	}
	if state == StateFilter {
		state = StateLocked
	}
	return ppb, state
}

// SyncInterval inform a clock servo about the master's sync interval in seconds
func (s *PiServo) SyncInterval(interval float64) {
	s.kp = s.cfg.PiKpScale * math.Pow(interval, s.cfg.PiKpExponent)
	if s.kp > s.cfg.PiKpNormMax/interval {
		s.kp = s.cfg.PiKpNormMax / interval
	}

	s.ki = s.cfg.PiKiScale * math.Pow(interval, s.cfg.PiKiExponent)
	if s.ki > s.cfg.PiKiNormMax/interval {
		s.ki = s.cfg.PiKiNormMax / interval
	}
}

// isSpike is used to check whether supplied offset is spike or not
func (f *PiServoFilter) isSpike(offset int64, lastCorrection time.Time) filterState {
	if f.skippedCount >= f.cfg.maxSkipCount {
		return filterReset
	}
	maxOffsetLocked := int64(f.cfg.offsetStdevFactor * float64(f.offsetStdev))
	secPassed := math.Round(time.Since(lastCorrection).Seconds())
	waitFactor := secPassed * (f.cfg.freqStdevFactor*f.freqStdev + float64(f.cfg.maxFreqChange/2))

	maxOffsetLocked += int64(waitFactor)

	if offset > max(maxOffsetLocked, f.cfg.minOffsetLocked) && f.skippedCount < f.cfg.maxSkipCount {
		return filterSpike
	}
	return filterNoSpike
}

// Sample to add a sample to filter and recalculate value
func (f *PiServoFilter) Sample(s *PiServoFilterSample) {
	f.samples.Value = s
	f.samples = f.samples.Next()
	if f.samplesCount != f.cfg.ringSize {
		f.samplesCount++
	}
	var offsetSigmaSq, offsetMean int64
	var freqSigmaSq, freqMean float64
	f.samples.Do(func(val any) {
		if val == nil {
			return
		}
		v := val.(*PiServoFilterSample)
		offsetSigmaSq += v.offset * v.offset
		offsetMean += v.offset
		freqSigmaSq += v.freq * v.freq
		freqMean += v.freq
	})
	f.offsetMean = offsetMean / int64(f.samplesCount)
	f.offsetStdev = int64(math.Sqrt(float64(offsetSigmaSq) / float64(f.samplesCount)))

	f.freqMean = freqMean / float64(f.samplesCount)
	f.freqStdev = math.Sqrt(freqSigmaSq / float64(f.samplesCount))
}

// Reset - cleanup and restart filter
func (f *PiServoFilter) Reset() {
	f.samples = ring.New(f.cfg.ringSize)
	f.offsetStdev = 0
	f.offsetSigmaSq = 0
	f.offsetMean = 0
	f.freqStdev = 0.0
	f.freqSigmaSq = 0.0
	f.freqMean = 0.0
	f.skippedCount = 0
	f.samplesCount = 0
}

// MeanFreq to return best calculated frequency
func (f *PiServoFilter) MeanFreq() float64 {
	return f.freqMean
}

// MeanFreq to return best calculated frequency from filter
func (s *PiServo) MeanFreq() float64 {
	if s.filter != nil {
		return s.filter.MeanFreq()
	}
	return s.lastFreq
}

// NewPiServo to create servo structure
func NewPiServo(s Servo, cfg *PiServoCfg, freq float64) *PiServo {
	var pi PiServo

	pi.Servo = s
	pi.cfg = cfg
	pi.lastFreq = freq
	pi.drift = freq

	return &pi
}

// NewPiServoFilter to create new filter instance
func NewPiServoFilter(s *PiServo, cfg *PiServoFilterCfg) *PiServoFilter {
	filter := &PiServoFilter{
		cfg: cfg,
	}
	filter.Reset()
	s.filter = filter
	return filter
}

// DefaultPiServoCfg to create default pi servo config
func DefaultPiServoCfg() *PiServoCfg {
	return &PiServoCfg{
		PiKp:         0.0,
		PiKi:         0.0,
		PiKpScale:    kpScale,
		PiKpExponent: 0.0,
		PiKpNormMax:  maxKpNormMax,
		PiKiScale:    kiScale,
		PiKiExponent: 0.0,
		PiKiNormMax:  maxKiNormMax,
	}
}

// DefaultPiServoFilterCfg to create a default pi servo filter config
func DefaultPiServoFilterCfg() *PiServoFilterCfg {
	return &PiServoFilterCfg{
		minOffsetLocked:   15000,
		maxFreqChange:     40,
		maxSkipCount:      15,
		maxOffsetInit:     500000,
		offsetStdevFactor: 3.0,
		freqStdevFactor:   3.0,
		ringSize:          30,
	}
}
