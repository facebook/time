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
	"os"
	"time"

	"github.com/facebook/time/phc"
)

const (
	phcTimeCardPath = "/dev/ptp_tcard"
	phcNICPath      = "/dev/ptp0"
)

func ts2phc() (time.Duration, error) {
	a, err := os.Open(phcTimeCardPath)
	if err != nil {
		return 0, err
	}
	defer a.Close()

	b, err := os.Open(phcNICPath)
	if err != nil {
		return 0, err
	}
	defer b.Close()

	return phc.OffsetBetweenDevices(a, b)
}
