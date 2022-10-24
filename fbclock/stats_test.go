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

package fbclock

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStatsUpdate(t *testing.T) {
	type in struct {
		tt  *TrueTime
		err error
	}
	testCases := []struct {
		name   string
		inputs []in
		want   Stats
	}{
		{
			name: "empty",
			want: Stats{},
		},
		{
			name: "single error",
			inputs: []in{
				{
					tt:  nil,
					err: fmt.Errorf("oh no"),
				},
			},
			want: Stats{
				Requests: 1,
				Errors:   1,
			},
		},
		{
			name: "single value",
			inputs: []in{
				{
					tt:  &TrueTime{Earliest: time.Unix(0, 1648137249050666302), Latest: time.Unix(0, 1648137249050666313)},
					err: nil,
				},
			},
			want: Stats{
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
					tt:  &TrueTime{Earliest: time.Unix(0, 1648137249050666302), Latest: time.Unix(0, 1648137249050666313)},
					err: nil,
				},
				{
					tt:  &TrueTime{Earliest: time.Unix(0, 1648137249050666902), Latest: time.Unix(0, 1648137249050667333)},
					err: nil,
				},
				{
					tt:  nil,
					err: fmt.Errorf("oh no"),
				},
				{
					tt:  &TrueTime{Earliest: time.Unix(0, 1648137249050667499), Latest: time.Unix(0, 1648137249050668300)},
					err: nil,
				},
				{
					tt:  nil,
					err: fmt.Errorf("whoops"),
				},
				{
					tt:  &TrueTime{Earliest: time.Unix(0, 1648137249050668999), Latest: time.Unix(0, 1648137249050699300)},
					err: nil,
				},
			},
			want: Stats{
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
			s := &StatsCollector{}
			for _, v := range tt.inputs {
				s.Update(v.tt, v.err)
			}
			require.Equal(t, tt.want, s.Stats())
		})
	}
}
