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
	"encoding/binary"

	ptp "github.com/facebook/time/ptp/protocol"
)

// formSignalingPacket creates PTP SIGNALING packet
// SequenceId contains origin hop; PortNumber contains origin port;
// ControlField contains the Zi(0xff)y identifier
func formSignalingPacket(hop int, routeIndex int) *ptp.Signaling {
	l := binary.Size(ptp.Header{}) + binary.Size(ptp.PortIdentity{}) + binary.Size(ptp.RequestUnicastTransmissionTLV{})
	return &ptp.Signaling{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageSignaling, 0),
			Version:         ptp.Version,
			SequenceID:      uint16(hop),
			MessageLength:   uint16(l),
			FlagField:       ptp.FlagUnicast,
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber: uint16(routeIndex),
			},
			ControlField:       ZiffyHexa, //identifier for zi(0xff)y
			LogMessageInterval: 0x7f,
		},
		TargetPortIdentity: ptp.PortIdentity{
			PortNumber:    0xffff,
			ClockIdentity: 0xffffffffffffffff,
		},
		TLVs: []ptp.TLV{
			&ptp.RequestUnicastTransmissionTLV{
				TLVHead: ptp.TLVHead{
					TLVType:     ptp.TLVRequestUnicastTransmission,
					LengthField: uint16(binary.Size(ptp.RequestUnicastTransmissionTLV{}) - binary.Size(ptp.TLVHead{})),
				},
				MsgTypeAndReserved:    ptp.NewUnicastMsgTypeAndFlags(ptp.MessageSync, 0),
				LogInterMessagePeriod: 1,
				DurationField:         0, // seconds
			},
		},
	}
}

// formSyncPacket creates PTP SYNC packet
// SequenceId contains origin hop; PortNumber contains origin port;
// ControlField contains the Zi(0xff)y identifier
func formSyncPacket(msgType ptp.MessageType, hop int, routeIndex int) *ptp.SyncDelayReq {
	return &ptp.SyncDelayReq{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(msgType, 0),
			Version:         ptp.Version,
			MessageLength:   uint16(binary.Size(ptp.Header{}) + binary.Size(ptp.SyncDelayReqBody{})), //#nosec G115
			FlagField:       ptp.FlagUnicast,
			SequenceID:      uint16(hop),
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber: uint16(routeIndex),
			},
			ControlField:       ZiffyHexa, //identifier for zi(0xff)y
			LogMessageInterval: 0x7f,
		},
	}
}
