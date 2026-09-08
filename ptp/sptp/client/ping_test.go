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
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/facebook/time/ptp/pdelay"
	ptp "github.com/facebook/time/ptp/protocol"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	testClockID = ptp.ClockIdentity(2)
)

var (
	pingPeer = netip.MustParseAddr("2401:db00::1")
)

// pWith wraps a request in the SPTP that owns it, so tests drive production paths
func pWith(req *pingRequest) *SPTP {
	return &SPTP{pinger: pingState{inflight: req}}
}

func newTestPingRequest(_ *testing.T, seq uint16, target netip.Addr) *pingRequest {
	return &pingRequest{
		seq:       seq,
		target:    target,
		multicast: target.IsMulticast(),
		requester: ptp.PortIdentity{ClockIdentity: testClockID, PortNumber: pingPDelayPort},
		results:   map[netip.Addr]*pdelay.Result{},
		done:      make(chan struct{}),
	}
}

func TestPingCorrectionFieldsAreNotSwapped(t *testing.T) {
	req := newTestPingRequest(t, 42, netip.MustParseAddr("ff02::6b"))

	base := time.Unix(1700000000, 0)
	resp := ptp.RespPDelay(ptp.ClockIdentity(1), 1, ptp.NewTimestamp(base.Add(100*time.Microsecond)),
		ptp.ReqPDelay(testClockID, pingPDelayPort, 42))
	resp.CorrectionField = ptp.NewCorrection(float64(3 * time.Microsecond))
	respBytes, err := ptp.Bytes(resp)
	require.NoError(t, err)

	followUp := ptp.RespFollowUpPDelay(ptp.ClockIdentity(1), 1, ptp.NewTimestamp(base.Add(200*time.Microsecond)),
		ptp.ReqPDelay(testClockID, pingPDelayPort, 42))
	followUp.CorrectionField = ptp.NewCorrection(float64(4 * time.Microsecond))
	followUpBytes, err := ptp.Bytes(followUp)
	require.NoError(t, err)

	require.NoError(t, pWith(req).handlePDelayResp(respBytes, pingPeer, base.Add(310*time.Microsecond)))
	require.NoError(t, pWith(req).handlePDelayRespFollowup(followUpBytes, pingPeer))

	res := req.results[pingPeer]
	res.T1 = base
	require.Equal(t, 3*time.Microsecond, res.CorrectionFieldReq, "CF from Pdelay_Resp is the request path")
	require.Equal(t, 4*time.Microsecond, res.CorrectionFieldResp, "CF from Follow_Up is the response path")
	require.True(t, res.Valid())

	// forward = (T2-T1)-CFReq = 100us-3us = 97us; backward = (T4-T3)-CFResp = 110us-4us = 106us
	require.Equal(t, (97*time.Microsecond+106*time.Microsecond)/2, res.PathDelay())
	require.Equal(t, (97*time.Microsecond-106*time.Microsecond)/2, res.Offset())
}

func TestPingOneStepResponderStaysIncomplete(t *testing.T) {
	req := newTestPingRequest(t, 7, netip.MustParseAddr("ff02::6b"))

	base := time.Unix(1700000000, 0)
	resp := ptp.RespPDelay(ptp.ClockIdentity(1), 1, ptp.NewTimestamp(base.Add(100*time.Microsecond)),
		ptp.ReqPDelay(testClockID, pingPDelayPort, 7))
	resp.FlagField = 0
	respBytes, err := ptp.Bytes(resp)
	require.NoError(t, err)

	require.NoError(t, pWith(req).handlePDelayResp(respBytes, pingPeer, base.Add(310*time.Microsecond)))

	// T3 only ever comes from the follow-up. Synthesising it as T2 would make
	// Valid() true while Offset() silently absorbs the responder turnaround.
	res := req.results[pingPeer]
	res.T1 = base
	require.True(t, res.T3.IsZero())
	require.False(t, res.Valid())
	require.Equal(t, pingPeer, res.Responder)
	require.False(t, res.Timestamp.IsZero())
}

// an sptp peer answers a unicast ping from handleDelayReq, which sends DELAY_REQ
// (not SYNC) with a fresh sequence id, so matching on sequence would drop it
func TestPingAcceptsUnicastSptpPeerReplyWithDifferentSequence(t *testing.T) {
	replyMsg := ReqDelay(ptp.ClockIdentity(1), 1)
	replyMsg.OriginTimestamp = ptp.NewTimestamp(time.Unix(1700000000, 0))
	reply, err := ptp.Bytes(replyMsg)
	require.NoError(t, err)
	require.NotEqual(t, uint16(42), replyMsg.SequenceID, "a peer replies with its own sequence")

	peer := newTestPingRequest(t, 42, pingPeer)
	require.True(t, pWith(peer).collectPingReply(reply, pingPeer, time.Unix(1700000000, 0)), "peer reply must be collected")
	require.False(t, pWith(peer).collectPingReply(reply, netip.MustParseAddr("2401:db00::2"), time.Unix(1700000000, 0)))

	// a configured GM echoes our sequence, so a mismatch there is real sync traffic
	gm := newTestPingRequest(t, 42, pingPeer)
	gm.targetIsGM = true
	require.False(t, pWith(gm).collectPingReply(reply, pingPeer, time.Unix(1700000000, 0)), "must not consume a GM's sync traffic")

	echoed := ReqDelay(ptp.ClockIdentity(1), 1)
	echoed.OriginTimestamp = ptp.NewTimestamp(time.Unix(1700000000, 0))
	echoed.SetSequence(42)
	echoedBytes, err := ptp.Bytes(echoed)
	require.NoError(t, err)
	require.True(t, pWith(gm).collectPingReply(echoedBytes, pingPeer, time.Unix(1700000000, 0)), "GM echoes our sequence")
}

