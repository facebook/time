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

package linearizability

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/timestamp"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"
)

// ErrGrantDenied is used when GM denied us new grant (server is drained),
// hence we can't proceed with the test
var ErrGrantDenied = errors.New("server denied us grant")

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
}

type udpConnTS struct {
	*net.UDPConn
}

func (c *udpConnTS) WriteToWithTS(b []byte, addr net.Addr) (int, time.Time, error) {
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

// reqUnicast is a helper to build ptp.RequestUnicastTransmission
func reqUnicast(clockID ptp.ClockIdentity, port uint16, duration time.Duration, what ptp.MessageType) *ptp.Signaling {
	l := binary.Size(ptp.Header{}) + binary.Size(ptp.PortIdentity{}) + binary.Size(ptp.RequestUnicastTransmissionTLV{})
	return &ptp.Signaling{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageSignaling, 0),
			Version:         ptp.Version,
			SequenceID:      0, // will be populated on sending
			MessageLength:   uint16(l),
			FlagField:       ptp.FlagUnicast,
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber:    port,
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

// reqDelay is a helper to build ptp.SyncDelayReq
func reqDelay(clockID ptp.ClockIdentity, port uint16, sptp bool) *ptp.SyncDelayReq {
	ptpFlags := ptp.FlagUnicast
	if sptp {
		ptpFlags = ptpFlags | ptp.FlagProfileSpecific1
	}
	return &ptp.SyncDelayReq{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageDelayReq, 0),
			Version:         ptp.Version,
			SequenceID:      0,                                                                       // will be populated on sending
			MessageLength:   uint16(binary.Size(ptp.Header{}) + binary.Size(ptp.SyncDelayReqBody{})), //#nosec G115
			FlagField:       ptpFlags,
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber:    port,
				ClockIdentity: clockID,
			},
			LogMessageInterval: 0x7f,
		},
	}
}

type state int

const (
	stateInit = iota
	stateInProgress
	stateDone
)

var stateToString = map[state]string{
	stateInit:       "INIT",
	stateDone:       "DONE",
	stateInProgress: "IN_PROGRESS",
}

func (s state) String() string {
	return stateToString[s]
}

// inPacket is input packet data + receive timestamp
type inPacket struct {
	data []byte
	ts   time.Time
}

// PTP4lTestResult is what we get after the test run
type PTP4lTestResult struct {
	Server      string
	RXTimestamp time.Time
	TXTimestamp time.Time
	Error       error
}

// Target returns value of server
func (tr PTP4lTestResult) Target() string {
	return tr.Server
}

// Delta is a difference between receiver's RX timestamp and our TX timestamp
func (tr PTP4lTestResult) Delta() time.Duration {
	return tr.RXTimestamp.Sub(tr.TXTimestamp)
}

// Good check if the test passed
func (tr PTP4lTestResult) Good() (bool, error) {
	if tr.Error != nil {
		return false, tr.Error
	}
	// compare stored TX and RX from DelayResp, RX should be after TX
	d := tr.Delta()
	if d <= 0 {
		return false, nil
	}
	return true, nil
}

// Explain provides plain text explanation of linearizability test result
func (tr PTP4lTestResult) Explain() string {
	msg := fmt.Sprintf("linearizability test against %q", tr.Server)
	good, err := tr.Good()
	if good {
		return fmt.Sprintf("%s passed", msg)
	}
	if err != nil {
		return fmt.Sprintf("%s couldn't be completed because of error: %v", msg, tr.Error)
	}
	d := tr.Delta()
	return fmt.Sprintf("%s failed because delta (%v) between RX and TX timestamps is not positive. TX=%v, RX=%v", msg, d, tr.TXTimestamp, tr.RXTimestamp)
}

// Err returns an error value of the PTP4lTestResult
func (tr PTP4lTestResult) Err() error {
	return tr.Error
}

// PTP4lTestConfig is a configuration for Tester
type PTP4lTestConfig struct {
	Timeout   time.Duration
	Server    string
	Interface string
}

// Target sets the server to test
func (p *PTP4lTestConfig) Target(server string) {
	p.Server = server
}

