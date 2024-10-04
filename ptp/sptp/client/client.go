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

package client

import (
	"context"
	"encoding/binary"
	"errors"
	rnd "math/rand"
	"net/netip"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/timestamp"
)

// ReqDelay is a helper to build ptp.SyncDelayReq
func ReqDelay(clockID ptp.ClockIdentity, portID uint16) *ptp.SyncDelayReq {
	return &ptp.SyncDelayReq{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageDelayReq, 0),
			Version:         ptp.Version,
			SequenceID:      0, // will be populated on sending
			MessageLength:   uint16(binary.Size(ptp.SyncDelayReq{})),
			FlagField:       ptp.FlagUnicast | ptp.FlagProfileSpecific1,
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber:    portID,
				ClockIdentity: clockID,
			},
			LogMessageInterval: 0x7f,
		},
	}
}

// ReqAnnounce is a helper to build ptp.Announce
// It's used for external pingers such as ptping and not required for sptp itself
func ReqAnnounce(clockID ptp.ClockIdentity, portID uint16, ts time.Time) *ptp.Announce {
	return &ptp.Announce{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageAnnounce, 0),
			Version:         ptp.Version,
			SequenceID:      0, // will be populated on sending
			MessageLength:   uint16(binary.Size(ptp.Header{}) + binary.Size(ptp.AnnounceBody{})),
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber:    portID,
				ClockIdentity: clockID,
			},
			LogMessageInterval: 0x7f,
		},
		AnnounceBody: ptp.AnnounceBody{
			OriginTimestamp: ptp.NewTimestamp(ts),
		},
	}
}

// RunResult is what we return from single client-server interaction
type RunResult struct {
	Server      netip.Addr
	Measurement *MeasurementResult
	Error       error
}

// Client is a part of PTPNG that talks to only one server
type Client struct {
	server netip.Addr
	// packet sequence counter
	eventSequence uint16
	// mask for sequence ID value
	sequenceIDMask uint16
	// const value for sequence ID
	sequenceIDValue uint16
	// chan for received packets event regardless of port
	inChan chan bool
	// listening connection on port 319
	eventConn UDPConnWithTS
	// outgoing delay request packet
	delayRequest *ptp.SyncDelayReq
	// outgoing packet bytes buffer
	delayReqBytes []byte

	eventAddr unix.Sockaddr

	// where we store timestamps
	m *measurements

	// where we store our metrics
	stats StatsServer
}

func (c *Client) incrementSequence() {
	c.eventSequence++
	c.eventSequence = c.sequenceIDValue + (c.eventSequence & c.sequenceIDMask)
}

// SendEventMsg sends an event message via event socket
func (c *Client) SendEventMsg(p *ptp.SyncDelayReq) (uint16, time.Time, error) {
	seq := c.eventSequence
	p.SetSequence(c.eventSequence)
	_, err := ptp.BytesTo(p, c.delayReqBytes)
	if err != nil {
		return 0, time.Time{}, err
	}
	// send packet
	_, hwts, err := c.eventConn.WriteToWithTS(c.delayReqBytes, c.eventAddr)

	c.incrementSequence()
	if err != nil {
		log.Warnf("Error sending packet with SeqID = %04x: %v", seq, err)
		return 0, time.Time{}, err
	}

	log.Debugf("sent packet to %v", c.eventAddr)
	return seq, hwts, nil
}

// SendAnnounce sends an announce message via event socket
// It's used for external pingers such as ptping and not required for sptp itself
func (c *Client) SendAnnounce(p *ptp.Announce) (uint16, error) {
	seq := c.eventSequence
	p.SetSequence(c.eventSequence)
	b, err := ptp.Bytes(p)
	if err != nil {
		return 0, err
	}
	// send packet
	// since client only has the event conn we have to read the TS
	_, _, err = c.eventConn.WriteToWithTS(b, c.eventAddr)

	c.incrementSequence()
	if err != nil {
		log.Warnf("Error sending packet with SeqID = %04x: %v", seq, err)
		return 0, err
	}

	log.Debugf("sent packet to %v", c.eventAddr)
	return seq, nil
}

