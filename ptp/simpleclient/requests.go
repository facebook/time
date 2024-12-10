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

package simpleclient

import (
	"encoding/binary"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
)

// reqUnicast is a helper to build ptp.RequestUnicastTransmission
func reqUnicast(clockID ptp.ClockIdentity, duration time.Duration, what ptp.MessageType) *ptp.Signaling {
	l := binary.Size(ptp.Header{}) + binary.Size(ptp.PortIdentity{}) + binary.Size(ptp.RequestUnicastTransmissionTLV{})
	return &ptp.Signaling{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageSignaling, 0),
			Version:         ptp.Version,
			SequenceID:      0, // will be populated on sending
			MessageLength:   uint16(l),
			FlagField:       ptp.FlagUnicast,
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber:    1,
				ClockIdentity: clockID,
			},
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
				MsgTypeAndReserved:    ptp.NewUnicastMsgTypeAndFlags(what, 0),
				LogInterMessagePeriod: 1,
				DurationField:         uint32(duration.Seconds()), // seconds
			},
		},
	}
}

// reqAckCancelUnicast is a helper to build ptp.AcknowledgeCancelUnicastTransmission
func reqAckCancelUnicast(clockID ptp.ClockIdentity, what ptp.MessageType) *ptp.Signaling {
	l := binary.Size(ptp.Header{}) + binary.Size(ptp.PortIdentity{}) + binary.Size(ptp.AcknowledgeCancelUnicastTransmissionTLV{})
	return &ptp.Signaling{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageSignaling, 0),
			Version:         ptp.Version,
			SequenceID:      0, // will be populated on sending
			MessageLength:   uint16(l),
			FlagField:       ptp.FlagUnicast,
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber:    1,
				ClockIdentity: clockID,
			},
			LogMessageInterval: 0x7f,
		},
		TargetPortIdentity: ptp.PortIdentity{
			PortNumber:    0xffff,
			ClockIdentity: 0xffffffffffffffff,
		},
		TLVs: []ptp.TLV{
			&ptp.AcknowledgeCancelUnicastTransmissionTLV{
				TLVHead: ptp.TLVHead{
					TLVType:     ptp.TLVCancelUnicastTransmission,
					LengthField: uint16(binary.Size(ptp.AcknowledgeCancelUnicastTransmissionTLV{}) - binary.Size(ptp.TLVHead{})),
				},
				MsgTypeAndFlags: ptp.NewUnicastMsgTypeAndFlags(what, 0),
			},
		},
	}
}

// reqDelay is a helper to build ptp.SyncDelayReq
func reqDelay(clockID ptp.ClockIdentity) *ptp.SyncDelayReq {
	return &ptp.SyncDelayReq{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageDelayReq, 0),
			Version:         ptp.Version,
			SequenceID:      0,                                                                       // will be populated on sending
			MessageLength:   uint16(binary.Size(ptp.Header{}) + binary.Size(ptp.SyncDelayReqBody{})), //#nosec G115
			FlagField:       ptp.FlagUnicast,
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber:    1,
				ClockIdentity: clockID,
			},
			LogMessageInterval: 0x7f,
		},
	}
}
