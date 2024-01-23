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

func TestNewPeerFromNTP(t *testing.T) {
	tests := []struct {
		name    string
		p       *control.NTPControlMsg
		want    *Peer
		wantErr bool
	}{
		{
			name:    "wrong operation type should give error",
			p:       &control.NTPControlMsg{},
			want:    nil,
			wantErr: true,
		},
		{
			name: "empty should give error",
			p: &control.NTPControlMsg{
				NTPControlMsgHead: control.NTPControlMsgHead{
					VnMode: control.MakeVnMode(3, control.Mode),
					REMOp:  control.OpReadVariables,
				},
				Data: []uint8(""),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "valid packet",
			p: &control.NTPControlMsg{
				NTPControlMsgHead: control.NTPControlMsgHead{
					VnMode: control.MakeVnMode(3, control.Mode),
					REMOp:  control.OpReadVariables,
					Status: (&control.PeerStatusWord{
						PeerStatus: control.PeerStatus{
							Broadcast:   false,
							Reachable:   true,
							AuthEnabled: false,
							AuthOK:      false,
							Configured:  true,
						},
						PeerSelection:    4,
						PeerEventCounter: 1,
						PeerEventCode:    2,
					}).Word(),
				},
				Data: []uint8("stratum=3,offset=0.1,hpoll=1024,ppoll=10"),
			},
			want: &Peer{
				Stratum:    3,
				Offset:     0.1,
				HPoll:      1024,
				PPoll:      10,
				Flashers:   []string{},
				Configured: true,
				Reachable:  true,
				Selection:  control.SelCandidate,
				Condition:  control.PeerSelect[control.SelCandidate],
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewPeerFromNTP(tt.p)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewPeerFromNTP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestNewPeerFromChrony(t *testing.T) {
	sourceData := &chrony.ReplySourceData{}
	sourceData.Stratum = 3
	sourceData.Poll = 10
	sourceData.Reachability = 255
	sourceData.State = chrony.SourceStateCandidate
	sourceData.Flags = chrony.NTPFlagsTests

	ntpData := &chrony.ReplyNTPData{}
	ntpData.Poll = 10
	ntpData.RefID = 123456
	ntpData.RefTime = time.Unix(1587738257, 0)
	ntpSourceName := &chrony.ReplyNTPSourceName{}
	tests := []struct {
		name    string
		s       *chrony.ReplySourceData
		p       *chrony.ReplyNTPData
		n       *chrony.ReplyNTPSourceName
		want    *Peer
		wantErr bool
	}{
		{
			name:    "no data",
			s:       nil,
			p:       nil,
			n:       nil,
			want:    nil,
			wantErr: true,
		},
		{
			name: "fallback, no ReplyNTPData",
			s:    sourceData,
			p:    nil,
			want: &Peer{
				Stratum:    3,
				Offset:     -0,
				HPoll:      10,
				PPoll:      10,
				Flashers:   []string{},
				Configured: true,
				Reachable:  true,
				Selection:  control.SelCandidate,
				Condition:  control.PeerSelect[control.SelCandidate],
				Reach:      255,
				SRCAdr:     "<nil>",
			},
			wantErr: false,
		},
		{
			name: "full data",
			s:    sourceData,
			p:    ntpData,
			n:    ntpSourceName,
			want: &Peer{
				Stratum:    3,
				Offset:     -0,
				HPoll:      10,
				PPoll:      10,
				Flashers:   []string{},
				Configured: true,
				Reachable:  true,
				Selection:  control.SelCandidate,
				Condition:  control.PeerSelect[control.SelCandidate],
				Reach:      255,
				SRCAdr:     "<nil>",
				DSTAdr:     "<nil>",
				RefID:      "0001E240",
				RefTime:    ntpData.RefTime.String(),
				Hostname:   "",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewPeerFromChrony(tt.s, tt.p, tt.n)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewPeerFromChrony() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			require.Equal(t, tt.want, got)
		})
	}
}
