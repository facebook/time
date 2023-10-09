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

func TestNormalizeData(t *testing.T) {
	data := []byte(
		`version="ntpd 4.2.6p5@1.2349-o Fri Apr 13 12:52:27 UTC 2018 (1)",
processor="x86_64", system="Linux/4.11.3-61_fbk16_3934_gd064a3c",
leap=0, stratum=4, precision=-24, rootdelay=64.685, rootdisp=76.350,
refid=174.141.68.116, reftime=0xdfb39d2d.8598591b,
clock=0xdfb39fbe.dd542f86, peer=60909, tc=10, mintc=3, offset=-0.180,
frequency=0.314, sys_jitter=0.246, clk_jitter=0.140, clk_wander=0.009
`)
	parsed, err := NormalizeData(data)

	require.NoError(t, err)
	expected := map[string]string{
		"version":   "ntpd 4.2.6p5@1.2349-o Fri Apr 13 12:52:27 UTC 2018 (1)",
		"processor": "x86_64", "system": "Linux/4.11.3-61_fbk16_3934_gd064a3c",
		"leap":       "0",
		"stratum":    "4",
		"precision":  "-24",
		"rootdelay":  "64.685",
		"rootdisp":   "76.350",
		"refid":      "174.141.68.116",
		"reftime":    "0xdfb39d2d.8598591b",
		"clock":      "0xdfb39fbe.dd542f86",
		"peer":       "60909",
		"tc":         "10",
		"mintc":      "3",
		"offset":     "-0.180",
		"frequency":  "0.314",
		"sys_jitter": "0.246",
		"clk_jitter": "0.140",
		"clk_wander": "0.009",
	}
	require.Equal(t, expected, parsed)
}

// test that we skip bad pairs
func TestNormalizeDataCorrupted(t *testing.T) {
	data := []byte(`srcadr=2401:db00:3110:5068:face:0:5c:0, srcport=123,
dstadr=2401:db00:3110:915d:face:0:5a:0, dstport=123, leap=0, stratum=3,
precision=-24, rootdelay=83.313, rootdisp=47.607, refid=1.104.123.73,
reftime=0xdfb8e24c.b57496e4, rec=0xdfb8e395.93319ff3, reach=0xff,
unreach=0, hmode=3, pmode=4, hpoll=7, ppoll=7, headway=8, flash=0x0,
keyid=0, offset=0.163, delay=0.136, dispersion=5.123, jitter=0.054,
xleave=0.022, filtdelay= 0.33 0.16 0.14 0.27 0.27 0.29 0.18 0.24filtoffset= 0.17 0.19 0.16 0.12 0.09 0.11 0.09 0.10,
filtdisp= 0.00 1.95 3.87 5.79 7.79 9.78 11.72 13.71
`)
	parsed, err := NormalizeData(data)

	require.NoError(t, err)
	expected := map[string]string{
		"delay":      "0.136",
		"dispersion": "5.123",
		"dstadr":     "2401:db00:3110:915d:face:0:5a:0",
		"dstport":    "123",
		"filtdisp":   "0.00 1.95 3.87 5.79 7.79 9.78 11.72 13.71",
		"flash":      "0x0",
		"headway":    "8",
		"hmode":      "3",
		"hpoll":      "7",
		"jitter":     "0.054",
		"keyid":      "0",
		"leap":       "0",
		"offset":     "0.163",
		"pmode":      "4",
		"ppoll":      "7",
		"precision":  "-24",
		"refid":      "1.104.123.73",
		"reftime":    "0xdfb8e24c.b57496e4",
		"rootdelay":  "83.313",
		"rootdisp":   "47.607",
		"stratum":    "3",
		"reach":      "0xff",
		"rec":        "0xdfb8e395.93319ff3",
		"srcadr":     "2401:db00:3110:5068:face:0:5c:0",
		"srcport":    "123",
		"unreach":    "0",
		"xleave":     "0.022",
	}
	require.Equal(t, expected, parsed)
}

func TestPeerStatus(t *testing.T) {
	var wantByte uint8 = 0x12
	wantPeerStatus := PeerStatus{
		Broadcast:   false,
		Reachable:   true,
		AuthEnabled: false,
		AuthOK:      false,
		Configured:  true,
	}
	input := wantPeerStatus.Byte()
	require.Equal(t, wantByte, input)
	peerStatus := ReadPeerStatus(input)
	require.Equal(t, wantPeerStatus, peerStatus)
}

func TestPeerStatusWord(t *testing.T) {
	var wantWord uint16 = 0x9412
	wantPeerStatusWord := &PeerStatusWord{
		PeerStatus: PeerStatus{
			Broadcast:   false,
			Reachable:   true,
			AuthEnabled: false,
			AuthOK:      false,
			Configured:  true,
		},
		PeerSelection:    4,
		PeerEventCounter: 1,
		PeerEventCode:    2,
	}
	input := wantPeerStatusWord.Word()
	require.Equal(t, wantWord, input)
	peerStatusWord := ReadPeerStatusWord(input)
	require.Equal(t, wantPeerStatusWord, peerStatusWord)
}