// an inbound ping from the peer we are pinging must still reach the responder path
func TestPingAcceptsUnicastIgnoresInboundRequest(t *testing.T) {
	peer := newTestPingRequest(t, 42, pingPeer)

	// a genuine request leaves OriginTimestamp zero
	inbound, err := ptp.Bytes(ReqDelay(ptp.ClockIdentity(1), 1))
	require.NoError(t, err)
	require.False(t, pWith(peer).collectPingReply(inbound, pingPeer, time.Unix(1700000000, 0)), "must not swallow a real DelayReq")

	// an ANNOUNCE alone cannot start a reply, or periodic GM traffic completes the probe
	announceMsg := ReqAnnounce(ptp.ClockIdentity(1), 1, time.Unix(1700000000, 0))
	announceMsg.SetSequence(1)
	announce, err := ptp.Bytes(announceMsg)
	require.NoError(t, err)
	require.False(t, pWith(peer).collectPingReply(announce, pingPeer, time.Unix(1700000000, 0)), "stray ANNOUNCE must not be collected")

	// handleDelayReq stamps OriginTimestamp on its reply
	replyMsg := ReqDelay(ptp.ClockIdentity(1), 1)
	replyMsg.OriginTimestamp = ptp.NewTimestamp(time.Unix(1700000000, 0))
	reply, err := ptp.Bytes(replyMsg)
	require.NoError(t, err)
	require.True(t, pWith(peer).collectPingReply(reply, pingPeer, time.Unix(1700000000, 0)), "peer reply must be collected")

	// once the DELAY_REQ leg landed, the ANNOUNCE that follows it is ours
	peer.Lock()
	peer.result(pingPeer).T2 = time.Unix(1700000000, 0)
	peer.Unlock()
	require.True(t, pWith(peer).collectPingReply(announce, pingPeer, time.Unix(1700000000, 0)), "the reply's ANNOUNCE must be collected")
}

// an unconfigured GM's periodic SYNC must never complete a ptping probe
func TestPingAcceptsUnicastIgnoresUnrelatedSync(t *testing.T) {
	peer := newTestPingRequest(t, 42, pingPeer)

	syncMsg := ReqDelay(ptp.ClockIdentity(1), 1)
	syncMsg.SdoIDAndMsgType = ptp.NewSdoIDAndMsgType(ptp.MessageSync, 0)
	syncMsg.OriginTimestamp = ptp.NewTimestamp(time.Unix(1700000000, 0))
	syncMsg.SetSequence(9999)
	unrelated, err := ptp.Bytes(syncMsg)
	require.NoError(t, err)

	require.False(t, pWith(peer).collectPingReply(unrelated, pingPeer, time.Unix(1700000000, 0)),
		"a SYNC with a foreign sequence must not be collected")
}

func TestPingMatchesFiltersBySequenceAndAddr(t *testing.T) {
	hdr := func(seq uint16) ptp.Header { return ptp.Header{SequenceID: seq} }

	mcast := newTestPingRequest(t, 42, netip.MustParseAddr("ff02::6b"))
	require.True(t, mcast.matches(hdr(42), pingPeer), "multicast accepts any responder")
	require.False(t, mcast.matches(hdr(43), pingPeer))

	ucast := newTestPingRequest(t, 42, pingPeer)
	require.True(t, ucast.matches(hdr(42), pingPeer))
	require.False(t, ucast.matches(hdr(42), netip.MustParseAddr("2401:db00::2")), "unicast only accepts its target")

	var none *pingRequest
	require.False(t, none.matches(hdr(42), pingPeer))
}

// a multicast probe is only ever answered with Pdelay_Resp, so it must never
// consume a SYNC or ANNOUNCE that belongs to the sync loop
func TestPingAcceptsUnicastNeverShadowsSync(t *testing.T) {
	// a reply, not a PDelay_Req: only reply-shaped packets are ever accepted
	reply := ReqDelay(testClockID, 1)
	reply.OriginTimestamp = ptp.NewTimestamp(time.Unix(1700000000, 0))
	reply.SetSequence(42)
	good, err := ptp.Bytes(reply)
	require.NoError(t, err)

	mcast := newTestPingRequest(t, 42, netip.MustParseAddr("ff02::6b"))
	require.False(t, pWith(mcast).collectPingReply(good, pingPeer, time.Unix(1700000000, 0)))

	ucast := newTestPingRequest(t, 42, pingPeer)
	require.True(t, pWith(ucast).collectPingReply(good, pingPeer, time.Unix(1700000000, 0)))
	require.False(t, pWith(ucast).collectPingReply(good, netip.MustParseAddr("2401:db00::2"), time.Unix(1700000000, 0)))

	var none *pingRequest
	require.False(t, pWith(none).collectPingReply(good, pingPeer, time.Unix(1700000000, 0)))
}

