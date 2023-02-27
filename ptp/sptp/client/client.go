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
	"net"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/sptp/stats"
	"github.com/facebook/time/timestamp"
)

// re-export timestamping
const (
	// HWTIMESTAMP is a hardware timestamp
	HWTIMESTAMP = timestamp.HWTIMESTAMP
	// SWTIMESTAMP is a software timestamp
	SWTIMESTAMP = timestamp.SWTIMESTAMP
)

// UDPConn describes what functionality we expect from UDP connection
type UDPConn interface {
	ReadFromUDP(b []byte) (int, *net.UDPAddr, error)
	WriteTo(b []byte, addr net.Addr) (int, error)
	Close() error
}

// UDPConnWithTS describes what functionality we expect from UDP connection that allows us to read TX timestamps
type UDPConnWithTS interface {
	UDPConn
	WriteToWithTS(b []byte, addr net.Addr) (int, time.Time, error)
	ReadPacketWithRXTimestamp() ([]byte, unix.Sockaddr, time.Time, error)
}

type udpConnTS struct {
	*net.UDPConn
	l sync.Mutex
}

func newUDPConnTS(conn *net.UDPConn) *udpConnTS {
	return &udpConnTS{
		UDPConn: conn,
	}
}

func (c *udpConnTS) WriteToWithTS(b []byte, addr net.Addr) (int, time.Time, error) {
	c.l.Lock()
	defer c.l.Unlock()
	n, err := c.WriteTo(b, addr)
	if err != nil {
		return 0, time.Time{}, err
	}
	// get FD of the connection. Can be optimized by doing this when connection is created
	connFd, err := timestamp.ConnFd(c.UDPConn)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to get conn fd udp connection: %w", err)
	}
	hwts, _, err := timestamp.ReadTXtimestamp(connFd)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("failed to get timestamp of last packet: %w", err)
	}
	return n, hwts, nil
}

func (c *udpConnTS) ReadPacketWithRXTimestamp() ([]byte, unix.Sockaddr, time.Time, error) {
	// get FD of the connection. Can be optimized by doing this when connection is created
	connFd, err := timestamp.ConnFd(c.UDPConn)
	if err != nil {
		return nil, nil, time.Time{}, fmt.Errorf("failed to get conn fd udp connection: %w", err)
	}
	return timestamp.ReadPacketWithRXTimestamp(connFd)
}

// corrToDuration converts PTP CorrectionField to time.Duration, ignoring
// case where correction is too big, and dropping fractions of nanoseconds
func corrToDuration(correction ptp.Correction) (corr time.Duration) {
	if !correction.TooBig() {
		corr = time.Duration(correction.Nanoseconds())
	}
	return
}

// reqDelay is a helper to build ptp.SyncDelayReq
func reqDelay(clockID ptp.ClockIdentity) *ptp.SyncDelayReq {
	return &ptp.SyncDelayReq{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageDelayReq, 0),
			Version:         ptp.Version,
			SequenceID:      0, // will be populated on sending
			MessageLength:   uint16(binary.Size(ptp.SyncDelayReq{})),
			FlagField:       ptp.FlagUnicast | ptp.FlagProfileSpecific1,
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber:    1,
				ClockIdentity: clockID,
			},
			LogMessageInterval: 0x7f,
		},
	}
}

// RunResult is what we return from single client-server interaction
type RunResult struct {
	Server      string
	Measurement *MeasurementResult
	Error       error
}

// inPacket is input packet data + receive timestamp
type inPacket struct {
	data []byte
	ts   time.Time
}

// Client is a part of PTPNG that talks to only one server
type Client struct {
	server string
	// packet sequence counter
	eventSequence uint16

	// chan for received packets regardless of port
	inChan chan *inPacket
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

func (c *Client) sendEventMsg(p ptp.Packet) (uint16, time.Time, error) {
	seq := c.eventSequence
	p.SetSequence(c.eventSequence)
	b, err := ptp.Bytes(p)
	if err != nil {
		return 0, time.Time{}, err
	}
	// send packet
	_, hwts, err := c.eventConn.WriteToWithTS(b, c.eventAddr)
	if err != nil {
		return 0, time.Time{}, err
	}
	c.eventSequence++

	log.Debugf("sent packet via port %d to %v", ptp.PortEvent, c.eventAddr)
	return seq, hwts, nil
}

// newClient initializes sptp client
func newClient(target string, clockID ptp.ClockIdentity, eventConn UDPConnWithTS, mcfg *MeasurementConfig, stats StatsServer) (*Client, error) {
	// addresses
	// where to send to
	eventAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(target, fmt.Sprintf("%d", ptp.PortEvent)))
	if err != nil {
		return nil, err
	}
	c := &Client{
		clockID:   clockID,
		eventConn: eventConn,
		eventAddr: eventAddr,
		inChan:    make(chan *inPacket, 100),
		server:    target,
		m:         newMeasurements(mcfg),
		stats:     stats,
	}
	return c, nil
}

// dispatch handler based on msg type
func (c *Client) handleMsg(msg *inPacket) error {
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
		c.stats.UpdateCounterBy(fmt.Sprintf("%s%s", stats.PortStatsRxPrefix, strings.ToLower(msgType.String())), 1)
		return c.handleAnnounce(announce)
	case ptp.MessageSync:
		b := &ptp.SyncDelayReq{}
		if err := ptp.FromBytes(msg.data, b); err != nil {
			return fmt.Errorf("reading sync msg: %w", err)
		}
		c.stats.UpdateCounterBy(fmt.Sprintf("%s%s", stats.PortStatsRxPrefix, strings.ToLower(msgType.String())), 1)
		return c.handleSync(b, msg.ts)
	default:
		c.logReceive(msgType, "unsupported, ignoring")
		c.stats.UpdateCounterBy(fmt.Sprintf("%s%s", stats.PortStatsRxPrefix, strings.ToLower(msgType.String())), 1)
		c.stats.UpdateCounterBy("ptp.sptp.portstats.rx.unsupported", 1)
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

// RunOnce produces one client-server exchange
func (c *Client) RunOnce(ctx context.Context, timeout time.Duration) *RunResult {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	eg, ctx := errgroup.WithContext(ctx)

	result := RunResult{
		Server: c.server,
	}

	eg.Go(func() error {
		// ask for delay
		seq, hwts, err := c.sendEventMsg(reqDelay(c.clockID))
		if err != nil {
			return err
		}
		c.m.addT3(seq, hwts)
		c.logSent(ptp.MessageDelayReq, "seq=%d, our T3=%v", seq, hwts)
		c.stats.UpdateCounterBy(fmt.Sprintf("%s%s", stats.PortStatsTxPrefix, strings.ToLower(ptp.MessageDelayReq.String())), 1)

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
					c.m.cleanup(latest.Timestamp, time.Minute)
					result.Measurement = latest
					return nil
				}
			}
		}
	})
	result.Error = eg.Wait()

	return &result
}