func TestSystemStatusWord(t *testing.T) {
	var wantWord uint16 = 0x4342
	wantSystemStatusWord := &SystemStatusWord{
		LI:                 1, // add_sec
		ClockSource:        3, // hf_radio
		SystemEventCounter: 4,
		SystemEventCode:    2, // freq_mode
	}
	input := wantSystemStatusWord.Word()
	require.Equal(t, wantWord, input)
	systemStatusWord := ReadSystemStatusWord(input)
	require.Equal(t, wantSystemStatusWord, systemStatusWord)
}

func TestMakeVnMode(t *testing.T) {
	version := 3
	mode := 2
	msg := NTPControlMsgHead{
		VnMode: MakeVnMode(version, mode),
	}
	require.Equal(t, version, msg.GetVersion())
	require.Equal(t, mode, msg.GetMode())
}

func TestMakeREMOp(t *testing.T) {
	response := true
	err := false
	more := true
	op := OpReadVariables
	msg := NTPControlMsgHead{
		REMOp: MakeREMOp(response, err, more, op),
	}
	require.True(t, msg.IsResponse())
	require.False(t, msg.HasError())
	require.True(t, msg.HasMore())
	require.Equal(t, uint8(op), msg.GetOperation())
}

func uint16to2x8(d uint16) []uint8 {
	return []uint8{uint8((d & 65280) >> 8), uint8(d & 255)}
}

func TestNTPControlMsg_GetAssociations(t *testing.T) {
	peerStatusWord1 := &PeerStatusWord{
		PeerStatus: PeerStatus{
			Broadcast:   false,
			Reachable:   true,
			AuthEnabled: false,
			AuthOK:      false,
			Configured:  true,
		},
		PeerSelection:    4,
		PeerEventCounter: 1,
		PeerEventCode:    2,
	}
	peerStatusWord2 := &PeerStatusWord{
		PeerStatus: PeerStatus{
			Broadcast:   false,
			Reachable:   true,
			AuthEnabled: false,
			AuthOK:      false,
			Configured:  true,
		},
		PeerSelection:    6,
		PeerEventCounter: 0,
		PeerEventCode:    3,
	}
	assocData := []uint8{}
	assocData = append(assocData, uint16to2x8(1)...)
	assocData = append(assocData, uint16to2x8(peerStatusWord1.Word())...)
	assocData = append(assocData, uint16to2x8(2)...)
	assocData = append(assocData, uint16to2x8(peerStatusWord2.Word())...)
	msg := NTPControlMsg{
		NTPControlMsgHead: NTPControlMsgHead{
			REMOp: MakeREMOp(true, false, false, OpReadStatus),
			Count: uint16(len(assocData)),
		},
		Data: assocData,
	}
	want := map[uint16]*PeerStatusWord{
		1: peerStatusWord1,
		2: peerStatusWord2,
	}
	got, err := msg.GetAssociations()
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func FuzzNormalizeData(f *testing.F) {
	data_0 := []byte(
		`version="ntpd 4.2.6p5@1.2349-o Fri Apr 13 12:52:27 UTC 2018 (1)",
processor="x86_64", system="Linux/4.11.3-61_fbk16_3934_gd064a3c",
leap=0, stratum=4, precision=-24, rootdelay=64.685, rootdisp=76.350,
refid=174.141.68.116, reftime=0xdfb39d2d.8598591b,
clock=0xdfb39fbe.dd542f86, peer=60909, tc=10, mintc=3, offset=-0.180,
frequency=0.314, sys_jitter=0.246, clk_jitter=0.140, clk_wander=0.009
`)

	data_1 := []byte(`srcadr=2401:db00:3110:5068:face:0:5c:0, srcport=123,
dstadr=2401:db00:3110:915d:face:0:5a:0, dstport=123, leap=0, stratum=3,
precision=-24, rootdelay=83.313, rootdisp=47.607, refid=1.104.123.73,
reftime=0xdfb8e24c.b57496e4, rec=0xdfb8e395.93319ff3, reach=0xff,
unreach=0, hmode=3, pmode=4, hpoll=7, ppoll=7, headway=8, flash=0x0,
keyid=0, offset=0.163, delay=0.136, dispersion=5.123, jitter=0.054,
xleave=0.022, filtdelay= 0.33 0.16 0.14 0.27 0.27 0.29 0.18 0.24filtoffset= 0.17 0.19 0.16 0.12 0.09 0.11 0.09 0.10,
filtdisp= 0.00 1.95 3.87 5.79 7.79 9.78 11.72 13.71
`)

	f.Add(data_0)
	f.Add(data_1)

	f.Fuzz(func(t *testing.T, input []byte) {
		_, _ = NormalizeData(input)
	})
}