func TestPingUnicastCollectsDelayReqShapedReply(t *testing.T) {
	req := newTestPingRequest(t, 9, pingPeer)
	base := time.Unix(1700000000, 0)

	// what an sptp peer's handleDelayReq actually puts on the wire
	reply := ReqDelay(ptp.ClockIdentity(1), 1)
	reply.OriginTimestamp = ptp.NewTimestamp(base.Add(100 * time.Microsecond))
	replyBytes, err := ptp.Bytes(reply)
	require.NoError(t, err)

	require.True(t, pWith(req).collectPingReply(replyBytes, pingPeer, base.Add(310*time.Microsecond)))
	res := req.results[pingPeer]
	require.Equal(t, base.Add(100*time.Microsecond).UnixNano(), res.T2.UnixNano())
	require.Equal(t, base.Add(310*time.Microsecond), res.T4)
}

func TestPingCancelledParentIsNotAnEmptyResult(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEventConn := NewMockUDPConnWithTS(ctrl)
	mockEventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(time.Unix(1700000000, 0), nil)

	p := &SPTP{clockID: ptp.ClockIdentity(1), eventConns: []UDPConnWithTS{mockEventConn}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := p.Ping(ctx, netip.MustParseAddr("ff02::6b"))
	require.ErrorIs(t, err, context.Canceled)
}

func TestPingUnicastSyncAndAnnounce(t *testing.T) {
	req := newTestPingRequest(t, 9, pingPeer)

	base := time.Unix(1700000000, 0)
	sync := ReqDelay(ptp.ClockIdentity(1), 1)
	sync.SetSequence(9)
	sync.OriginTimestamp = ptp.NewTimestamp(base.Add(100 * time.Microsecond))
	syncBytes, err := ptp.Bytes(sync)
	require.NoError(t, err)

	announce := ReqAnnounce(ptp.ClockIdentity(1), 1, base.Add(200*time.Microsecond))
	announce.SetSequence(9)
	announceBytes, err := ptp.Bytes(announce)
	require.NoError(t, err)

	require.True(t, pWith(req).collectPingReply(syncBytes, pingPeer, base.Add(310*time.Microsecond)))
	select {
	case <-req.done:
		t.Fatal("must not complete before ANNOUNCE arrives")
	default:
	}

	require.True(t, pWith(req).collectPingReply(announceBytes, pingPeer, base))
	select {
	case <-req.done:
	default:
		t.Fatal("must complete once SYNC and ANNOUNCE are both in")
	}

	res := req.results[pingPeer]
	require.Equal(t, base.Add(100*time.Microsecond).UnixNano(), res.T2.UnixNano())
	require.Equal(t, base.Add(200*time.Microsecond).UnixNano(), res.T3.UnixNano())
	require.Equal(t, base.Add(310*time.Microsecond), res.T4)
}

func TestPingMulticastSendsPDelayReq(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEventConn := NewMockUDPConnWithTS(ctrl)

	var sent []byte
	mockEventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(b []byte, _ any, _ uint16) (time.Time, error) {
			sent = append([]byte{}, b...)
			return time.Unix(1700000000, 0), nil
		})

	p := &SPTP{clockID: ptp.ClockIdentity(1), eventConns: []UDPConnWithTS{mockEventConn}}
	// multicast always waits the full window, so cap it rather than burn PingTimeout
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	res, err := p.Ping(ctx, netip.MustParseAddr("ff02::6b"))
	require.NoError(t, err)
	require.Empty(t, res, "no responders means an empty result, not an error")

	msgType, err := ptp.ProbeMsgType(sent)
	require.NoError(t, err)
	require.Equal(t, ptp.MessagePDelayReq, msgType)
}

func TestPingUnicastSendsDelayReq(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEventConn := NewMockUDPConnWithTS(ctrl)

	var sent []byte
	mockEventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(b []byte, _ any, _ uint16) (time.Time, error) {
			sent = append([]byte{}, b...)
			return time.Unix(1700000000, 0), nil
		})

	p := &SPTP{clockID: ptp.ClockIdentity(1), eventConns: []UDPConnWithTS{mockEventConn}}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	_, err := p.Ping(ctx, pingPeer)
	require.NoError(t, err)

	msgType, err := ptp.ProbeMsgType(sent)
	require.NoError(t, err)
	require.Equal(t, ptp.MessageDelayReq, msgType)
}

func TestPingIncompleteResponseIsReported(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEventConn := NewMockUDPConnWithTS(ctrl)
	mockEventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(time.Unix(1700000000, 0), nil)

	p := &SPTP{clockID: ptp.ClockIdentity(1), eventConns: []UDPConnWithTS{mockEventConn}}

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	go func() {
		// let Ping register the request first
		time.Sleep(10 * time.Millisecond)
		req := p.inflight()
		if req == nil {
			return
		}
		req.Lock()
		req.result(pingPeer).T2 = time.Unix(1700000000, 100)
		req.Unlock()
	}()

	res, err := p.Ping(ctx, pingPeer)
	require.NoError(t, err)
	require.Len(t, res, 1)
	require.EqualError(t, res[0].Error, "incomplete response")
}

