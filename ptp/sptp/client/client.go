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
	"fmt"
	rnd "math/rand"
	"net"
	"time"

	"github.com/fatih/color"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	ptp "github.com/facebook/time/ptp/protocol"
)

// corrToDuration converts PTP CorrectionField to time.Duration, ignoring
// case where correction is too big, and dropping fractions of nanoseconds
func corrToDuration(correction ptp.Correction) (corr time.Duration) {
	if !correction.TooBig() {
		corr = time.Duration(correction.Nanoseconds())
	}
	return
}

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
	Server      string
	Measurement *MeasurementResult
	Error       error
}

// InPacket is input packet data + receive timestamp
type InPacket struct {
	data []byte
	ts   time.Time
}

// NewInPacket returns a new instance of InPacket
func NewInPacket(data []byte, ts time.Time) *InPacket {
	return &InPacket{data: data, ts: ts}
}

// Data returns data
func (i *InPacket) Data() []byte {
	return i.data
}

// TS returns timestamp
func (i *InPacket) TS() time.Time {
	return i.ts
}

// Client is a part of PTPNG that talks to only one server
type Client struct {
	server string
	// packet sequence counter
	eventSequence uint16
	// mask for sequence ID value
	sequenceIDMask uint16
	// const value for sequence ID
	sequenceIDValue uint16
	// chan for received packets regardless of port
	inChan chan *InPacket
	// listening connection on port 319
	eventConn UDPConnWithTS
	// our clockID derived from MAC address
	clockID ptp.ClockIdentity

	eventAddr *net.UDPAddr

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
func (c *Client) SendEventMsg(p ptp.Packet) (uint16, time.Time, error) {
	seq := c.eventSequence
	p.SetSequence(c.eventSequence)
	b, err := ptp.Bytes(p)
	if err != nil {
		return 0, time.Time{}, err
	}
	// send packet
	_, hwts, err := c.eventConn.WriteToWithTS(b, c.eventAddr)

	c.incrementSequence()
	if err != nil {
		log.Warnf("Error sending packet with SeqID = %04x: %v", seq, err)
		return 0, time.Time{}, err
	}

	log.Debugf("sent packet to %v", c.eventAddr)
	return seq, hwts, nil
}

// NewClient initializes sptp client
func NewClient(target string, targetPort int, clockID ptp.ClockIdentity, eventConn UDPConnWithTS, cfg *Config, stats StatsServer) (*Client, error) {
	// addresses
	// where to send to
	eventAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(target, fmt.Sprintf("%d", targetPort)))
	if err != nil {
		return nil, err
	}
	sequenceIDMask, sequenceIDMaskedValue := cfg.GenerateMaskAndValue()
	c := &Client{
		clockID:         clockID,
		eventSequence:   uint16(rnd.Int31n(65536)) & sequenceIDMask,
		sequenceIDMask:  sequenceIDMask,
		sequenceIDValue: sequenceIDMaskedValue,
		eventConn:       eventConn,
		eventAddr:       eventAddr,
		inChan:          make(chan *InPacket, 100),
		server:          target,
		m:               newMeasurements(&cfg.Measurement),
		stats:           stats,
	}
	return c, nil
}

// dispatch handler based on msg type
func (c *Client) handleMsg(msg *InPacket) error {
	msgType, err := ptp.ProbeMsgType(msg.data)
	if err != nil {
		return err
	}
	switch msgType {
	case ptp.MessageAnnounce:
		announce := &ptp.Announce{}
		if err := ptp.FromBytes(msg.data, announce); err != nil {
			return fmt.Errorf("reading announce msg: %w", err)
		}
		c.stats.IncRXAnnounce()
		return c.handleAnnounce(announce)
	case ptp.MessageSync:
		b := &ptp.SyncDelayReq{}
		if err := ptp.FromBytes(msg.data, b); err != nil {
			return fmt.Errorf("reading sync msg: %w", err)
		}
		c.stats.IncRXSync()
		return c.handleSync(b, msg.ts)
	default:
		c.logReceive(msgType, "unsupported, ignoring")
		c.stats.IncUnsupported()
		return nil
	}
}