// PTP4lTester is basically a half of PTP unicast client
type PTP4lTester struct {
	clockID         ptp.ClockIdentity
	cfg             *PTP4lTestConfig
	sequence        uint16
	eConn           UDPConnWithTS
	gConn           UDPConn
	eventAddr       *net.UDPAddr
	generalAddr     *net.UDPAddr
	listenerRunning bool

	// port we send/receive event msgs on
	localEventPort uint16
	// chan for received packets regardless of port
	inChan chan *inPacket
	// measurement result
	result *PTP4lTestResult
	// state enum
	state state
	// per sequence
	sendTS map[uint16]time.Time
	// whether to use sptp or IEEE 1588 ptp
	sptp bool
}

// NewPTP4lTester initializes a Tester
func NewPTP4lTester(server string, iface string, useSPTP bool) (*PTP4lTester, error) {
	cfg := &PTP4lTestConfig{
		Timeout:   time.Second,
		Server:    server,
		Interface: iface,
	}

	t := &PTP4lTester{
		inChan: make(chan *inPacket, 10),
		cfg:    cfg,
		sendTS: make(map[uint16]time.Time),
		sptp:   useSPTP,
	}
	if err := t.init(cfg.Interface, cfg.Server); err != nil {
		return nil, err
	}
	return t, nil
}

// Close the connection
func (lt *PTP4lTester) Close() error {
	lt.eConn.Close()
	return lt.gConn.Close()
}

// dedicated function just for logging state changes
func (lt *PTP4lTester) setState(s state) {
	if lt.state != s {
		log.Debugf("Changing state to %s", s)
		lt.state = s
	}
}

func (lt *PTP4lTester) init(ifaceStr, destination string) error {
	// get iface data and clock ID
	iface, err := net.InterfaceByName(ifaceStr)
	if err != nil {
		return err
	}

	cid, err := ptp.NewClockIdentity(iface.HardwareAddr)
	if err != nil {
		return err
	}
	lt.clockID = cid

	// addresses
	// where to send to
	eventAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(destination, fmt.Sprintf("%d", ptp.PortEvent)))
	if err != nil {
		return err
	}
	lt.eventAddr = eventAddr
	generalAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(destination, fmt.Sprintf("%d", ptp.PortGeneral)))
	if err != nil {
		return err
	}
	lt.generalAddr = generalAddr
	// bind
	gConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("::"), Port: 0})
	if err != nil {
		return err
	}
	lt.gConn = gConn
	eConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("::"), Port: 0})
	if err != nil {
		return err
	}
	lt.eConn = &udpConnTS{eConn}
	local := eConn.LocalAddr()
	lt.localEventPort = uint16(local.(*net.UDPAddr).Port)
	log.Debugf("listening on %v", local)

	// get FD of the connection. Can be optimized by doing this when connection is created
	connFd, err := timestamp.ConnFd(eConn)
	if err != nil {
		return err
	}

	// we need to enable HW or SW timestamps on event port
	if err := timestamp.EnableHWTimestamps(connFd, ifaceStr); err != nil {
		return fmt.Errorf("failed to enable hardware timestamps on port %d: %w", lt.localEventPort, err)
	}

	// set it to blocking mode, otherwise recvmsg will just return with nothing most of the time
	if err := unix.SetNonblock(connFd, false); err != nil {
		return fmt.Errorf("failed to set event socket to blocking: %w", err)
	}
	return nil
}

func (lt *PTP4lTester) sendEventMsg(p ptp.Packet) (uint16, time.Time, error) {
	seq := lt.sequence
	p.SetSequence(lt.sequence)
	b, err := ptp.Bytes(p)
	if err != nil {
		return 0, time.Time{}, err
	}
	// send packet
	_, hwts, err := lt.eConn.WriteToWithTS(b, lt.eventAddr)
	if err != nil {
		return 0, time.Time{}, err
	}
	lt.sequence++

	log.Debugf("sent packet to %v", lt.eventAddr)
	return seq, hwts, nil
}

