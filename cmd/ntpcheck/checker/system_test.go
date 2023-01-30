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
	"time"

	"github.com/stretchr/testify/require"

	"github.com/facebook/time/ntp/chrony"
	"github.com/facebook/time/ntp/control"
)

func TestSanityCheckSysVars(t *testing.T) {
	tests := []struct {
		name    string
		sysVars *SystemVariables
		wantErr bool
	}{
		{
			name:    "nil SystemVariables doesn't pass sanity check",
			sysVars: nil,
			wantErr: true,
		},
		{
			name:    "SystemVariables with Stratum=0 doesn't pass sanity check",
			sysVars: &SystemVariables{},
			wantErr: true,
		},
		{
			name:    "SystemVariables with Stratum=1 passes sanity check",
			sysVars: &SystemVariables{Stratum: 1},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := sanityCheckSysVars(tt.sysVars); (err != nil) != tt.wantErr {
				t.Errorf("sanityCheckSysVars() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewSystemVariablesFromChrony(t *testing.T) {
	p := &chrony.ReplyTracking{}
	p.LeapStatus = 1
	p.Stratum = 3
	p.RootDelay = 0.003
	p.RootDispersion = 0.001
	p.RefID = 123456
	p.RefTime = time.Unix(1587738257, 0)
	p.LastOffset = 0.010
	p.FreqPPM = 100
	s := NewSystemVariablesFromChrony(p)
	expected := &SystemVariables{
		Leap:      1,
		Stratum:   3,
		RootDelay: 3,
		RootDisp:  1,
		RefID:     "0001E240",
		RefTime:   p.RefTime.String(),
		Offset:    10,
		Frequency: 100,
	}
	require.Equal(t, expected, s)
}

func TestNewSystemVariablesFromNTP(t *testing.T) {
	tests := []struct {
		name    string
		p       *control.NTPControlMsg
		want    *SystemVariables
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
			name: "valid system variables",
			p: &control.NTPControlMsg{
				NTPControlMsgHead: control.NTPControlMsgHead{
					VnMode: vnMode,
					REMOp:  control.OpReadVariables,
				},
				Data: []uint8("leap=1,stratum=3,rootdelay=3,rootdisp=1,refid=0001E240,reftime=0x12345,offset=10,frequency=100"),
			},
			want: &SystemVariables{
				Leap:      1,
				Stratum:   3,
				RootDelay: 3,
				RootDisp:  1,
				RefID:     "0001E240",
				RefTime:   "0x12345",
				Offset:    10,
				Frequency: 100,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewSystemVariablesFromNTP(tt.p)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewSystemVariablesFromNTP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			require.Equal(t, tt.want, got)
		})
	}
}
