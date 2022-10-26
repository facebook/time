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
func reqDelay(clockID ptp.ClockIdentity, port uint16) *ptp.SyncDelayReq {
	return &ptp.SyncDelayReq{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(ptp.MessageDelayReq, 0),
			Version:         ptp.Version,
			SequenceID:      0, // will be populated on sending
			MessageLength:   uint16(binary.Size(ptp.SyncDelayReq{})),
			FlagField:       ptp.FlagUnicast,
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

// TestResult is what we get after the test run
type TestResult struct {
	Server      string
	RXTimestamp time.Time
	TXTimestamp time.Time
	Error       error
}

// Delta is a difference between receiver's RX timestamp and our TX timestamp
func (tr *TestResult) Delta() time.Duration {
	return tr.RXTimestamp.Sub(tr.TXTimestamp)
}

// Good check if the test passed
func (tr *TestResult) Good() (bool, error) {
	if tr == nil {
		return false, fmt.Errorf("no data")
	}
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
func (tr *TestResult) Explain() string {
	if tr == nil {
		return "linearizability test is nil"
	}
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

// TestConfig is a configuration for Tester
type TestConfig struct {
	Timeout   time.Duration
	Server    string
	Interface string
}

// Tester is basically a half of PTP unicast client
type Tester struct {
	clockID  ptp.ClockIdentity
	cfg      *TestConfig
	sequence uint16

	eConn       UDPConnWithTS
	gConn       UDPConn
	eventAddr   *net.UDPAddr
	generalAddr *net.UDPAddr
	// port we send/receive event msgs on
	localEventPort uint16
	// chan for received packets regardless of port
	inChan chan *inPacket
	// measurement result
	result *TestResult
	// state enum
	state state
	// per sequence
	sendTS map[uint16]time.Time
}

// NewTester initializes a Tester
func NewTester(cfg *TestConfig) (*Tester, error) {
	t := &Tester{
		inChan: make(chan *inPacket, 10),
		cfg:    cfg,
		sendTS: make(map[uint16]time.Time),
	}
	if err := t.init(cfg.Interface, cfg.Server); err != nil {
		return nil, err
	}
	return t, nil
}

// Close the connection
func (lt *Tester) Close() error {
	lt.eConn.Close()
	return lt.gConn.Close()
}

// dedicated function just for logging state changes
func (lt *Tester) setState(s state) {
	if lt.state != s {
		log.Debugf("Changing state to %s", s)
		lt.state = s
	}
}

func (lt *Tester) init(ifaceStr, destination string) error {
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

func (lt *Tester) sendEventMsg(p ptp.Packet) (uint16, time.Time, error) {
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

func (lt *Tester) sendGeneralMsg(p ptp.Packet) (uint16, error) {
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

func (lt *Tester) sendDelay() error {
	// form DelayReq, set OriginTimestamp to PHC time
	delayReq := reqDelay(lt.clockID, lt.localEventPort)
	// send DelayReq, store TX ts
	seq, hwts, err := lt.sendEventMsg(delayReq)
	if err != nil {
		return fmt.Errorf("sending delay req: %w", err)
	}
	log.Debugf("sent msg #%d at %v", seq, hwts)
	lt.sendTS[seq] = hwts
	return nil
}

func (lt *Tester) handleMsg(msg *inPacket) error {
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
				msgType := v.MsgTypeAndReserved.MsgType()
				switch msgType {
				case ptp.MessageDelayResp:
					if v.DurationField == 0 {
						return fmt.Errorf("%w for %v", ErrGrantDenied, msgType)
					}
					log.Debugf("got unicast grant for Delay Response")
					if err := lt.sendDelay(); err != nil {
						return err
					}
				default:
					log.Warningf("got unexpected grant for %s", msgType)
				}

			case *ptp.CancelUnicastTransmissionTLV:
				log.Debugf("got unicast transmission cancellation for %s", v.MsgTypeAndFlags.MsgType())
			default:
				return fmt.Errorf("got unsupported TLV type %s(%d)", tlv.Type(), tlv.Type())
			}
		}
		return nil
	case ptp.MessageDelayResp:
		b := &ptp.DelayResp{}
		if err := ptp.FromBytes(msg.data, b); err != nil {
			return fmt.Errorf("reading delay_resp msg: %w", err)
		}
		log.Debugf("got delayResp: %+v", b)
		sendTS, found := lt.sendTS[b.SequenceID]
		if !found {
			expected := []uint16{}
			for e := range lt.sendTS {
				expected = append(expected, e)
			}
			return fmt.Errorf("unexpected DelayResp sequence %d, expected one of %v", b.SequenceID, expected)
		}
		delete(lt.sendTS, b.SequenceID)
		log.Debugf("we sent packet at %v", sendTS)
		log.Debugf("it was received at %v", b.ReceiveTimestamp)
		log.Debugf("difference RX - TX = %v", b.ReceiveTimestamp.Time().Sub(sendTS))
		lt.result = &TestResult{
			Server:      lt.cfg.Server,
			TXTimestamp: sendTS,
			RXTimestamp: b.ReceiveTimestamp.Time(),
		}
		lt.setState(stateDone)
		return nil
	default:
		log.Errorf("got unexpected packet %v", msgType)
	}
	return nil
}

// RunListener starts incoming packet listener.
// It's meant to be run in a goroutine before issuing calls to RunTest.
func (lt *Tester) RunListener(ctx context.Context) error {
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
	<-ctx.Done()
	log.Debugf("cancelled port receiver")
	return ctx.Err()
}

// runSingleTest performs one Tester run and will exit on completion.
// The run consists of:
// * starting the Unicast DelayResponse subscription on the receiver, if subDuration is not zero
// * sending one DelayRequest
// * receiving one DelayResponse
// The result of the test will be stored in the lt.result variable, unless error was returned.
// Warning: the listener must be started via RunListener before calling this function.
func (lt *Tester) runSingleTest(ctx context.Context, subDuration time.Duration) error {
	lt.setState(stateInit)
	var err error
	ctx, cancel := context.WithTimeout(ctx, lt.cfg.Timeout)
	defer cancel()
	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				log.Debugf("cancelled main loop")
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
					if subDuration != 0 {
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

// RunTest performs one Tester run and will exit on completion.
// The result of the test will be returned, including any error arising during the test.
// Warning: the listener must be started via RunListener before calling this function.
func (lt *Tester) RunTest(ctx context.Context) TestResult {
	result := TestResult{
		Server: lt.cfg.Server,
	}
	log.Debugf("test starting %s", lt.cfg.Server)
	err := lt.runSingleTest(ctx, 0)
	log.Debugf("test done %s", lt.cfg.Server)
	// re-run with subscription request
	if errors.Is(err, context.DeadlineExceeded) {
		log.Debugf("re-running timed out test with subscription renewal %s", lt.cfg.Server)
		err = lt.runSingleTest(ctx, time.Minute)
		log.Debugf("test done %s", lt.cfg.Server)
	}
	if lt.result != nil {
		result = *lt.result
	}
	if err != nil {
		result.Error = err
		if !errors.Is(err, context.Canceled) {
			log.Debugf("test against %s error: %v", lt.cfg.Server, err)
		}
	}
	return result
}

// ProcessMonitoringResults returns map of metrics based on TestResults
func ProcessMonitoringResults(prefix string, results map[string]*TestResult) map[string]int {
	failed := 0
	broken := 0
	skipped := 0

	for _, tr := range results {
		good, err := tr.Good()
		if err != nil {
			if errors.Is(err, ErrGrantDenied) {
				log.Debugf("denied grant is just drained GM")
				skipped++
			} else {
				broken++
			}
		} else {
			if !good {
				failed++
			}
		}
	}
	// general stats to JSON output
	output := map[string]int{}
	output[fmt.Sprintf("%sfailed_tests", prefix)] = failed
	output[fmt.Sprintf("%sbroken_tests", prefix)] = broken
	output[fmt.Sprintf("%sskipped_tests", prefix)] = skipped
	output[fmt.Sprintf("%stotal_tests", prefix)] = len(results)
	output[fmt.Sprintf("%spassed_tests", prefix)] = len(results) - skipped - failed - broken
	if len(results) == 0 {
		output[fmt.Sprintf("%sfailed_pct", prefix)] = 0
		output[fmt.Sprintf("%sbroken_pct", prefix)] = 0
		output[fmt.Sprintf("%sskipped_pct", prefix)] = 0
	} else {
		output[fmt.Sprintf("%sfailed_pct", prefix)] = int(100.0 * float64(failed) / float64(len(results)))
		output[fmt.Sprintf("%sbroken_pct", prefix)] = int(100.0 * float64(broken) / float64(len(results)))
		output[fmt.Sprintf("%sskipped_pct", prefix)] = int(100.0 * float64(skipped) / float64(len(results)))
	}
	return output
}
