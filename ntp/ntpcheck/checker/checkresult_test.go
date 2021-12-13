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

	"github.com/facebookincubator/time/ntp/protocol/control"
	"github.com/stretchr/testify/require"
)

func TestNTPCheckResult_FindSysPeer(t *testing.T) {
	tests := []struct {
		name    string
		r       *NTPCheckResult
		want    *Peer
		wantErr bool
	}{
		{
			name:    "no peers",
			r:       &NTPCheckResult{},
			want:    nil,
			wantErr: true,
		},
		{
			name: "no sys peer",
			r: &NTPCheckResult{
				Peers: map[uint16]*Peer{
					0: &Peer{
						Selection: control.SelCandidate,
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "found sys peer",
			r: &NTPCheckResult{
				Peers: map[uint16]*Peer{
					0: &Peer{
						Selection: control.SelCandidate,
					},
					1: &Peer{
						Selection: control.SelSYSPeer,
					},
				},
			},
			want: &Peer{
				Selection: control.SelSYSPeer,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.r.FindSysPeer()
			if (err != nil) != tt.wantErr {
				t.Errorf("NTPCheckResult.FindSysPeer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestNTPCheckResult_FindGoodPeers(t *testing.T) {
	tests := []struct {
		name    string
		r       *NTPCheckResult
		want    []*Peer
		wantErr bool
	}{
		{
			name:    "no peers",
			r:       &NTPCheckResult{},
			want:    []*Peer{},
			wantErr: true,
		},
		{
			name: "no good peers",
			r: &NTPCheckResult{
				Peers: map[uint16]*Peer{
					0: &Peer{
						Selection: control.SelOutlier,
					},
					1: &Peer{
						Selection: control.SelReject,
					},
				},
			},
			want:    []*Peer{},
			wantErr: true,
		},
		{
			name: "some good peers",
			r: &NTPCheckResult{
				Peers: map[uint16]*Peer{
					0: &Peer{
						Selection: control.SelOutlier,
					},
					1: &Peer{
						Selection: control.SelReject,
					},
					2: &Peer{
						Selection: control.SelCandidate,
					},
					3: &Peer{
						Selection: control.SelBackup,
					},
				},
			},
			want: []*Peer{
				&Peer{
					Selection: control.SelCandidate,
				},
				&Peer{
					Selection: control.SelBackup,
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.r.FindGoodPeers()
			if (err != nil) != tt.wantErr {
				t.Errorf("NTPCheckResult.FindGoodPeers() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			require.ElementsMatch(t, tt.want, got)
		})
	}
}
