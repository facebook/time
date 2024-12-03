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

package protocol

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTLVHeadType(t *testing.T) {
	head := &TLVHead{
		TLVType:     TLVRequestUnicastTransmission,
		LengthField: 10,
	}
	require.Equal(t, TLVRequestUnicastTransmission, head.Type())
}

func TestParseAnnounceWithPathTrace(t *testing.T) {
	raw := []uint8("\x0b\x12\x00\x4c\x00\x00\x04\x08\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x08\xc0\xeb\xff\xfe\x63\x7a\x4e\x00\x01\x00\x00\x05\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x25\x00\x80\xf8\xfe\xff\xff\x80\x08\xc0\xeb\xff\xfe\x63\x7a\x4e\x00\x00\xa0\x00\x08\x00\x18\x08\xc0\xeb\xff\xfe\x63\x7a\x4e\x01\xb6\xaf\xc4\xe5\x46\x12\x29\x04\xc0\x87\x32\xf0\x61\xee\xce\x00\x00")
	packet := new(Announce)
	err := FromBytes(raw, packet)
	require.Nil(t, err)
	want := Announce{
		Header: Header{
			SdoIDAndMsgType: NewSdoIDAndMsgType(MessageAnnounce, 0),
			Version:         Version,
			MessageLength:   76,
			DomainNumber:    0,
			FlagField:       FlagUnicast | FlagPTPTimescale,
			SequenceID:      0,
			SourcePortIdentity: PortIdentity{
				PortNumber:    1,
				ClockIdentity: 630763432548989518,
			},
			LogMessageInterval: 1,
			ControlField:       5,
		},
		AnnounceBody: AnnounceBody{
			CurrentUTCOffset:     37,
			Reserved:             0,
			GrandmasterPriority1: 128,
			GrandmasterClockQuality: ClockQuality{
				ClockClass:              248,
				ClockAccuracy:           254,
				OffsetScaledLogVariance: 65535,
			},
			GrandmasterPriority2: 128,
			GrandmasterIdentity:  630763432548989518,
			StepsRemoved:         0,
			TimeSource:           TimeSourceInternalOscillator,
		},
		TLVs: []TLV{
			&PathTraceTLV{
				TLVHead: TLVHead{
					TLVType:     TLVPathTrace,
					LengthField: 24,
				},
				PathSequence: []ClockIdentity{
					630763432548989518,
					123479299994292777,
					342422224531222222,
				},
			},
		},
	}
	require.Equal(t, want, *packet)
	b, err := Bytes(packet)
	require.Nil(t, err)
	require.Equal(t, raw, b)

	// test generic DecodePacket as well
	pp, err := DecodePacket(b)
	require.Nil(t, err)
	require.Equal(t, &want, pp)
}

func TestParseAnnounceWithAlternateTimeOffsetIndicator(t *testing.T) {
	raw := []uint8("\x0b\x12\x00\x5a\x00\x00\x04\x08\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x08\xc0\xeb\xff\xfe\x63\x7a\x4e\x00\x01\x00\x00\x05\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x25\x00\x80\xf8\xfe\xff\xff\x80\x08\xc0\xeb\xff\xfe\x63\x7a\x4e\x00\x00\xa0\x00\x09\x00\x16\x01\x00\x00\x00\x25\x00\x00\x00\x01\x00\x00\x62\xc2\xfd\xb6\x03\x50\x54\x50\x00\x00\x00")
	packet := new(Announce)
	err := FromBytes(raw, packet)
	require.Nil(t, err)
	want := Announce{
		Header: Header{
			SdoIDAndMsgType: NewSdoIDAndMsgType(MessageAnnounce, 0),
			Version:         Version,
			MessageLength:   90,
			DomainNumber:    0,
			FlagField:       FlagUnicast | FlagPTPTimescale,
			SequenceID:      0,
			SourcePortIdentity: PortIdentity{
				PortNumber:    1,
				ClockIdentity: 630763432548989518,
			},
			LogMessageInterval: 1,
			ControlField:       5,
		},
		AnnounceBody: AnnounceBody{
			CurrentUTCOffset:     37,
			Reserved:             0,
			GrandmasterPriority1: 128,
			GrandmasterClockQuality: ClockQuality{
				ClockClass:              248,
				ClockAccuracy:           254,
				OffsetScaledLogVariance: 65535,
			},
			GrandmasterPriority2: 128,
			GrandmasterIdentity:  630763432548989518,
			StepsRemoved:         0,
			TimeSource:           TimeSourceInternalOscillator,
		},
		TLVs: []TLV{
			&AlternateTimeOffsetIndicatorTLV{
				TLVHead: TLVHead{
					TLVType:     TLVAlternateTimeOffsetIndicator,
					LengthField: 22,
				},
				KeyField:       0x01,
				CurrentOffset:  37,
				JumpSeconds:    1,
				TimeOfNextJump: NewPTPSeconds(time.Unix(1656946102, 0)),
				DisplayName:    PTPText("PTP"),
			},
		},
	}
	require.Equal(t, want, *packet)
	b, err := Bytes(packet)
	require.Nil(t, err)
	require.Equal(t, raw, b)

	// test generic DecodePacket as well
	pp, err := DecodePacket(raw)
	require.Nil(t, err)
	require.Equal(t, &want, pp)
}

func TestParseSyncDelayReqWithAlternateResponsePort(t *testing.T) {
	raw := []byte{1, 18, 0, 49, 0, 0, 36, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 184, 206, 246, 255, 254, 68, 148, 144, 0, 1, 138, 207, 0, 127, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 66, 0, 1, 207, 0, 0}
	packet := new(SyncDelayReq)
	err := FromBytes(raw, packet)
	require.Nil(t, err)

	want := SyncDelayReq{
		Header: Header{
			SdoIDAndMsgType: NewSdoIDAndMsgType(MessageDelayReq, 0),
			Version:         Version,
			SequenceID:      35535,
			MessageLength:   49,
			FlagField:       FlagUnicast | FlagProfileSpecific1,
			SourcePortIdentity: PortIdentity{
				PortNumber:    1,
				ClockIdentity: 13316852727524136080,
			},
			LogMessageInterval: 0x7f,
		},
		TLVs: []TLV{&AlternateResponsePortTLV{
			TLVHead: TLVHead{TLVType: TLVAlternateResponsePort, LengthField: uint16(1)},
			Count:   uint8(207),
		}},
	}
	require.Equal(t, want, *packet)
	b, err := Bytes(packet)
	require.Nil(t, err)
	require.Equal(t, raw, b)

	// test generic DecodePacket as well
	pp, err := DecodePacket(raw)
	require.Nil(t, err)
	require.Equal(t, &want, pp)
}
