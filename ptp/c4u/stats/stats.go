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

/*
Package stats implements statistics collection and reporting.
It is used by server to report internal statistics, such as number of
requests and responses.
*/
package stats

// Stats is a metric collection interface
type Stats interface {
	// Start starts a stat reporter
	// Use this for passive reporters
	Start(monitoringport int)

	// Snapshot the values so they can be reported atomically
	Snapshot()

	// IncReload atomically add 1 to the counter
	IncReload()

	// ResetReload atomically sets counter to 0
	ResetReload()

	// IncDataError atomically add 1 to the counter
	IncDataError()

	// ResetDataError atomically sets counter to 0
	ResetDataError()

	// SetUTCOffsetSec atomically sets the utcOffsetSec
	SetUTCOffsetSec(utcOffsetSec int64)

	// SetUTCOffsetNS atomically sets the phcOffsetNS
	SetPHCOffsetNS(phcOffsetNS int64)

	// SetOscillatorOffsetNS atomically sets the oscillatorOffsetNS
	SetOscillatorOffsetNS(oscillatorOffsetNS int64)

	// SetClockAccuracy atomically sets the clock accuracy
	SetClockAccuracy(clockAccuracy int64)

	// SetClockClass atomically sets the clock class
	SetClockClass(clockClass int64)
}

type counters struct {
	utcOffsetSec       int64
	phcOffsetNS        int64
	oscillatorOffsetNS int64
	clockAccuracy      int64
	clockClass         int64
	reload             int64
	dataError          int64
}

// toMap converts counters to a map
func (c *counters) toMap() (export map[string]int64) {
	res := make(map[string]int64)
	res["utcoffset_sec"] = c.utcOffsetSec
	res["phcoffset_ns"] = c.phcOffsetNS
	res["oscillatoroffset_ns"] = c.oscillatorOffsetNS
	res["clockaccuracy"] = c.clockAccuracy
	res["clockclass"] = c.clockClass
	res["reload"] = c.reload
	res["dataerror"] = c.dataError

	return res
}