func TestPingRejectsConcurrentCalls(t *testing.T) {
	p := &SPTP{clockID: ptp.ClockIdentity(1)}
	_, err := p.startPing(pingPeer)
	require.NoError(t, err)

	_, err = p.startPing(pingPeer)
	require.ErrorIs(t, err, ErrPingInFlight)

	p.finishPing()
	_, err = p.startPing(pingPeer)
	require.NoError(t, err)
}

// the production handlers, not just the collectors they delegate to
func TestHandlePDelayRespRouting(t *testing.T) {
	base := time.Unix(1700000000, 0)
	req := ptp.ReqPDelay(testClockID, pingPDelayPort, 42)
	resp := ptp.RespPDelay(ptp.ClockIdentity(1), 1, ptp.NewTimestamp(base.Add(100*time.Microsecond)), req)
	respBytes, err := ptp.Bytes(resp)
	require.NoError(t, err)
	followUp := ptp.RespFollowUpPDelay(ptp.ClockIdentity(1), 1, ptp.NewTimestamp(base.Add(200*time.Microsecond)), req)
	followUpBytes, err := ptp.Bytes(followUp)
	require.NoError(t, err)

	p := &SPTP{}
	require.NoError(t, p.handlePDelayResp(respBytes, pingPeer, base), "no ping in flight is a no-op")
	require.NoError(t, p.handlePDelayRespFollowup(followUpBytes, pingPeer))

	// wrong sequence must not be collected
	p.pinger.inflight = newTestPingRequest(t, 43, netip.MustParseAddr("ff02::6b"))
	require.NoError(t, p.handlePDelayResp(respBytes, pingPeer, base))
	require.Empty(t, p.pinger.inflight.results)

	// matching sequence is collected through the handler
	p.pinger.inflight = newTestPingRequest(t, 42, netip.MustParseAddr("ff02::6b"))
	require.NoError(t, p.handlePDelayResp(respBytes, pingPeer, base.Add(310*time.Microsecond)))
	require.NoError(t, p.handlePDelayRespFollowup(followUpBytes, pingPeer))
	res := p.pinger.inflight.results[pingPeer]
	require.NotNil(t, res)
	require.Equal(t, base.Add(100*time.Microsecond).UnixNano(), res.T2.UnixNano())
	require.Equal(t, base.Add(200*time.Microsecond).UnixNano(), res.T3.UnixNano())
	require.Equal(t, base.Add(310*time.Microsecond), res.T4)
}

// startPing must detect a configured GM rather than relying on a hand-set flag
func TestStartPingDetectsGM(t *testing.T) {
	gm := netip.MustParseAddr("2401:db00::9")
	p := &SPTP{clients: map[netip.Addr]*Client{gm: {}}}

	req, err := p.startPing(gm)
	require.NoError(t, err)
	require.True(t, req.targetIsGM)
	p.finishPing()

	req, err = p.startPing(pingPeer)
	require.NoError(t, err)
	require.False(t, req.targetIsGM, "an unconfigured peer is not a GM")
}

// results handed to the caller must not alias the map a listener can still write
func TestPingResultsAreSnapshots(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEventConn := NewMockUDPConnWithTS(ctrl)
	mockEventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(time.Unix(1700000000, 0), nil)

	p := &SPTP{clockID: ptp.ClockIdentity(1), eventConns: []UDPConnWithTS{mockEventConn}}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	go func() {
		time.Sleep(10 * time.Millisecond)
		if req := p.inflight(); req != nil {
			req.Lock()
			req.result(pingPeer).T2 = time.Unix(1700000000, 100)
			req.Unlock()
		}
	}()

	res, err := p.Ping(ctx, pingPeer)
	require.NoError(t, err)
	require.Len(t, res, 1)

	// mutating what we got back must not be visible in the (detached) request
	before := res[0].T2
	res[0].T2 = time.Unix(1, 0)
	require.NotEqual(t, res[0].T2, before)
}

