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
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	ptp "github.com/facebook/time/ptp/protocol"
)

func grantUnicastPkt(seq int, clockID ptp.ClockIdentity, duration time.Duration, what ptp.MessageType) *ptp.Signaling {
	l := binary.Size(ptp.Header{}) + binary.Size(ptp.PortIdentity{}) + binary.Size(ptp.GrantUnicastTransmissionTLV{})
	return &ptp.Signaling{
		Header: ptp.Header{
			SdoIDAndMsgType:    ptp.NewSdoIDAndMsgType(ptp.MessageSignaling, 0),
			Version:            ptp.Version,
			SequenceID:         uint16(seq),
			MessageLength:      uint16(l),
			FlagField:          ptp.FlagUnicast,
			LogMessageInterval: 0x7f,
		},
		TargetPortIdentity: ptp.PortIdentity{
			PortNumber:    1,
			ClockIdentity: clockID,
		},
		TLVs: []ptp.TLV{
			&ptp.GrantUnicastTransmissionTLV{
				TLVHead: ptp.TLVHead{
					TLVType:     ptp.TLVGrantUnicastTransmission,
					LengthField: uint16(binary.Size(ptp.GrantUnicastTransmissionTLV{}) - binary.Size(ptp.TLVHead{})),
				},
				MsgTypeAndReserved:    ptp.NewUnicastMsgTypeAndFlags(what, 0),
				LogInterMessagePeriod: 1,
				DurationField:         uint32(duration.Seconds()), // seconds
				Renewal:               1,
			},
		},
	}
}

func cancelUnicastPkt(seq int, clockID ptp.ClockIdentity, what ptp.MessageType) *ptp.Signaling {
	l := binary.Size(ptp.Header{}) + binary.Size(ptp.PortIdentity{}) + binary.Size(ptp.CancelUnicastTransmissionTLV{})
	return &ptp.Signaling{
		Header: ptp.Header{
			SdoIDAndMsgType:    ptp.NewSdoIDAndMsgType(ptp.MessageSignaling, 0),
			Version:            ptp.Version,
			SequenceID:         uint16(seq),
			MessageLength:      uint16(l),
			FlagField:          ptp.FlagUnicast,
			LogMessageInterval: 0x7f,
		},
		TargetPortIdentity: ptp.PortIdentity{
			PortNumber:    1,
			ClockIdentity: clockID,
		},
		TLVs: []ptp.TLV{
			&ptp.CancelUnicastTransmissionTLV{
				TLVHead: ptp.TLVHead{
					TLVType:     ptp.TLVCancelUnicastTransmission,
					LengthField: uint16(binary.Size(ptp.CancelUnicastTransmissionTLV{}) - binary.Size(ptp.TLVHead{})),
				},
				MsgTypeAndFlags: ptp.NewUnicastMsgTypeAndFlags(what, 0),
			},
		},
	}
}

func announcePkt(seq int) *ptp.Announce {
	l := binary.Size(ptp.Header{}) + binary.Size(ptp.AnnounceBody{})
	return &ptp.Announce{
		Header: ptp.Header{
			SdoIDAndMsgType:    ptp.NewSdoIDAndMsgType(ptp.MessageAnnounce, 0),
			Version:            ptp.Version,
			SequenceID:         uint16(seq),
			MessageLength:      uint16(l),
			FlagField:          ptp.FlagUnicast,
			LogMessageInterval: 0x7f,
		},
	}
}

func syncPkt(seq int) *ptp.SyncDelayReq {
	l := binary.Size(ptp.Header{}) + binary.Size(ptp.SyncDelayReqBody{})
	return &ptp.SyncDelayReq{
		Header: ptp.Header{
			SdoIDAndMsgType:    ptp.NewSdoIDAndMsgType(ptp.MessageSync, 0),
			Version:            ptp.Version,
			SequenceID:         uint16(seq),
			MessageLength:      uint16(l),
			FlagField:          ptp.FlagUnicast,
			LogMessageInterval: 0x7f,
		},
	}
}
func fwupPkt(seq int) *ptp.FollowUp {
	l := binary.Size(ptp.FollowUp{})
	return &ptp.FollowUp{
		Header: ptp.Header{
			SdoIDAndMsgType:    ptp.NewSdoIDAndMsgType(ptp.MessageFollowUp, 0),
			Version:            ptp.Version,
			SequenceID:         uint16(seq),
			MessageLength:      uint16(l),
			FlagField:          ptp.FlagUnicast,
			LogMessageInterval: 0x7f,
		},
		FollowUpBody: ptp.FollowUpBody{
			PreciseOriginTimestamp: ptp.NewTimestamp(time.Now()),
		},
	}
}
func delayRespPkt(seq int) *ptp.DelayResp {
	l := binary.Size(ptp.DelayResp{})
	return &ptp.DelayResp{
		Header: ptp.Header{
			SdoIDAndMsgType:    ptp.NewSdoIDAndMsgType(ptp.MessageDelayResp, 0),
			Version:            ptp.Version,
			SequenceID:         uint16(seq),
			MessageLength:      uint16(l),
			FlagField:          ptp.FlagUnicast,
			LogMessageInterval: 0x7f,
		},
		DelayRespBody: ptp.DelayRespBody{
			ReceiveTimestamp: ptp.NewTimestamp(time.Now()),
		},
	}
}

