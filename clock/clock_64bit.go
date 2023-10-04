//go:build !386

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

package clock

import (
	"time"

	"golang.org/x/sys/unix"
)

func setFreq(tx *unix.Timex, freqPPB float64) {
	// man(2) clock_adjtime, turn ppb to ppm
	tx.Freq = int64(freqPPB * PPBToTimexPPM)
}

func setTime(tx *unix.Timex, sec, usec time.Duration) {
	tx.Time.Sec = int64(sec)
	tx.Time.Usec = int64(usec)
}