// the public entrypoint, end to end: Ping dispatches, the production handlers
// route matching responses in, and a complete measurement comes back
func TestPingMulticastEndToEnd(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEventConn := NewMockUDPConnWithTS(ctrl)

	base := time.Unix(1700000000, 0)
	seq := make(chan uint16, 1)
	mockEventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ []byte, _ any, s uint16) (time.Time, error) {
			seq <- s
			return base, nil
		})

	p := &SPTP{clockID: testClockID, eventConns: []UDPConnWithTS{mockEventConn}}

	// the responder goroutine only records an error; require runs on the test goroutine
	var respErr error
	var wg sync.WaitGroup
	wg.Go(func() {
		req := ptp.ReqPDelay(testClockID, pingPDelayPort, <-seq)
		resp := ptp.RespPDelay(ptp.ClockIdentity(3), 1, ptp.NewTimestamp(base.Add(100*time.Microsecond)), req)
		resp.CorrectionField = ptp.NewCorrection(float64(3 * time.Microsecond))
		respBytes, err := ptp.Bytes(resp)
		if err != nil {
			respErr = err
			return
		}
		followUp := ptp.RespFollowUpPDelay(ptp.ClockIdentity(3), 1, ptp.NewTimestamp(base.Add(200*time.Microsecond)), req)
		followUp.CorrectionField = ptp.NewCorrection(float64(4 * time.Microsecond))
		followUpBytes, err := ptp.Bytes(followUp)
		if err != nil {
			respErr = err
			return
		}
		// through the same handlers RunListener calls
		if err := p.handlePDelayResp(respBytes, pingPeer, base.Add(310*time.Microsecond)); err != nil {
			respErr = err
			return
		}
		respErr = p.handlePDelayRespFollowup(followUpBytes, pingPeer)
	})

	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	res, err := p.Ping(ctx, netip.MustParseAddr("ff02::6b"))
	wg.Wait()

	require.NoError(t, respErr)
	require.NoError(t, err)
	require.Len(t, res, 1)

	r := res[0]
	require.NoError(t, r.Error)
	require.True(t, r.Valid())
	require.Equal(t, pingPeer, r.Responder)
	require.Equal(t, base, r.T1)
	require.Equal(t, base.Add(100*time.Microsecond).UnixNano(), r.T2.UnixNano())
	require.Equal(t, base.Add(200*time.Microsecond).UnixNano(), r.T3.UnixNano())
	require.Equal(t, base.Add(310*time.Microsecond), r.T4)
	require.Positive(t, r.SWRTT)
	// forward = 100us-3us, backward = 110us-4us
	require.Equal(t, (97*time.Microsecond+106*time.Microsecond)/2, r.PathDelay())
}

func TestCollectPingReply(t *testing.T) {
	base := time.Unix(1700000000, 0)
	reply := ReqDelay(ptp.ClockIdentity(1), 1)
	reply.OriginTimestamp = ptp.NewTimestamp(base)
	replyBytes, err := ptp.Bytes(reply)
	require.NoError(t, err)
	announceMsg := ReqAnnounce(ptp.ClockIdentity(1), 1, base)
	announceMsg.SetSequence(1)
	announce, err := ptp.Bytes(announceMsg)
	require.NoError(t, err)
	pdelayResp, err := ptp.Bytes(ptp.RespPDelay(ptp.ClockIdentity(1), 1,
		ptp.NewTimestamp(base), ptp.ReqPDelay(testClockID, pingPDelayPort, 42)))
	require.NoError(t, err)

	p := &SPTP{}
	require.False(t, p.collectPingReply(replyBytes, pingPeer, base), "no ping in flight")

	p.pinger.inflight = newTestPingRequest(t, 42, pingPeer)
	require.False(t, p.collectPingReply(pdelayResp, pingPeer, base), "PDELAY_RESP is not a unicast reply")
	require.False(t, p.collectPingReply(replyBytes, netip.MustParseAddr("2401:db00::9"), base), "wrong source")

	require.True(t, p.collectPingReply(replyBytes, pingPeer, base.Add(310*time.Microsecond)))
	require.Equal(t, base.Add(310*time.Microsecond), p.pinger.inflight.results[pingPeer].T4)

	require.True(t, p.collectPingReply(announce, pingPeer, base))
	require.False(t, p.pinger.inflight.results[pingPeer].T3.IsZero())
}

func TestPingSendFailureReleasesSlot(t *testing.T) {
	for _, tt := range []struct {
		name   string
		target netip.Addr
	}{
		{"multicast", netip.MustParseAddr("ff02::6b")},
		{"unicast", pingPeer},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockEventConn := NewMockUDPConnWithTS(ctrl)
			mockEventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(time.Time{}, errors.New("send failed"))

			p := &SPTP{clockID: ptp.ClockIdentity(1), eventConns: []UDPConnWithTS{mockEventConn}}
			_, err := p.Ping(t.Context(), tt.target)
			require.ErrorContains(t, err, "send failed")
			require.Nil(t, p.inflight(), "a failed send must not leave the slot held")
		})
	}
}

// the general port has no RX timestamp, so a T4-bearing reply must not be
// collected there or the measurement records T4=0
func TestCollectPingReplyRequiresRXTimestamp(t *testing.T) {
	base := time.Unix(1700000000, 0)
	reply := ReqDelay(ptp.ClockIdentity(1), 1)
	reply.OriginTimestamp = ptp.NewTimestamp(base)
	replyBytes, err := ptp.Bytes(reply)
	require.NoError(t, err)
	announceMsg := ReqAnnounce(ptp.ClockIdentity(1), 1, base)
	announceMsg.SetSequence(1)
	announce, err := ptp.Bytes(announceMsg)
	require.NoError(t, err)

	p := &SPTP{pinger: pingState{inflight: newTestPingRequest(t, 42, pingPeer)}}
	require.False(t, p.collectPingReply(replyBytes, pingPeer, time.Time{}), "no timestamp, no T4")
	require.Empty(t, p.pinger.inflight.results)

	// with a timestamp the reply lands, and the ANNOUNCE that follows completes it
	require.True(t, p.collectPingReply(replyBytes, pingPeer, base.Add(310*time.Microsecond)))
	require.True(t, p.collectPingReply(announce, pingPeer, time.Time{}), "ANNOUNCE needs no RX timestamp")
	require.False(t, p.pinger.inflight.results[pingPeer].T3.IsZero())
}

