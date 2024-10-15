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

package node

import (
	"strconv"
	"sync"
	"testing"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/require"
)

// raw PTP packet (SYNC) received from sender
var syncPTPPacket = []byte{
	0xb8, 0xce, 0xf6, 0x61, 0x00, 0x80, 0xc2, 0x18, 0x50, 0x09, 0xca, 0x4e, 0x86, 0xdd, 0x60, 0x00,
	0x00, 0x00, 0x00, 0x36, 0x11, 0x01, 0xfa, 0xce, 0xdb, 0x00, 0xfa, 0xce, 0x12, 0x02, 0xfa, 0xce,
	0x00, 0x00, 0xfa, 0xce, 0x00, 0xff, 0xfa, 0xce, 0xdb, 0x00, 0xfa, 0xce, 0x26, 0x08, 0xfa, 0xce,
	0x00, 0x00, 0xfa, 0xce, 0x00, 0xff, 0x80, 0x02, 0x01, 0x3f, 0x00, 0x36, 0x51, 0xeb, 0x00, 0x02,
	0x00, 0x2c, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0f, 0x57, 0x10, 0x05, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x02, 0x00, 0x06, 0xff, 0x7f,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

// raw PTP packet (Signaling) received from sender
var signalPTPPacket = []byte{
	0xb8, 0xce, 0xf6, 0x61, 0x00, 0x80, 0xc2, 0x18, 0x50, 0x09, 0xca, 0x4e, 0x86, 0xdd, 0x60, 0x00,
	0x00, 0x00, 0x00, 0x36, 0x11, 0x01, 0xfa, 0xce, 0xdb, 0x00, 0xfa, 0xce, 0x14, 0x20, 0xfa, 0xce,
	0x00, 0x00, 0xfa, 0xce, 0x00, 0xff, 0xfa, 0xce, 0xdb, 0x00, 0xfa, 0xce, 0x26, 0x08, 0xfa, 0xce,
	0x00, 0x00, 0xfa, 0xce, 0x00, 0xff, 0xd2, 0xfa, 0x01, 0x3f, 0x00, 0x36, 0x51, 0xeb, 0x0c, 0x02,
	0x00, 0x2c, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0f, 0x57, 0x10, 0x05, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xd2, 0xfa, 0x00, 0x06, 0xff, 0x7f,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

func TestIncRunningHandlers(t *testing.T) {
	r := Receiver{
		Config:          &Config{PTPRecvHandlers: 2},
		Mutex:           &sync.Mutex{},
		runningHandlers: 0,
	}

	require.Equal(t, 0, r.runningHandlers)
	require.Equal(t, true, r.incRunningHandlers())
	require.Equal(t, 1, r.runningHandlers)
	require.Equal(t, true, r.incRunningHandlers())
	require.Equal(t, 2, r.runningHandlers)
	require.Equal(t, false, r.incRunningHandlers())
	require.Equal(t, 2, r.runningHandlers)
}

func TestDecRunningHandlers(t *testing.T) {
	r := Receiver{
		Config:          &Config{PTPRecvHandlers: 2},
		Mutex:           &sync.Mutex{},
		runningHandlers: 2,
	}

	require.Equal(t, true, r.decRunningHandlers())
	require.Equal(t, 1, r.runningHandlers)
	require.Equal(t, true, r.decRunningHandlers())
	require.Equal(t, 0, r.runningHandlers)
	require.Equal(t, false, r.decRunningHandlers())
}

func TestHandlePacket(t *testing.T) {
	packet := gopacket.NewPacket(syncPTPPacket, layers.LinkTypeEthernet, gopacket.DecodeOptions{Lazy: true, NoCopy: true})

	r := Receiver{
		Config:          &Config{PTPRecvHandlers: 3},
		Mutex:           &sync.Mutex{},
		runningHandlers: 0,
	}

	require.Equal(t, true, r.incRunningHandlers())
	require.Equal(t, true, r.incRunningHandlers())
	require.Equal(t, true, r.incRunningHandlers())
	require.Equal(t, false, r.incRunningHandlers())
	r.handlePacket(packet)
	r.handlePacket(packet)
	r.handlePacket(packet)
	require.Equal(t, true, r.incRunningHandlers())
}

func TestParseSyncPacketOnSyncPacket(t *testing.T) {
	packet := gopacket.NewPacket(syncPTPPacket, layers.LinkTypeEthernet, gopacket.DecodeOptions{Lazy: true, NoCopy: true})

	ptpPacket, srcIP, srcPort, err := parseSyncPacket(packet)
	require.Nil(t, err)
	require.Equal(t, uint8(ZiffyHexa), ptpPacket.ControlField)
	require.Equal(t, ptp.MessageSync, ptpPacket.MessageType())
	require.Equal(t, "face:db00:face:1202:face:0:face:ff", srcIP)
	require.Equal(t, strconv.Itoa(32770), srcPort)
	require.Equal(t, uint16(32770), ptpPacket.SourcePortIdentity.PortNumber)
}

func TestParseSyncPacketOnSignalPacket(t *testing.T) {
	packet := gopacket.NewPacket(signalPTPPacket, layers.LinkTypeEthernet, gopacket.DecodeOptions{Lazy: true, NoCopy: true})

	_, _, _, err := parseSyncPacket(packet)
	require.NotNil(t, err)
}
