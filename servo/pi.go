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
	// kp and ki scale for high offset range - more aggressive servo
	kpScale = 0.7
	kiScale = 0.3

	// kp and ki scale for low offset range - less aggressive servo
	kpScaleLow = 0.07
	kiScaleLow = 0.03

	maxKpNormMax = 1.0
	maxKiNormMax = 2.0

	freqEstMargin = 0.001

	defaultOffsetRange = 100
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
	offsetRange       int64   // The range of values within which we consider the sample as valid
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
	offsetStdev        int64
	offsetSigmaSq      int64
	offsetMean         int64
	lastOffset         int64
	freqStdev          float64
	freqSigmaSq        float64
	freqMean           float64
	skippedCount       int
	offsetSamples      *ring.Ring
	offsetSamplesCount int
	freqSamples        *ring.Ring
	freqSamplesCount   int
	cfg                *PiServoFilterCfg
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
	syncInterval       float64
	count              int
	lastCorrectionTime time.Time
	filter             *PiServoFilter
	/* configuration: */
	cfg *PiServoCfg
}

// SetLastFreq function to set last freq
func (s *PiServo) SetLastFreq(freq float64) {
	s.lastFreq = freq
}

// InitLastFreq function to reset last freq and drift
func (s *PiServo) InitLastFreq(freq float64) {
	s.lastFreq = freq
	s.drift = freq
}

// SetMaxFreq is to adjust frequency range supported by PHC
func (s *PiServo) SetMaxFreq(freq float64) {
	s.maxFreq = freq
}

// UnsetFirstUpdate function to unset first update flag
func (s *PiServo) UnsetFirstUpdate() {
	s.FirstUpdate = false
}

// GetMaxFreq gets current configured max frequency
func (s *PiServo) GetMaxFreq() float64 {
	return s.maxFreq
}

// IsStable is used to check if the offset measurement is stable or not
func (s *PiServo) IsStable(offset int64) bool {
	if s.filter != nil {
		return s.filter.IsStable(offset)
	}
	return inRange(offset, -defaultOffsetRange, defaultOffsetRange)
}

// IsSpike function to check if offset is considered as spike
func (s *PiServo) IsSpike(offset int64) bool {
	if s.filter == nil || s.count < 2 {
		return false
	}
	fState := s.filter.isSpike(offset, s.lastCorrectionTime)
	if fState == filterSpike {
		s.lastFreq = s.filter.freqMean
		s.filter.skippedCount++ // it's safe because fState can only be filterNoSpike without filter
		return true
	}
	// if there were too many outstanding offsets, reset the filter and the servo
	if fState == filterReset {
		s.lastFreq = s.filter.freqMean
		s.count = 0
		s.drift = 0
		s.filter.Reset() // it's safe because fState can only be filterNoSpike without filter
		s.cfg.makePiFast()
		s.resyncInterval()
		log.Warning("servo was reset")
		return true
	}
	return false
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
			log.Warning("servo Sample is called too often, not enough time passed since first sample")
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

	return ppb, state
}

func (s *PiServo) resyncInterval() {
	if s.syncInterval == 0 {
		return
	}
	s.kp = s.cfg.PiKpScale * math.Pow(s.syncInterval, s.cfg.PiKpExponent)
	if s.kp > s.cfg.PiKpNormMax/s.syncInterval {
		s.kp = s.cfg.PiKpNormMax / s.syncInterval
	}

	s.ki = s.cfg.PiKiScale * math.Pow(s.syncInterval, s.cfg.PiKiExponent)
	if s.ki > s.cfg.PiKiNormMax/s.syncInterval {
		s.ki = s.cfg.PiKiNormMax / s.syncInterval
	}
}

// SyncInterval inform a clock servo about the master's sync interval in seconds
func (s *PiServo) SyncInterval(interval float64) {
	s.syncInterval = interval
	s.resyncInterval()
}

// GetState returns current state of PiServo
func (s *PiServo) GetState() State {
	switch s.count {
	case 0:
		return StateInit
	case 1:
		return StateJump
	default:
		return StateLocked
	}
}

// IsStable is used to check if the offset measurement is stable or not
func (f *PiServoFilter) IsStable(offset int64) bool {
	return inRange(f.lastOffset, -f.cfg.offsetRange, f.cfg.offsetRange) && inRange(offset, -f.cfg.offsetRange, f.cfg.offsetRange)
}

// isSpike is used to check whether supplied offset is spike or not
func (f *PiServoFilter) isSpike(offset int64, lastCorrection time.Time) filterState {
	if f.skippedCount >= f.cfg.maxSkipCount {
		return filterReset
	}
	if f.offsetSamplesCount != f.cfg.ringSize {
		return filterNoSpike
	}
	maxOffsetLocked := int64(f.cfg.offsetStdevFactor * float64(f.offsetStdev))
	secPassed := math.Round(time.Since(lastCorrection).Seconds())
	waitFactor := secPassed * (f.cfg.freqStdevFactor*f.freqStdev + float64(f.cfg.maxFreqChange/2))

	maxOffsetLocked += int64(waitFactor)

	log.Debugf("Filter.isSpike: offset stdev %d, wait factor %0.3f, max offset locked %d", f.offsetStdev, waitFactor, maxOffsetLocked)
	// offset can be negative, we have to check absolute value
	if offset < 0 {
		offset *= -1
	}
	if offset > max(maxOffsetLocked, f.cfg.minOffsetLocked) && f.skippedCount < f.cfg.maxSkipCount {
		return filterSpike
	}
	return filterNoSpike
}

