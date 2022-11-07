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

package phc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSysoffEstimateBasic(t *testing.T) {
	ts1 := time.Unix(0, 1667818190552297411)
	rt := time.Unix(0, 1667818153552297462)
	ts2 := time.Unix(0, 1667818190552297522)
	got := sysoffEstimateBasic(ts1, rt, ts2)
	want := SysoffResult{
		SysTime: time.Unix(0, 1667818190552297466),
		PHCTime: rt,
		Delay:   ts2.Sub(ts1),
		Offset:  time.Duration(37000000005),
	}
	require.Equal(t, want, got)
}

func TestSysoffEstimateExtended(t *testing.T) {
	extended := &PTPSysOffsetExtended{
		NSamples: 3,
		TS: [ptpMaxSamples][3]PTPClockTime{
			{{Sec: 1667818190, NSec: 552297411}, {Sec: 1667818153, NSec: 552297462}, {Sec: 1667818190, NSec: 552297522}},
			{{Sec: 1667818190, NSec: 552297533}, {Sec: 1667818153, NSec: 552297582}, {Sec: 1667818190, NSec: 552297622}},
			{{Sec: 1667818190, NSec: 552297644}, {Sec: 1667818153, NSec: 552297661}, {Sec: 1667818190, NSec: 552297722}},
		},
	}
	got := sysoffEstimateExtended(extended)
	want := SysoffResult{
		SysTime: time.Unix(0, 1667818190552297683),
		PHCTime: time.Unix(0, 1667818153552297661),
		Delay:   time.Duration(78),
		Offset:  time.Duration(37000000022),
	}
	require.Equal(t, want, got)
}
