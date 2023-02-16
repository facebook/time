package servo

// Servo structure has values common for any type of servo
type Servo struct {
	maxFreq            float64
	StepThreshold      int64
	FirstStepThreshold int64
	FirstUpdate        bool
	OffsetThreshold    int64
	numOffsetValues    int
	currOffsetValues   int
}

// ServoState provides the result of servo calculation
type ServoState uint8

// All the states of servo
const (
	ServoInit   ServoState = 0
	ServoJump   ServoState = 1
	ServoLocked ServoState = 2
	ServoFilter ServoState = 3
)

func (s ServoState) String() string {
	switch s {
	case ServoInit:
		return "INIT"
	case ServoJump:
		return "JUMP"
	case ServoLocked:
		return "LOCKED"
	case ServoFilter:
		return "FILTER"
	}
	return "UNSUPPORTED"
}

// DefaultServoConfig generates default servo struct
func DefaultServoConfig() Servo {
	return Servo{
		maxFreq:            900000000,
		StepThreshold:      0,
		FirstStepThreshold: 20000,
		FirstUpdate:        false,
		OffsetThreshold:    0,
		numOffsetValues:    0,
		currOffsetValues:   0,
	}
}
