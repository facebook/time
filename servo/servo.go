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

// State provides the result of servo calculation
type State uint8

// All the states of servo
const (
	StateInit   State = 0
	StateJump   State = 1
	StateLocked State = 2
	StateFilter State = 3
)

func (s State) String() string {
	switch s {
	case StateInit:
		return "INIT"
	case StateJump:
		return "JUMP"
	case StateLocked:
		return "LOCKED"
	case StateFilter:
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