// multicast makes another host's exchange visible to us; a sequence collision
// inside the 1s window must not be recorded as our measurement
func TestCollectPDelayRejectsForeignRequester(t *testing.T) {
	base := time.Unix(1700000000, 0)
	foreign := ptp.ReqPDelay(ptp.ClockIdentity(99), pingPDelayPort, 42)
	resp, err := ptp.Bytes(ptp.RespPDelay(ptp.ClockIdentity(3), 1, ptp.NewTimestamp(base), foreign))
	require.NoError(t, err)
	followUp, err := ptp.Bytes(ptp.RespFollowUpPDelay(ptp.ClockIdentity(3), 1, ptp.NewTimestamp(base), foreign))
	require.NoError(t, err)

	req := newTestPingRequest(t, 42, netip.MustParseAddr("ff02::6b"))
	require.NoError(t, pWith(req).handlePDelayResp(resp, pingPeer, base.Add(310*time.Microsecond)))
	require.NoError(t, pWith(req).handlePDelayRespFollowup(followUp, pingPeer))
	require.Empty(t, req.results, "another requester's exchange must not be collected")

	// the same sequence addressed to us is collected
	ours := ptp.ReqPDelay(testClockID, pingPDelayPort, 42)
	resp, err = ptp.Bytes(ptp.RespPDelay(ptp.ClockIdentity(3), 1, ptp.NewTimestamp(base), ours))
	require.NoError(t, err)
	require.NoError(t, pWith(req).handlePDelayResp(resp, pingPeer, base.Add(310*time.Microsecond)))
	require.Len(t, req.results, 1)
}

// a genuine inbound DelayReq that collides on our 16-bit sequence must still reach
// the responder path instead of being consumed as a ping reply
func TestPingAcceptsUnicastIgnoresCollidingRequest(t *testing.T) {
	inbound := ReqDelay(ptp.ClockIdentity(9), 1)
	inbound.SetSequence(42)
	buf, err := ptp.Bytes(inbound)
	require.NoError(t, err)

	for _, targetIsGM := range []bool{false, true} {
		req := newTestPingRequest(t, 42, pingPeer)
		req.targetIsGM = targetIsGM
		require.False(t, pWith(req).collectPingReply(buf, pingPeer, time.Unix(1700000000, 0)),
			"targetIsGM=%v: a request with no OriginTimestamp is not a reply", targetIsGM)
	}
}

// WithCancelCause leaves ctx.Err() as Canceled but replaces the cause, so gating on
// the cause would report an empty result as a successful probe
func TestPingCancelledWithCauseIsNotAnEmptyResult(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockEventConn := NewMockUDPConnWithTS(ctrl)
	mockEventConn.EXPECT().WriteToWithTS(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(time.Unix(1700000000, 0), nil)

	p := &SPTP{clockID: testClockID, eventConns: []UDPConnWithTS{mockEventConn}}
	shutdown := errors.New("shutting down")
	ctx, cancel := context.WithCancelCause(t.Context())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel(shutdown)
	}()

	res, err := p.Ping(ctx, netip.MustParseAddr("ff02::6b"))
	require.ErrorIs(t, err, shutdown, "the cause must surface, not an empty success")
	require.Nil(t, res)
}

// only the ANNOUNCE owed by a T2-bearing reply may complete the exchange, or a
// periodic one pairs its timestamp with our T2 and silently skews the result
func TestCollectPingReplyRejectsUnrelatedAnnounce(t *testing.T) {
	base := time.Unix(1700000000, 0)
	announceMsg := ReqAnnounce(ptp.ClockIdentity(1), 1, base)
	announceMsg.SetSequence(1)
	announce, err := ptp.Bytes(announceMsg)
	require.NoError(t, err)
	reply := ReqDelay(ptp.ClockIdentity(1), 1)
	reply.OriginTimestamp = ptp.NewTimestamp(base)
	replyBytes, err := ptp.Bytes(reply)
	require.NoError(t, err)

	p := &SPTP{pinger: pingState{inflight: newTestPingRequest(t, 42, pingPeer)}}
	require.False(t, p.collectPingReply(announce, pingPeer, time.Time{}), "no reply yet, nothing owed")

	require.True(t, p.collectPingReply(replyBytes, pingPeer, base.Add(310*time.Microsecond)))
	require.True(t, p.collectPingReply(announce, pingPeer, time.Time{}), "the owed ANNOUNCE")
	require.False(t, p.collectPingReply(announce, pingPeer, time.Time{}), "a second one is unrelated")
}

