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

package test

import (
	"fmt"
	"testing"
	"time"

	lib "github.com/facebook/time/fbclock"

	"github.com/stretchr/testify/require"
)

func TestStatsUpdate(t *testing.T) {
	type in struct {
		tt  *lib.TrueTime
		err error
	}
	testCases := []struct {
		name   string
		inputs []in
		want   lib.Stats
	}{
		{
			name: "empty",
			want: lib.Stats{},
		},
		{
			name: "single error",
			inputs: []in{
				{
					tt:  nil,
					err: fmt.Errorf("oh no"),
				},
			},
			want: lib.Stats{
				Requests: 1,
				Errors:   1,
			},
		},
		{
			name: "single value",
			inputs: []in{
				{
					tt:  &lib.TrueTime{Earliest: time.Unix(0, 1648137249050666302), Latest: time.Unix(0, 1648137249050666313)},
					err: nil,
				},
			},
			want: lib.Stats{
				Requests:  1,
				Errors:    0,
				WOUAvg:    11,
				WOUMax:    11,
				WOUlt10us: 1,
			},
		},
		{
			name: "mixed",
			inputs: []in{
				{
					tt:  &lib.TrueTime{Earliest: time.Unix(0, 1648137249050666302), Latest: time.Unix(0, 1648137249050666313)},
					err: nil,
				},
				{
					tt:  &lib.TrueTime{Earliest: time.Unix(0, 1648137249050666902), Latest: time.Unix(0, 1648137249050667333)},
					err: nil,
				},
				{
					tt:  nil,
					err: fmt.Errorf("oh no"),
				},
				{
					tt:  &lib.TrueTime{Earliest: time.Unix(0, 1648137249050667499), Latest: time.Unix(0, 1648137249050668300)},
					err: nil,
				},
				{
					tt:  nil,
					err: fmt.Errorf("whoops"),
				},
				{
					tt:  &lib.TrueTime{Earliest: time.Unix(0, 1648137249050668999), Latest: time.Unix(0, 1648137249050699300)},
					err: nil,
				},
			},
			want: lib.Stats{
				Requests:   6,
				Errors:     2,
				WOUAvg:     7886,
				WOUMax:     30301,
				WOUlt10us:  3,
				WOUlt100us: 1,
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			s := &lib.StatsCollector{}
			for _, v := range tt.inputs {
				s.Update(v.tt, v.err)
			}
			require.Equal(t, tt.want, s.Stats())
		})
	}
}
