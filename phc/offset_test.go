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

	"github.com/facebook/time/phc/unix" // a temporary shim for "golang.org/x/sys/unix" until v0.27.0 is cut
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

func TestSysoffBestSample(t *testing.T) {
	extended := &PTPSysOffsetExtended{
		Samples: 3,
		Ts: [unix.PTP_MAX_SAMPLES][3]PtpClockTime{
			{{Sec: 1667818190, Nsec: 552297411}, {Sec: 1667818153, Nsec: 552297462}, {Sec: 1667818190, Nsec: 552297522}},
			{{Sec: 1667818190, Nsec: 552297533}, {Sec: 1667818153, Nsec: 552297582}, {Sec: 1667818190, Nsec: 552297622}},
			{{Sec: 1667818190, Nsec: 552297644}, {Sec: 1667818153, Nsec: 552297661}, {Sec: 1667818190, Nsec: 552297722}},
		},
	}
	got := extended.BestSample()
	want := SysoffResult{
		SysTime: time.Unix(0, 1667818190552297683),
		PHCTime: time.Unix(0, 1667818153552297661),
		Delay:   time.Duration(78),
		Offset:  time.Duration(37000000022),
	}
	require.Equal(t, want, got)
}

func TestSysoffFromExtendedTS(t *testing.T) {
	extendedTS := [3]PtpClockTime{
		{Sec: 1667818190, Nsec: 552297411},
		{Sec: 1667818153, Nsec: 552297462},
		{Sec: 1667818190, Nsec: 552297522},
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

func TestSysoffFromPrecise(t *testing.T) {
	preciseTs := &PTPSysOffsetPrecise{
		Realtime: PtpClockTime{Sec: 1667818190, Nsec: 552297411},
		Device:   PtpClockTime{Sec: 1667818153, Nsec: 552297462},
		Monoraw:  PtpClockTime{Sec: 1667818190, Nsec: 552297522},
	}
	sysoff := SysoffFromPrecise(preciseTs)
	want := SysoffResult{
		SysTime:    time.Unix(1667818190, 552297522),
		PHCTime:    time.Unix(1667818153, 552297462),
		SysClockID: unix.CLOCK_MONOTONIC_RAW,
		Delay:      0,
		Offset:     37000000060,
	}
	require.Equal(t, want, sysoff)
}

func TestOffsetBetweenPreciseReadings(t *testing.T) {
	preciseA := &PTPSysOffsetPrecise{
		Realtime: PtpClockTime{Sec: 1667818190, Nsec: 552297411},
		Device:   PtpClockTime{Sec: 1667818153, Nsec: 552297462},
		Monoraw:  PtpClockTime{Sec: 1667818190, Nsec: 552297522},
	}
	preciseB := &PTPSysOffsetPrecise{
		Realtime: PtpClockTime{Sec: 1667818190, Nsec: 552297666}, // 255ns later than A
		Device:   PtpClockTime{Sec: 1667818153, Nsec: 552297462},
		Monoraw:  PtpClockTime{Sec: 1667818190, Nsec: 552297777}, // 255ns later than A
	}
	offset := preciseB.Sub(preciseA)
	require.Equal(t, time.Duration(-255), offset)
}

func TestOffsetBetweenExtendedReadings(t *testing.T) {
	extendedA := &PTPSysOffsetExtended{
		Samples: 6,
		Ts: [unix.PTP_MAX_SAMPLES][3]PtpClockTime{
			{{Sec: 1667818190, Nsec: 552297411}, {Sec: 1667818153, Nsec: 552297462}, {Sec: 1667818190, Nsec: 552297522}},
			{{Sec: 1667818190, Nsec: 552297533}, {Sec: 1667818153, Nsec: 552297582}, {Sec: 1667818190, Nsec: 552297602}},
			{{Sec: 1667818190, Nsec: 552297644}, {Sec: 1667818153, Nsec: 552297661}, {Sec: 1667818190, Nsec: 552297722}},
			{{Sec: 1667818190, Nsec: 552297755}, {Sec: 1667818153, Nsec: 552297782}, {Sec: 1667818190, Nsec: 552297822}},
			{{Sec: 1667818190, Nsec: 552297866}, {Sec: 1667818153, Nsec: 552297861}, {Sec: 1667818190, Nsec: 552297922}},
			{{Sec: 1667818190, Nsec: 552297966}, {Sec: 1667818153, Nsec: 552297961}, {Sec: 1667818190, Nsec: 552298022}},
		},
	}

	extendedB := &PTPSysOffsetExtended{
		Samples: 5,
		Ts: [unix.PTP_MAX_SAMPLES][3]PtpClockTime{
			{{Sec: 1667818191, Nsec: 552298311}, {Sec: 1667818154, Nsec: 552297452}, {Sec: 1667818191, Nsec: 552298512}},
			{{Sec: 1667818191, Nsec: 552298033}, {Sec: 1667818154, Nsec: 552297572}, {Sec: 1667818191, Nsec: 552298712}},
			{{Sec: 1667818191, Nsec: 552299644}, {Sec: 1667818154, Nsec: 552297691}, {Sec: 1667818191, Nsec: 552308702}},
			{{Sec: 1667818191, Nsec: 552300755}, {Sec: 1667818154, Nsec: 552297782}, {Sec: 1667818191, Nsec: 552309812}},
			{{Sec: 1667818191, Nsec: 552301866}, {Sec: 1667818154, Nsec: 552297861}, {Sec: 1667818191, Nsec: 552328912}},
		},
	}
	offset, err := extendedB.Sub(extendedA)
	require.NoError(t, err)
	require.Equal(t, time.Duration(-815), offset)
}