// a periodic ANNOUNCE landing between the T2 reply and the real one must neither
// complete the exchange nor consume the owed state
func TestCollectPingReplyWrongSequenceAnnounceDoesNotConsume(t *testing.T) {
	base := time.Unix(1700000000, 0)
	reply := ReqDelay(ptp.ClockIdentity(1), 1)
	reply.OriginTimestamp = ptp.NewTimestamp(base)
	reply.SetSequence(100)
	replyBytes, err := ptp.Bytes(reply)
	require.NoError(t, err)

	stray := ReqAnnounce(ptp.ClockIdentity(1), 1, base.Add(time.Hour))
	stray.SetSequence(9000)
	strayBytes, err := ptp.Bytes(stray)
	require.NoError(t, err)

	paired := ReqAnnounce(ptp.ClockIdentity(1), 1, base.Add(200*time.Microsecond))
	paired.SetSequence(101)
	pairedBytes, err := ptp.Bytes(paired)
	require.NoError(t, err)

	p := &SPTP{pinger: pingState{inflight: newTestPingRequest(t, 42, pingPeer)}}
	require.True(t, p.collectPingReply(replyBytes, pingPeer, base.Add(310*time.Microsecond)))

	require.False(t, p.collectPingReply(strayBytes, pingPeer, time.Time{}), "far sequence is not ours")
	require.Equal(t, awaitingAnnounce, p.pinger.inflight.stage, "a rejected ANNOUNCE must not advance the exchange")

	require.True(t, p.collectPingReply(pairedBytes, pingPeer, time.Time{}), "the paired ANNOUNCE")
	require.Equal(t, base.Add(200*time.Microsecond).UnixNano(), p.pinger.inflight.results[pingPeer].T3.UnixNano())
}

// an ANNOUNCE echoing our sequence before the T2 reply must not seed T3, or
// collectSync later completes the exchange against an unrelated timestamp
func TestCollectPingReplyAnnounceBeforeReplyRejected(t *testing.T) {
	base := time.Unix(1700000000, 0)
	early := ReqAnnounce(ptp.ClockIdentity(1), 1, base.Add(time.Hour))
	early.SetSequence(42)
	earlyBytes, err := ptp.Bytes(early)
	require.NoError(t, err)

	p := &SPTP{pinger: pingState{inflight: newTestPingRequest(t, 42, pingPeer)}}
	require.False(t, p.collectPingReply(earlyBytes, pingPeer, time.Time{}), "no T2 yet")
	require.Empty(t, p.pinger.inflight.results, "T3 must not be seeded before the reply")

	select {
	case <-p.pinger.inflight.done:
		t.Fatal("must not complete from an ANNOUNCE alone")
	default:
	}
}

// a peer replies from its own counter, so an ANNOUNCE bearing our requester
// sequence is unrelated traffic and must not seed T3
func TestCollectPingReplyPeerAnnounceCollidingWithRequesterSeq(t *testing.T) {
	base := time.Unix(1700000000, 0)
	reply := ReqDelay(ptp.ClockIdentity(1), 1)
	reply.OriginTimestamp = ptp.NewTimestamp(base)
	reply.SetSequence(9000)
	replyBytes, err := ptp.Bytes(reply)
	require.NoError(t, err)

	colliding := ReqAnnounce(ptp.ClockIdentity(1), 1, base.Add(time.Hour))
	colliding.SetSequence(42)
	collidingBytes, err := ptp.Bytes(colliding)
	require.NoError(t, err)

	p := &SPTP{pinger: pingState{inflight: newTestPingRequest(t, 42, pingPeer)}}
	require.True(t, p.collectPingReply(replyBytes, pingPeer, base.Add(310*time.Microsecond)))
	require.False(t, p.collectPingReply(collidingBytes, pingPeer, time.Time{}),
		"our own sequence is not how a peer answers")
	require.True(t, p.pinger.inflight.results[pingPeer].T3.IsZero(), "T3 must stay unset")
}

// once a unicast exchange completes, a later reply must not pair a fresh T2/T4
// with the T3 already recorded
func TestCollectPingReplyFreezesCompletedUnicast(t *testing.T) {
	base := time.Unix(1700000000, 0)
	mk := func(seq uint16, origin time.Time) ([]byte, []byte) {
		reply := ReqDelay(ptp.ClockIdentity(1), 1)
		reply.OriginTimestamp = ptp.NewTimestamp(origin)
		reply.SetSequence(seq)
		rb, err := ptp.Bytes(reply)
		require.NoError(t, err)
		ann := ReqAnnounce(ptp.ClockIdentity(1), 1, origin.Add(100*time.Microsecond))
		ann.SetSequence(seq + 1)
		ab, err := ptp.Bytes(ann)
		require.NoError(t, err)
		return rb, ab
	}

	first, firstAnn := mk(100, base)
	later, _ := mk(200, base.Add(time.Hour))

	p := &SPTP{pinger: pingState{inflight: newTestPingRequest(t, 42, pingPeer)}}
	require.True(t, p.collectPingReply(first, pingPeer, base.Add(310*time.Microsecond)))
	require.True(t, p.collectPingReply(firstAnn, pingPeer, time.Time{}))

	want := p.pinger.inflight.results[pingPeer].T2
	p.collectPingReply(later, pingPeer, base.Add(2*time.Hour))
	require.Equal(t, want, p.pinger.inflight.results[pingPeer].T2, "a completed exchange is frozen")
}

