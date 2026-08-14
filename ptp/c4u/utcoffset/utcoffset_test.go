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
	"testing"
	"time"

	"github.com/facebook/time/leapsectz"
	"github.com/facebook/time/leapsectz/leaptest"
	"github.com/stretchr/testify/require"
)

func TestRunValidatesOffsetSanity(t *testing.T) {
	tests := []struct {
		name    string
		nleap   int32
		want    time.Duration
		wantErr string
	}{
		{
			name:  "lower bound",
			nleap: 20,
			want:  30 * time.Second,
		},
		{
			name:  "current offset",
			nleap: 27,
			want:  37 * time.Second,
		},
		{
			name:  "upper bound",
			nleap: 40,
			want:  50 * time.Second,
		},
		{
			name:    "above range",
			nleap:   41,
			wantErr: "unusable UTC offset 51s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			leaptest.Use(t, tt.nleap)

			got, err := Run()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestRunRejectsOffsetFromATableWithNoPastLeapSecond(t *testing.T) {
	leaptest.UseFutureOnly(t)

	latest, err := leapsectz.Latest("")
	require.NoError(t, err, "leapsectz reports no leap second without an error")
	require.Zero(t, latest.Nleap)

	_, err = Run()
	require.ErrorContains(t, err, "unusable UTC offset 10s")
}
