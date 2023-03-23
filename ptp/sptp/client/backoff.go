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

package client

import (
	"errors"
	"math"
)

var errBackoff = errors.New("backoff for faulty GM")

const (
	backoffNone        = ""
	backoffFixed       = "fixed"
	backoffLinear      = "linear"
	backoffExponential = "exponential"
)

type backoff struct {
	cfg BackoffConfig
	// state
	counter int
	value   int
}

func (b *backoff) active() bool {
	return b.value != 0
}

func (b *backoff) reset() {
	b.value = 0
	b.counter = 0
}

func (b *backoff) tick() int {
	b.value--
	if b.value < 0 {
		b.value = 0
	}
	return b.value
}

func (b *backoff) bump() int {
	b.counter++
	switch b.cfg.Mode {
	case backoffFixed:
		b.value = b.cfg.Step
	case backoffLinear:
		b.value = b.cfg.Step * b.counter
	case backoffExponential:
		b.value = int(math.Pow(float64(b.cfg.Step), float64(b.counter)))
	default:
		// do nothing, never active
		b.counter = 0
		b.value = 0
	}
	if b.value > b.cfg.MaxValue {
		b.value = b.cfg.MaxValue
	}
	return b.value
}

func newBackoff(cfg BackoffConfig) *backoff {
	return &backoff{cfg: cfg}
}