func TestClientRun(t *testing.T) {
	cfg := &Config{
		Address:  "blah",
		Iface:    "ethBlah",
		Timeout:  5 * time.Second,
		Duration: 5 * time.Second,
	}
	// we simply append samples to the array to match what we collected in the end
	history := []*MeasurementResult{}
	c := New(cfg, func(m *MeasurementResult) {
		history = append(history, m)
	})

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	genConn := NewMockUDPConn(ctrl)
	c.genConn = genConn
	// handle whatever client is sending over genConn
	genConn.EXPECT().WriteTo(gomock.Any(), gomock.Any()).DoAndReturn(func(b []byte, _ net.Addr) (int, error) {
		r := bytes.NewReader(b)
		h := &ptp.Header{}
		err := binary.Read(r, binary.BigEndian, h)
		require.Nil(t, err, "reading header")
		require.Equal(t, ptp.MessageSignaling, h.SdoIDAndMsgType.MsgType(), "only expect signaling msgs over genConn")
		signaling := &ptp.Signaling{}
		err = ptp.FromBytes(b, signaling)
		require.Nil(t, err, "reading signaling msg")
		require.Equal(t, 1, len(signaling.TLVs), "expect only 1 TLV in signaling msg")
		tlv := signaling.TLVs[0]
		// we only expect SIGNALING messages where client asks for unicast grants.
		// for each such request we grant it.
		switch v := tlv.(type) {
		case *ptp.RequestUnicastTransmissionTLV:
			msgType := v.MsgTypeAndReserved.MsgType()
			switch msgType {
			case ptp.MessageAnnounce:
				grantAnnounce := grantUnicastPkt(0, c.clockID, c.cfg.Duration, ptp.MessageAnnounce)
				grantAnnounceBytes, err := ptp.Bytes(grantAnnounce)
				require.Nil(t, err)
				c.inChan <- &inPacket{
					data: grantAnnounceBytes,
					ts:   time.Now(),
				}
				return 20, nil
			case ptp.MessageSync:
				grantSync := grantUnicastPkt(1, c.clockID, c.cfg.Duration, ptp.MessageSync)
				grantSyncBytes, err := ptp.Bytes(grantSync)
				require.Nil(t, err)
				c.inChan <- &inPacket{
					data: grantSyncBytes,
					ts:   time.Now(),
				}
				return 20, nil
			case ptp.MessageDelayResp:
				grantDelayResp := grantUnicastPkt(2, c.clockID, c.cfg.Duration, ptp.MessageDelayResp)
				grantDelayRespBytes, err := ptp.Bytes(grantDelayResp)
				require.Nil(t, err)
				c.inChan <- &inPacket{
					data: grantDelayRespBytes,
					ts:   time.Now(),
				}

				announce := announcePkt(3)
				announceBytes, err := ptp.Bytes(announce)
				require.Nil(t, err)
				c.inChan <- &inPacket{
					data: announceBytes,
					ts:   time.Now(),
				}

				sync := syncPkt(4)
				syncBytes, err := ptp.Bytes(sync)
				require.Nil(t, err)
				c.inChan <- &inPacket{
					data: syncBytes,
					ts:   time.Now(),
				}

				fwup := fwupPkt(4)
				fwupBytes, err := ptp.Bytes(fwup)
				require.Nil(t, err)
				c.inChan <- &inPacket{
					data: fwupBytes,
					ts:   time.Now(),
				}
				return 20, nil
			default:
				require.Fail(t, "got unexpected grant", "message type: %s", msgType)
			}
		case *ptp.CancelUnicastTransmissionTLV:
			return 0, nil
		default:
			require.Fail(t, "got unsupported TLV type", "type: %s(%d)", tlv.Type(), tlv.Type())
		}
		return 10, nil
	}).Times(4)

	eventConn := NewMockUDPConnWithTS(ctrl)
	c.eventConn = eventConn

	// handle whatever client is sending over eventConn
	eventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any()).DoAndReturn(func(b []byte, _ net.Addr) (int, time.Time, error) {
		delayReq := &ptp.SyncDelayReq{}
		err := ptp.FromBytes(b, delayReq)
		require.Nil(t, err, "reading delayReq msg")

		delayResp := delayRespPkt(0)
		delayRespBytes, err := ptp.Bytes(delayResp)
		require.Nil(t, err)
		c.inChan <- &inPacket{
			data: delayRespBytes,
			ts:   time.Now(),
		}

		cancelUnicast := cancelUnicastPkt(1, c.clockID, ptp.MessageAnnounce)
		cancelUnicastBytes, err := ptp.Bytes(cancelUnicast)
		require.Nil(t, err)
		c.inChan <- &inPacket{
			data: cancelUnicastBytes,
			ts:   time.Now(),
		}
		return 10, time.Now(), nil
	})

	err := c.runInternal(true)
	require.Nil(t, err, "full client run should succeed")

	assert.Equal(t, 1, len(history), "measurements should be collected by client")
}

func TestClientTimeout(t *testing.T) {
	cfg := &Config{
		Address:  "blah",
		Iface:    "ethBlah",
		Timeout:  500 * time.Millisecond,
		Duration: 500 * time.Millisecond,
	}
	// we simply append samples to the array to match what we collected in the end
	history := []*MeasurementResult{}
	c := New(cfg, func(m *MeasurementResult) {
		history = append(history, m)
	})

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	genConn := NewMockUDPConn(ctrl)
	c.genConn = genConn
	// handle whatever client is sending over genConn
	genConn.EXPECT().WriteTo(gomock.Any(), gomock.Any())

	eventConn := NewMockUDPConnWithTS(ctrl)
	c.eventConn = eventConn

	err := c.runInternal(true)
	require.Error(t, err, "full client run should fail")
	assert.Equal(t, 0, len(history))
}