// NewClient initializes sptp client
func NewClient(target netip.Addr, targetPort int, clockID ptp.ClockIdentity, eventConn UDPConnWithTS, cfg *Config, stats StatsServer) (*Client, error) {
	// where to send to
	eventAddr := timestamp.AddrToSockaddr(target, targetPort)
	sequenceIDMask, sequenceIDMaskedValue := cfg.GenerateMaskAndValue()
	c := &Client{
		eventSequence:   uint16(rnd.Int31n(65536)) & sequenceIDMask,
		sequenceIDMask:  sequenceIDMask,
		sequenceIDValue: sequenceIDMaskedValue,
		delayRequest:    ReqDelay(clockID, 1),
		delayReqBytes:   make([]byte, binary.Size(ptp.Header{})+binary.Size(ptp.SyncDelayReq{})),
		eventConn:       eventConn,
		eventAddr:       eventAddr,
		inChan:          make(chan bool, 100),
		server:          target,
		m:               newMeasurements(&cfg.Measurement),
		stats:           stats,
	}
	return c, nil
}

// handleAnnounce handles ANNOUNCE packet and records UTC offset from it's data
func (c *Client) handleAnnounce(b *ptp.Announce) {
	t1 := b.OriginTimestamp.Time()
	cf := b.CorrectionField.Duration()
	log.Debugf("[%s] server -> %s (seq=%d, T1=%v, CF2=%v, gmIdentity=%s, gmTimeSource=%s, stepsRemoved=%d)",
		c.server,
		ptp.MessageAnnounce,
		b.SequenceID,
		t1,
		cf,
		b.GrandmasterIdentity,
		b.TimeSource,
		b.StepsRemoved)
	c.m.currentUTCoffset = time.Duration(b.CurrentUTCOffset) * time.Second
	// announce carries T1 and CF2
	c.m.addT1(b.SequenceID, t1)
	c.m.addCF2(b.SequenceID, cf)
	c.m.addAnnounce(*b)
}

// handleSync handles SYNC packet and adds send timestamp to measurements
func (c *Client) handleSync(b *ptp.SyncDelayReq, ts time.Time) {
	t4 := b.OriginTimestamp.Time()
	cf := b.CorrectionField.Duration()
	log.Debugf("[%s] server -> %s (seq=%d, T2=%v, T4=%v, CF1=%v)",
		c.server,
		ptp.MessageSync,
		b.SequenceID,
		ts,
		t4,
		cf)
	// T2 and CF1
	c.m.addT2andCF1(b.SequenceID, ts, cf)
	// sync carries T4 as well
	c.m.addT4(b.SequenceID, t4)
}

// handleDelayReq handles Delay Reqest packet and responds with SYNC
// It's used for external pingers such as ptping and not required for sptp itself
func (c *Client) handleDelayReq(clockID ptp.ClockIdentity, ts time.Time) error {
	sync := ReqDelay(clockID, 1)
	sync.OriginTimestamp = ptp.NewTimestamp(ts)
	_, txts, err := c.SendEventMsg(sync)
	if err != nil {
		return err
	}
	announce := ReqAnnounce(clockID, 1, txts)
	_, err = c.SendAnnounce(announce)
	return err
}

// RunOnce produces one client-server exchange
func (c *Client) RunOnce(ctx context.Context, timeout time.Duration) *RunResult {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	errchan := make(chan error)

	result := &RunResult{
		Server: c.server,
	}
	c.m.cleanup()

	go func() {
		defer close(errchan)
		// ask for delay
		seq, hwts, err := c.SendEventMsg(c.delayRequest)
		if err != nil {
			errchan <- err
			return
		}
		c.m.addT3(seq, hwts)
		log.Debugf("[%s] client -> %s (seq=%d, our T3=%v)", c.server, ptp.MessageDelayReq, seq, hwts)
		c.stats.IncTXDelayReq()

		for {
			select {
			case <-ctx.Done():
				log.Debugf("[%s] cancelled routine", c.server)
				errchan <- ctx.Err()
				return
			case <-c.inChan:
				latest, err := c.m.latest()
				if err != nil {
					log.Debugf("[%s] getting latest measurement: %v", c.server, err)
					if !errors.Is(err, errNotEnoughData) {
						errchan <- err
						return
					}
				} else {
					log.Debugf("[%s] latest measurement: %+v", c.server, latest)
					result.Measurement = latest
					errchan <- nil
					return
				}
			}
		}
	}()

	select {
	case err := <-errchan:
		result.Error = err
	case <-ctx.Done():
		if result.Measurement == nil {
			result.Error = ctx.Err()
		}
	}

	return result
}