// couple of helpers to log nice lines about happening communication
func (c *Client) logSent(t ptp.MessageType, msg string, v ...interface{}) {
	log.Debugf(color.GreenString("[%s] client -> %s (%s)", c.server, t, fmt.Sprintf(msg, v...)))
}
func (c *Client) logReceive(t ptp.MessageType, msg string, v ...interface{}) {
	log.Debugf(color.BlueString("[%s] server -> %s (%s)", c.server, t, fmt.Sprintf(msg, v...)))
}

// handleAnnounce handles ANNOUNCE packet and records UTC offset from it's data
func (c *Client) handleAnnounce(b *ptp.Announce) error {
	c.logReceive(ptp.MessageAnnounce, "seq=%d, T1=%v, CF2=%v, gmIdentity=%s, gmTimeSource=%s, stepsRemoved=%d",
		b.SequenceID, b.OriginTimestamp.Time(), corrToDuration(b.CorrectionField), b.GrandmasterIdentity, b.TimeSource, b.StepsRemoved)
	c.m.currentUTCoffset = time.Duration(b.CurrentUTCOffset) * time.Second
	// announce carries T1 and CF2
	c.m.addT1(b.SequenceID, b.OriginTimestamp.Time())
	c.m.addCF2(b.SequenceID, corrToDuration(b.CorrectionField))
	c.m.addAnnounce(*b)
	return nil
}

// handleSync handles SYNC packet and adds send timestamp to measurements
func (c *Client) handleSync(b *ptp.SyncDelayReq, ts time.Time) error {
	c.logReceive(ptp.MessageSync, "seq=%d, T2=%v, T4=%v, CF1=%v", b.SequenceID, ts, b.OriginTimestamp.Time(), corrToDuration(b.CorrectionField))
	// T2 and CF1
	c.m.addT2andCF1(b.SequenceID, ts, corrToDuration(b.CorrectionField))
	// sync carries T4 as well
	c.m.addT4(b.SequenceID, b.OriginTimestamp.Time())
	return nil
}

// handleDelayReq handles Delay Reqest packet and responds with SYNC
// It's used for external pingers such as ptping and not required for sptp itself
func (c *Client) handleDelayReq(ts time.Time) error {
	sync := ReqDelay(c.clockID, 1)
	sync.OriginTimestamp = ptp.NewTimestamp(ts)
	_, txts, err := c.SendEventMsg(sync)
	if err != nil {
		return err
	}
	announce := ReqAnnounce(c.clockID, 1, txts)
	_, _, err = c.SendEventMsg(announce)
	return err
}

// RunOnce produces one client-server exchange
func (c *Client) RunOnce(ctx context.Context, timeout time.Duration) *RunResult {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	eg, ctx := errgroup.WithContext(ctx)

	result := RunResult{
		Server: c.server,
	}
	c.m.cleanup()

	eg.Go(func() error {
		// ask for delay
		seq, hwts, err := c.SendEventMsg(ReqDelay(c.clockID, 1))
		if err != nil {
			return err
		}
		c.m.addT3(seq, hwts)
		c.logSent(ptp.MessageDelayReq, "seq=%d, our T3=%v", seq, hwts)
		c.stats.IncTXDelayReq()

		for {
			select {
			case <-ctx.Done():
				log.Debugf("cancelled main loop")
				return ctx.Err()
			case msg := <-c.inChan:
				if err := c.handleMsg(msg); err != nil {
					return err
				}
				latest, err := c.m.latest()

				if err != nil {
					log.Debugf("getting latest measurement: %v", err)
					if !errors.Is(err, errNotEnoughData) {
						return err
					}
				} else {
					log.Debugf("latest measurement: %+v", latest)
					result.Measurement = latest
					return nil
				}
			}
		}
	})
	result.Error = eg.Wait()

	return &result
}
