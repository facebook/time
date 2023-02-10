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
	got := SysoffEstimateBasic(ts1, rt, ts2)
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
	got := SysoffEstimateExtended(extended)
	want := SysoffResult{
		SysTime: time.Unix(0, 1667818190552297683),
		PHCTime: time.Unix(0, 1667818153552297661),
		Delay:   time.Duration(78),
		Offset:  time.Duration(37000000022),
	}
	require.Equal(t, want, got)
}

func TestSysoffFromExtendedTS(t *testing.T) {
	extendedTS := [3]PTPClockTime{
		{Sec: 1667818190, NSec: 552297411},
		{Sec: 1667818153, NSec: 552297462},
		{Sec: 1667818190, NSec: 552297522},
	}
	sysoff := sysoffFromExtendedTS(extendedTS)
	want := SysoffResult{
		SysTime: time.Unix(1667818190, 552297466),
		PHCTime: time.Unix(1667818153, 552297462),
		Delay:   111,
		Offset:  37000000004,
	}
	require.Equal(t, want, sysoff)
}

func TestOffsetBetweenExtendedReadings(t *testing.T) {
	extendedA := &PTPSysOffsetExtended{
		NSamples: 6,
		TS: [ptpMaxSamples][3]PTPClockTime{
			{{Sec: 1667818190, NSec: 552297411}, {Sec: 1667818153, NSec: 552297462}, {Sec: 1667818190, NSec: 552297522}},
			{{Sec: 1667818190, NSec: 552297533}, {Sec: 1667818153, NSec: 552297582}, {Sec: 1667818190, NSec: 552297602}},
			{{Sec: 1667818190, NSec: 552297644}, {Sec: 1667818153, NSec: 552297661}, {Sec: 1667818190, NSec: 552297722}},
			{{Sec: 1667818190, NSec: 552297755}, {Sec: 1667818153, NSec: 552297782}, {Sec: 1667818190, NSec: 552297822}},
			{{Sec: 1667818190, NSec: 552297866}, {Sec: 1667818153, NSec: 552297861}, {Sec: 1667818190, NSec: 552297922}},
			{{Sec: 1667818190, NSec: 552297966}, {Sec: 1667818153, NSec: 552297961}, {Sec: 1667818190, NSec: 552298022}},
		},
	}

	extendedB := &PTPSysOffsetExtended{
		NSamples: 5,
		TS: [ptpMaxSamples][3]PTPClockTime{
			{{Sec: 1667818191, NSec: 552298311}, {Sec: 1667818154, NSec: 552297452}, {Sec: 1667818191, NSec: 552298512}},
			{{Sec: 1667818191, NSec: 552298033}, {Sec: 1667818154, NSec: 552297572}, {Sec: 1667818191, NSec: 552298712}},
			{{Sec: 1667818191, NSec: 552299644}, {Sec: 1667818154, NSec: 552297691}, {Sec: 1667818191, NSec: 552308702}},
			{{Sec: 1667818191, NSec: 552300755}, {Sec: 1667818154, NSec: 552297782}, {Sec: 1667818191, NSec: 552309812}},
			{{Sec: 1667818191, NSec: 552301866}, {Sec: 1667818154, NSec: 552297861}, {Sec: 1667818191, NSec: 552328912}},
		},
	}
	offset := OffsetBetweenExtendedReadings(extendedA, extendedB)
	require.Equal(t, time.Duration(-815), offset)
}
