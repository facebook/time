package servo

import (
	"container/ring"
	"math"
	"time"
)

const (
	kpScale = 0.7
	kiScale = 0.3

	maxKpNormMax = 1.0
	maxKiNormMax = 2.0

	freqEstMargin = 0.001
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
	offsetStdev       int64
	offsetSigmaSq     int64
	offsetMean        int64
	freqStdev         float64
	freqSigmaSq       float64
	freqMean          float64
	skippedCount      int
	minOffsetFreqMean float64
	minOffsetStddev   int64
	samples           *ring.Ring
	samplesCount      int
	cfg               *PiServoFilterCfg
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

// Sample function to calculate frequency based on the offset
func (s *PiServo) Sample(offset int64, localTs uint64) (float64, ServoState) {
	var kiTerm, freqEstInterval, localDiff float64
	state := ServoInit
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
			state = ServoJump
		} else {
			state = ServoLocked
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
			state = ServoInit
			if s.filter != nil {
				s.filter.Reset()
			}
			break
		}
		if s.filter != nil && s.filter.IsSpike(offset, s.lastCorrectionTime) {
			ppb = s.filter.freqMean
			state = ServoFilter
			break
		}
		state = ServoLocked
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
	if state == ServoLocked && s.filter != nil {
		s.filter.Sample(&PiServoFilterSample{offset: offset, freq: ppb})
	}
	if state == ServoFilter {
		state = ServoLocked
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

// IsSpike is used to check whether supplied offset is spike or not
func (f *PiServoFilter) IsSpike(offset int64, lastCorrection time.Time) bool {
	if f.skippedCount >= f.cfg.maxSkipCount {
		f.Reset()
		return false
	}
	maxOffsetLocked := int64(f.cfg.offsetStdevFactor * float64(f.offsetStdev))
	// TODO: compensate sync delay wait time
	//maxOffsetLocked += (time.Now() - lastCorrection) * f.cfg.freqStdevFactor * f.freqStdev +

	if offset > max(maxOffsetLocked, f.cfg.minOffsetLocked) && f.skippedCount < f.cfg.maxSkipCount {
		f.skippedCount++
		return true
	}
	return false
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
