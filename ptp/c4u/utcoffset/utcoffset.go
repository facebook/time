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

package utcoffset

import (
	"time"

	"github.com/facebook/time/leapsectz"
)

// Run the utcoffset calculation
func Run() (time.Duration, error) {
	// TAI <-> UTC offset was 10 seconds before introduction of leap seconds.
	// https://en.wikipedia.org/wiki/Leap_second
	var uo int32 = 10

	latestLeap, err := leapsectz.Latest("")
	if err != nil {
		return time.Duration(0), err
	}

	uo += latestLeap.Nleap

	return time.Duration(uo) * time.Second, nil
}
