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
	"os"
	"testing"
	"time"

	"github.com/facebook/time/leapsectz"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	if _, err := os.Stat(leapsectz.LeapFile()); err != nil {
		ls := make([]leapsectz.LeapSecond, 0, len(leapTimestamps))
		for i, ts := range leapTimestamps {
			ls = append(ls, leapsectz.LeapSecond{Tleap: ts, Nleap: int32(i + 1)})
		}
		f, err := os.CreateTemp("", "leaptest-")
		require.NoError(t, err)
		defer os.Remove(f.Name())
		err = leapsectz.Write(f, '2', ls, "UTC")
		require.NoError(t, err)
		require.NoError(t, f.Close())
		leapsectz.SetLeapFile(f.Name())
		defer leapsectz.SetLeapFile("")
	}
	u, err := Run()
	require.NoError(t, err)
	require.Greater(t, 50*time.Second, u)
	require.Less(t, 30*time.Second, u)
}

// All 27 leap seconds (POSIX timestamps including prior leap seconds)
var leapTimestamps = []uint64{
	78796800,   // 1972-07-01
	94694401,   // 1973-01-01
	126230402,  // 1974-01-01
	157766403,  // 1975-01-01
	189302404,  // 1976-01-01
	220924805,  // 1977-01-01
	252460806,  // 1978-01-01
	283996807,  // 1979-01-01
	315532808,  // 1980-01-01
	362793609,  // 1981-07-01
	394329610,  // 1982-07-01
	425865611,  // 1983-07-01
	489024012,  // 1985-07-01
	567993613,  // 1988-01-01
	631152014,  // 1990-01-01
	662688015,  // 1991-01-01
	709948816,  // 1992-07-01
	741484817,  // 1993-07-01
	773020818,  // 1994-07-01
	820454419,  // 1996-01-01
	867715220,  // 1997-07-01
	915148821,  // 1999-01-01
	1136073622, // 2006-01-01
	1230768023, // 2009-01-01
	1341100824, // 2012-07-01
	1435708825, // 2015-07-01
	1483228826, // 2017-01-01
}
