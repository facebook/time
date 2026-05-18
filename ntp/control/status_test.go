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

package control

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadFlashStatusWord(t *testing.T) {
	// no flags set
	result := ReadFlashStatusWord(0)
	require.Empty(t, result)

	// single flag set (bit 0 = TEST1)
	result = ReadFlashStatusWord(0x0001)
	require.Len(t, result, 1)
	require.Equal(t, FlashDescMap[0x0001], result[0])

	// multiple flags — should return one entry per bit in FlashDescMap
	result = ReadFlashStatusWord(0xFFFF)
	require.Equal(t, len(FlashDescMap), len(result))
}

func TestGetSystemStatus(t *testing.T) {
	msg := NTPControlMsg{
		NTPControlMsgHead: NTPControlMsgHead{
			VnMode: MakeVnMode(2, 6),
			REMOp:  MakeREMOp(true, false, false, OpReadStatus),
			Status: 0xC601,
		},
	}
	ssw, err := msg.GetSystemStatus()
	require.NoError(t, err)
	require.NotNil(t, ssw)
	require.Equal(t, uint8(3), ssw.LI)
	require.Equal(t, uint8(6), ssw.ClockSource)
	require.Equal(t, uint8(0), ssw.SystemEventCounter)
	require.Equal(t, uint8(1), ssw.SystemEventCode)
}

func TestGetSystemStatusWrongOp(t *testing.T) {
	msg := NTPControlMsg{
		NTPControlMsgHead: NTPControlMsgHead{
			REMOp: MakeREMOp(true, false, false, OpReadVariables),
		},
	}
	_, err := msg.GetSystemStatus()
	require.Error(t, err)
}

func TestGetPeerStatus(t *testing.T) {
	msg := NTPControlMsg{
		NTPControlMsgHead: NTPControlMsgHead{
			VnMode: MakeVnMode(2, 6),
			REMOp:  MakeREMOp(true, false, false, OpReadVariables),
			Status: 0x9614,
		},
	}
	psw, err := msg.GetPeerStatus()
	require.NoError(t, err)
	require.NotNil(t, psw)
}

func TestGetPeerStatusWrongOp(t *testing.T) {
	msg := NTPControlMsg{
		NTPControlMsgHead: NTPControlMsgHead{
			REMOp: MakeREMOp(true, false, false, OpReadStatus),
		},
	}
	_, err := msg.GetPeerStatus()
	require.Error(t, err)
}

func TestGetAssociationInfo(t *testing.T) {
	// set up a message with OpReadVariables and data in "key=value" format
	data := []byte("srcadr=192.168.1.1, offset=0.123")
	msg := NTPControlMsg{
		NTPControlMsgHead: NTPControlMsgHead{
			VnMode: MakeVnMode(2, 6),
			REMOp:  MakeREMOp(true, false, false, OpReadVariables),
			Count:  uint16(len(data)),
		},
		Data: data,
	}
	result, err := msg.GetAssociationInfo()
	require.NoError(t, err)
	require.Len(t, result, 2)
	require.Equal(t, "192.168.1.1", result["srcadr"])
	require.Equal(t, "0.123", result["offset"])
}

func TestGetAssociationInfoWrongOp(t *testing.T) {
	msg := NTPControlMsg{
		NTPControlMsgHead: NTPControlMsgHead{
			REMOp: MakeREMOp(true, false, false, OpReadStatus),
		},
	}
	_, err := msg.GetAssociationInfo()
	require.Error(t, err)
}

func TestGetAssociations(t *testing.T) {
	// 2 associations, each encoded as 4 bytes (id uint16 + status uint16)
	data := []byte{
		0x00, 0x01, 0x96, 0x14, // id=1, status word
		0x00, 0x02, 0xF8, 0x00, // id=2, status word
	}
	msg := NTPControlMsg{
		NTPControlMsgHead: NTPControlMsgHead{
			VnMode: MakeVnMode(2, 6),
			REMOp:  MakeREMOp(true, false, false, OpReadStatus),
			Count:  uint16(len(data)),
		},
		Data: data,
	}
	assocs, err := msg.GetAssociations()
	require.NoError(t, err)
	require.Len(t, assocs, 2)
	require.Contains(t, assocs, uint16(1))
	require.Contains(t, assocs, uint16(2))
}

func TestSystemStatusWordRoundTrip(t *testing.T) {
	ssw := &SystemStatusWord{
		LI:                 2,
		ClockSource:        6,
		SystemEventCounter: 3,
		SystemEventCode:    7,
	}
	word := ssw.Word()
	decoded := ReadSystemStatusWord(word)
	require.Equal(t, ssw, decoded)
}

func TestPeerStatusWordRoundTrip(t *testing.T) {
	psw := &PeerStatusWord{
		PeerStatus: PeerStatus{
			Configured:  true,
			AuthOK:      false,
			AuthEnabled: true,
			Reachable:   true,
			Broadcast:   false,
		},
		PeerSelection:    SelSYSPeer,
		PeerEventCounter: 2,
		PeerEventCode:    1,
	}
	word := psw.Word()
	decoded := ReadPeerStatusWord(word)
	require.Equal(t, psw.PeerStatus.Configured, decoded.PeerStatus.Configured)
	require.Equal(t, psw.PeerStatus.AuthEnabled, decoded.PeerStatus.AuthEnabled)
	require.Equal(t, psw.PeerStatus.Reachable, decoded.PeerStatus.Reachable)
	require.Equal(t, psw.PeerSelection, decoded.PeerSelection)
}
