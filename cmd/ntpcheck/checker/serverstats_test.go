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

package checker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/facebook/time/ntp/chrony"
	"github.com/facebook/time/ntp/control"
)

func TestNewServerStatsFromChrony(t *testing.T) {
	p := &chrony.ReplyServerStats{}
	p.NTPHits = 1234
	p.NTPDrops = 5678
	s := NewServerStatsFromChrony(p)

	expected := &ServerStats{
		PacketsReceived: 1234,
		PacketsDropped:  5678,
	}
	require.Equal(t, expected, s)
}

func TestNewServerStatsFromNTP(t *testing.T) {
	tests := []struct {
		name    string
		p       *control.NTPControlMsg
		want    *ServerStats
		wantErr bool
	}{
		{
			name:    "wrong operation type should give error",
			p:       &control.NTPControlMsg{},
			want:    nil,
			wantErr: true,
		},
		{
			name: "packet with empty data should give error",
			p: &control.NTPControlMsg{
				NTPControlMsgHead: control.NTPControlMsgHead{
					VnMode: vnMode,
					REMOp:  control.OpReadVariables,
				},
				Data: []uint8(""),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "valid packet should give no error",
			p: &control.NTPControlMsg{
				NTPControlMsgHead: control.NTPControlMsgHead{
					VnMode: vnMode,
					REMOp:  control.OpReadVariables,
				},
				Data: []uint8("ss_received=1234,ss_badformat=5670,ss_badauth=5,ss_declined=1,ss_restricted=1,ss_limited=1"),
			},
			want: &ServerStats{
				PacketsReceived: 1234,
				PacketsDropped:  5678,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewServerStatsFromNTP(tt.p)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			require.Equal(t, tt.want, got)
		})
	}
}