func (lt *PTP4lTester) sendGeneralMsg(p ptp.Packet) (uint16, error) {
	seq := lt.sequence
	p.SetSequence(lt.sequence)
	b, err := ptp.Bytes(p)
	if err != nil {
		return 0, err
	}
	// send packet
	_, err = lt.gConn.WriteTo(b, lt.generalAddr)
	if err != nil {
		return 0, err
	}
	lt.sequence++

	log.Debugf("sent packet to %v", lt.generalAddr)
	return seq, nil
}

func (lt *PTP4lTester) sendDelay() error {
	// form DelayReq, set OriginTimestamp to PHC time
	delayReq := reqDelay(lt.clockID, lt.localEventPort, lt.sptp)
	// send DelayReq, store TX ts
	seq, hwts, err := lt.sendEventMsg(delayReq)
	if err != nil {
		return fmt.Errorf("sending delay req: %w", err)
	}
	log.Debugf("sent msg #%d at %v", seq, hwts)
	lt.sendTS[seq] = hwts
	return nil
}

func (lt *PTP4lTester) handleMsg(msg *inPacket) error {
	msgType, err := ptp.ProbeMsgType(msg.data)
	if err != nil {
		return err
	}
	switch msgType {
	case ptp.MessageSignaling:
		signaling := &ptp.Signaling{}
		if err := ptp.FromBytes(msg.data, signaling); err != nil {
			return fmt.Errorf("reading signaling msg: %w", err)
		}

		for _, tlv := range signaling.TLVs {
			switch v := tlv.(type) {
			case *ptp.GrantUnicastTransmissionTLV:
				tlvType := v.MsgTypeAndReserved.MsgType()
				switch tlvType {
				case ptp.MessageDelayResp:
					if v.DurationField == 0 {
						return fmt.Errorf("%w for %v", ErrGrantDenied, tlvType)
					}
					log.Debug("got unicast grant for Delay Response")
					if err := lt.sendDelay(); err != nil {
						return err
					}
				default:
					log.Warningf("got unexpected grant for %s", tlvType)
				}

			case *ptp.CancelUnicastTransmissionTLV:
				log.Debugf("got unicast transmission cancellation for %s", v.MsgTypeAndFlags.MsgType())
			default:
				return fmt.Errorf("got unsupported TLV type %s(%d)", tlv.Type(), tlv.Type())
			}
		}
		return nil
	case ptp.MessageDelayResp:
		ptpMsg := &ptp.DelayResp{}
		if err := ptp.FromBytes(msg.data, ptpMsg); err != nil {
			return fmt.Errorf("reading delay_resp msg: %w", err)
		}
		log.Debugf("got delayResp: %+v", ptpMsg)
		return lt.processTimestamp(ptpMsg.SequenceID, ptpMsg.ReceiveTimestamp.Time())
	case ptp.MessageSync:
		ptpMsg := &ptp.SyncDelayReq{}
		if err := ptp.FromBytes(msg.data, ptpMsg); err != nil {
			return fmt.Errorf("reading sync msg: %w", err)
		}
		log.Debugf("got SYNC: %+v", ptpMsg)
		return lt.processTimestamp(ptpMsg.SequenceID, ptpMsg.OriginTimestamp.Time())
	case ptp.MessageAnnounce:
		log.Debug("Ignoring ANNOUNCE")
	default:
		log.Errorf("got unexpected packet %v", msgType)
	}
	return nil
}

// runListener starts incoming packet listener.
// It's meant to be run in a goroutine before issuing calls to RunTest.
func (lt *PTP4lTester) runListener(ctx context.Context) {
	listen := func(conn UDPConn, expectedAddr net.IP) {
		for {
			response := make([]uint8, 1024)
			n, addr, err := conn.ReadFromUDP(response)
			if err != nil {
				log.Errorf("receiver err:%v", err)
				continue
			}
			if !addr.IP.Equal(expectedAddr) {
				log.Warningf("ignoring packets from server %v", addr)
				continue
			}
			lt.inChan <- &inPacket{data: response[:n]}
		}
	}
	// it's done in non-blocking way, so if context is cancelled we exit correctly
	go listen(lt.gConn, lt.generalAddr.IP)
	go listen(lt.eConn, lt.eventAddr.IP)

	lt.listenerRunning = true
	<-ctx.Done()
	log.Debug("cancelled port receiver")
}