// a peer ANNOUNCE reusing the reply's own sequence is not the one that follows it
func TestCollectPingReplySameSequenceAnnounceRejected(t *testing.T) {
	base := time.Unix(1700000000, 0)
	reply := ReqDelay(ptp.ClockIdentity(1), 1)
	reply.OriginTimestamp = ptp.NewTimestamp(base)
	reply.SetSequence(100)
	replyBytes, err := ptp.Bytes(reply)
	require.NoError(t, err)

	same := ReqAnnounce(ptp.ClockIdentity(1), 1, base.Add(time.Hour))
	same.SetSequence(100)
	sameBytes, err := ptp.Bytes(same)
	require.NoError(t, err)

	p := &SPTP{pinger: pingState{inflight: newTestPingRequest(t, 42, pingPeer)}}
	require.True(t, p.collectPingReply(replyBytes, pingPeer, base.Add(310*time.Microsecond)))
	require.False(t, p.collectPingReply(sameBytes, pingPeer, time.Time{}))
	require.True(t, p.pinger.inflight.results[pingPeer].T3.IsZero())
}

// the listener checks the ping before p.clients, so a ping sequence inside the
// sync clients' masked space would steal a genuine SYNC from an exchange
func TestStartPingAvoidsSyncSequenceSpace(t *testing.T) {
	cfg := &Config{SequenceIDMaskBits: 2, SequenceIDMaskValue: 1}
	mask, value := cfg.GenerateMaskAndValue()

	// start one below the sync space so the very next sequence would collide
	p := &SPTP{cfg: cfg, clockID: testClockID, pinger: pingState{sequence: value - 1, mask: mask, value: value, masked: true}}
	for i := 0; i < 3*int(^mask+1); i++ {
		req, err := p.startPing(pingPeer)
		require.NoError(t, err)
		require.NotEqual(t, value, req.seq&^mask,
			"ping seq %#04x lands in the sync space %#04x..%#04x", req.seq, value, value|mask)
		p.finishPing()
	}
}

// with no mask configured the clients own the whole space, so the skip must not spin
func TestStartPingNoMaskDoesNotHang(t *testing.T) {
	p := &SPTP{cfg: &Config{}, clockID: testClockID}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 100 {
			if _, err := p.startPing(pingPeer); err != nil {
				return
			}
			p.finishPing()
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("startPing hung with no sequence mask configured")
	}
}

// Ping is exported, so an SPTP without event connections must error, not panic
func TestPingWithoutEventConnErrors(t *testing.T) {
	p := &SPTP{clockID: testClockID}
	_, err := p.Ping(t.Context(), pingPeer)
	require.ErrorContains(t, err, "no event connection")
	require.Nil(t, p.inflight(), "the failed send must release the slot")
}

// SyncDelayReq carries both SYNC and DELAY_REQ, so the decoded header must report
// the same type the listener read off the wire; the reply guard depends on it
func TestDecodedReplyMessageTypeMatchesWire(t *testing.T) {
	base := time.Unix(1700000000, 0)
	for _, want := range []ptp.MessageType{ptp.MessageSync, ptp.MessageDelayReq} {
		t.Run(want.String(), func(t *testing.T) {
			msg := ReqDelay(ptp.ClockIdentity(1), 1)
			msg.OriginTimestamp = ptp.NewTimestamp(base)
			msg.SetSequence(7)
			msg.SdoIDAndMsgType = ptp.NewSdoIDAndMsgType(want, 0)
			b, err := ptp.Bytes(msg)
			require.NoError(t, err)

			onWire, err := ptp.ProbeMsgType(b)
			require.NoError(t, err)
			require.Equal(t, want, onWire)

			decoded := &ptp.SyncDelayReq{}
			require.NoError(t, ptp.FromBytes(b, decoded))
			require.Equal(t, onWire, decoded.MessageType(), "decoded type must match the wire")
		})
	}
}

// an unconfigured ptp4u GM is not in p.clients, so targetIsGM is false, but it
// still echoes our sequence and reuses it for the ANNOUNCE
func TestCollectPingReplyUnconfiguredGMCompletes(t *testing.T) {
	base := time.Unix(1700000000, 0)
	reply := ReqDelay(ptp.ClockIdentity(1), 1)
	reply.OriginTimestamp = ptp.NewTimestamp(base)
	reply.SetSequence(42)
	replyBytes, err := ptp.Bytes(reply)
	require.NoError(t, err)

	announce := ReqAnnounce(ptp.ClockIdentity(1), 1, base.Add(200*time.Microsecond))
	announce.SetSequence(42)
	announceBytes, err := ptp.Bytes(announce)
	require.NoError(t, err)

	req := newTestPingRequest(t, 42, pingPeer)
	require.False(t, req.targetIsGM, "not a configured client")
	p := pWith(req)

	require.True(t, p.collectPingReply(replyBytes, pingPeer, base.Add(310*time.Microsecond)))
	require.True(t, p.collectPingReply(announceBytes, pingPeer, time.Time{}),
		"the echoed-sequence ANNOUNCE completes the exchange")

	res := req.results[pingPeer]
	require.False(t, res.T3.IsZero(), "T3 must be recorded")
	select {
	case <-req.done:
	default:
		t.Fatal("exchange must complete")
	}
}