func inRange(value, minimum, maximum int64) bool {
	if value >= minimum && value <= maximum {
		return true
	}
	return false
}

// Sample to add a sample to filter and recalculate value
func (f *PiServoFilter) Sample(s *PiServoFilterSample) {
	if f.offsetSamples.Value != nil {
		v := f.offsetSamples.Value.(*PiServoFilterSample)
		f.offsetMean -= v.offset / int64(f.offsetSamplesCount)
	}
	f.offsetSamples.Value = s
	f.offsetSamples = f.offsetSamples.Next()
	if f.offsetSamplesCount != f.cfg.ringSize {
		f.offsetSamplesCount++
		f.offsetMean = -1 * (s.offset / int64(f.offsetSamplesCount))
		f.offsetSamples.Do(func(val any) {
			if val == nil {
				return
			}
			v := val.(*PiServoFilterSample)
			f.offsetMean += v.offset / int64(f.offsetSamplesCount)
		})
	}
	f.offsetMean += s.offset / int64(f.offsetSamplesCount)
	var offsetSigmaSq int64
	f.offsetSamples.Do(func(val any) {
		if val == nil {
			return
		}
		v := val.(*PiServoFilterSample)
		offsetSigmaSq += (v.offset - f.offsetMean) * (v.offset - f.offsetMean)
	})
	f.offsetStdev = int64(math.Sqrt(float64(offsetSigmaSq) / float64(f.offsetSamplesCount)))
	f.lastOffset = s.offset

	/*
	 * Mean frequency is heavily affected by the values used to compensate for offsets in case of
	 * recovering after holdover state. If we have to go to holdover again while recovering from
	 * previous holdover, we may apply bad frequency which will cause PHC going off pretty fast.
	 * Let's calculate mean frequency only when we are sure that PHC is running more or less stable.
	 */
	if f.IsStable(s.offset) {
		var freqSigmaSq float64
		if f.freqSamples.Value != nil {
			// this means that ring buffer is fully filled
			v := f.freqSamples.Value.(*PiServoFilterSample)
			f.freqMean -= v.freq / float64(f.freqSamplesCount)
			f.freqSamples.Value = s
			f.freqSamples = f.freqSamples.Next()
			f.freqMean += s.freq / float64(f.freqSamplesCount)
		} else {
			// the ring wasn't full yet
			f.freqSamples.Value = s
			f.freqSamples = f.freqSamples.Next()
			f.freqSamplesCount++
			if f.freqSamples.Value != nil {
				// we have to calculate mean frequency here for the first time
				f.freqMean = float64(0)
				f.freqSamples.Do(func(val any) {
					if val == nil {
						return
					}
					v := val.(*PiServoFilterSample)
					f.freqMean += v.freq / float64(f.freqSamplesCount)
				})
			}
		}
		f.freqSamples.Do(func(val any) {
			if val == nil {
				return
			}
			v := val.(*PiServoFilterSample)
			freqSigmaSq += (v.freq - f.freqMean) * (v.freq - f.freqMean)
		})
		f.freqStdev = math.Sqrt(freqSigmaSq / float64(f.offsetSamplesCount))
		log.Debugf("Filter.Sample: freq stdev %f, meanFreq = %f", f.freqStdev, f.freqMean)
	}
}

// Unlock resets and unlocks the servo
func (s *PiServo) Unlock() {
	s.count = 0
	s.cfg.makePiFast()
	s.resyncInterval()
	s.filter.Reset()
}

// Reset - cleanup and restart filter
func (f *PiServoFilter) Reset() {
	f.offsetSamples = ring.New(f.cfg.ringSize)
	f.freqSamples = ring.New(f.cfg.ringSize)
	f.offsetStdev = 0
	f.offsetSigmaSq = 0
	f.offsetMean = 0
	f.freqStdev = 0.0
	f.freqSigmaSq = 0.0
	f.skippedCount = 0
	f.offsetSamplesCount = 0
	f.freqSamplesCount = 0
	// Does not reset freqMean (mean frequency). It's either good enough from previous iteration
	// or it's last used mean frequency read during restart
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
	filter.freqMean = s.lastFreq
	s.filter = filter
	return filter
}

func (cfg *PiServoCfg) makePiFast() {
	cfg.PiKpScale = kpScale
	cfg.PiKiScale = kiScale
}

func (cfg *PiServoCfg) makePiSlow() {
	cfg.PiKpScale = kpScaleLow
	cfg.PiKiScale = kiScaleLow
}

// DefaultPiServoCfg to create default pi servo config
func DefaultPiServoCfg() *PiServoCfg {
	cfg := PiServoCfg{
		PiKp:         0.0,
		PiKi:         0.0,
		PiKpExponent: 0.0,
		PiKpNormMax:  maxKpNormMax,
		PiKiExponent: 0.0,
		PiKiNormMax:  maxKiNormMax,
	}
	cfg.makePiFast()
	return &cfg
}

// DefaultPiServoFilterCfg to create a default pi servo filter config
func DefaultPiServoFilterCfg() *PiServoFilterCfg {
	return &PiServoFilterCfg{
		minOffsetLocked:   15000,
		maxFreqChange:     40,
		maxSkipCount:      15,
		maxOffsetInit:     500000,
		offsetRange:       defaultOffsetRange, // the range of the offset values that are considered "normal"
		offsetStdevFactor: 3.0,
		freqStdevFactor:   3.0,
		ringSize:          30,
	}
}