// runSingleTest performs one Tester run and will exit on completion.
// The run consists of:
// * sending the Unicast DelayResponse subscription on the receiver, if subDuration is not zero and not using sptp
// * sending one DelayRequest (using sptp if lt.sptp is true)
// * receiving one DelayResponse
// The result of the test will be stored in the lt.result variable, unless error was returned.
func (lt *PTP4lTester) runSingleTest(ctx context.Context, subDuration time.Duration) error {
	if !lt.listenerRunning {
		go lt.runListener(ctx)
	}

	lt.setState(stateInit)
	var err error
	ctx, cancel := context.WithTimeout(ctx, lt.cfg.Timeout)
	defer cancel()
	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				log.Debug("cancelled main loop")
				return ctx.Err()
			case msg := <-lt.inChan:
				if err := lt.handleMsg(msg); err != nil {
					return fmt.Errorf("handling incoming packet: %w", err)
				}
			default:
				switch lt.state {
				case stateInit:
					// reset the result
					lt.result = nil
					if subDuration != 0 && !lt.sptp {
						// request subscription
						reqDelayResp := reqUnicast(lt.clockID, lt.localEventPort, subDuration, ptp.MessageDelayResp)
						_, err = lt.sendGeneralMsg(reqDelayResp)
						if err != nil {
							return fmt.Errorf("sending delay response subscription request: %w", err)
						}
						// we will send delay req on receipt of the grant
					} else {
						// just send the delay req assuming we have a subscription
						if err := lt.sendDelay(); err != nil {
							return fmt.Errorf("sending delay request: %w", err)
						}
					}
					lt.setState(stateInProgress)
				case stateDone:
					cancel()
					return nil
				}
			}
		}
	})

	return eg.Wait()
}

func simplifyIPv6(ipStr string) string {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return ""
	}
	return ip.String()
}

// RunTest performs one Tester run and will exit on completion.
// The result of the test will be returned, including any error arising during the test.
// If sptp is true, sptp is used. Otherwise, IEEE 1588 ptp is used
func (lt *PTP4lTester) RunTest(ctx context.Context) TestResult {
	if !lt.listenerRunning {
		go lt.runListener(ctx)
	}

	result := PTP4lTestResult{
		Server: lt.cfg.Server,
	}
	log.Debugf("test starting %s", simplifyIPv6(lt.cfg.Server))
	err := lt.runSingleTest(ctx, 0)
	log.Debugf("test done %s", simplifyIPv6(lt.cfg.Server))
	// re-run with subscription request. Do not use subscription mechanism with sptp
	if errors.Is(err, context.DeadlineExceeded) && !lt.sptp {
		log.Debugf("re-running timed out test with subscription renewal %s", simplifyIPv6(lt.cfg.Server))
		err = lt.runSingleTest(ctx, time.Minute)
		log.Debugf("test done %s", simplifyIPv6(lt.cfg.Server))
	}
	if lt.result != nil {
		result = *lt.result
	}
	if err != nil {
		result.Error = err
		if !errors.Is(err, context.Canceled) {
			log.Debugf("test against %s error: %v", simplifyIPv6(lt.cfg.Server), err)
		}
	}
	return result
}

func (lt *PTP4lTester) processTimestamp(sequenceID uint16, rxTimestamp time.Time) error {
	sendTS, found := lt.sendTS[sequenceID]
	if !found {
		expected := []uint16{}
		for e := range lt.sendTS {
			expected = append(expected, e)
		}
		return fmt.Errorf("unexpected sequence %d, expected one of %v", sequenceID, expected)
	}
	delete(lt.sendTS, sequenceID)
	log.Debugf("we sent packet at %v", sendTS)
	log.Debugf("it was received at %v", rxTimestamp)
	log.Debugf("difference RX - TX = %v", rxTimestamp.Sub(sendTS))
	lt.result = &PTP4lTestResult{
		Server:      lt.cfg.Server,
		TXTimestamp: sendTS,
		RXTimestamp: rxTimestamp,
	}
	lt.setState(stateDone)
	return nil
}
