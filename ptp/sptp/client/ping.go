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
	"fmt"
	rnd "math/rand/v2"
	"net/netip"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/facebook/time/ptp/pdelay"
	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/timestamp"
	"golang.org/x/sys/unix"
)

const (
	// PingTimeout is how long we collect responses for, both multicast and unicast
	PingTimeout = time.Second
	// pingPDelayPort is the port number advertised in Pdelay_Req
	pingPDelayPort = 1
	// announceSeqWindow is how far past the reply sequence the paired ANNOUNCE may
	// sit: handleDelayReq sends it next, but the peer masks its counter
	announceSeqWindow = 4
)

// ErrPingInFlight is returned when another ping is already running
var ErrPingInFlight = errors.New("ping already in flight")

// Pinger measures network latency to a peer, reusing the PTP event port
type Pinger interface {
	Ping(ctx context.Context, target netip.Addr) (pdelay.Results, error)
}

// pingState owns everything a ping needs: the single in-flight slot and the
// identifiers it allocates. Kept together rather than spread across SPTP, which
// otherwise holds only sync state.
type pingState struct {
	mu       sync.Mutex
	inflight *pingRequest
	// sequence and portID mirror what Client tracks for a sync exchange: a
	// running sequence counter plus the port identity we send from
	sequence uint16
	portID   uint16
	// mask and value describe the sync clients' sequence space, which the ping
	// must stay out of because the listener checks it before p.clients
	mask, value uint16
	masked      bool
}

// init seeds the identifiers. It takes a pointer because pingState holds a mutex
// and must never be copied.
func (s *pingState) init(cfg *Config) {
	// a random seed avoids colliding with the sync clients on every restart
	s.sequence = rnd.N[uint16](65535)
	// stay above 10 so ptp4u sends the ANNOUNCE back to the port we sent from
	s.portID = rnd.N[uint16](65524) + 11
	if cfg != nil && cfg.SequenceIDMaskBits > 0 {
		s.mask, s.value = cfg.GenerateMaskAndValue()
		s.masked = true
	}
}

// nextSequence allocates the next ping sequence, skipping the sync clients' space
func (s *pingState) nextSequence() uint16 {
	s.sequence++
	for s.masked && s.sequence&^s.mask == s.value {
		s.sequence++
	}
	return s.sequence
}

// pingRequest is a single in-flight ping and the responses collected for it so far
type pingRequest struct {
	sync.Mutex
	seq       uint16
	target    netip.Addr
	multicast bool
	// targetIsGM means the target is a configured grandmaster, whose sync traffic we
	// must not consume, so its replies are matched strictly on sequence ID
	targetIsGM bool
	// requester is the identity we put in the Pdelay_Req; responders echo it back, and
	// on multicast another requester's exchange is equally visible to us
	requester ptp.PortIdentity
	// sentAt is when the probe left, so each responder's software round trip is
	// measured on arrival rather than after the whole collection window
	sentAt time.Time
	// echoedSeq is how the T2 reply matched: a GM echoes our sequence and reuses it
	// for the ANNOUNCE, a peer answers from its own counter. Taken from the reply,
	// not config, because an unconfigured GM still echoes.
	stage     unicastStage
	replySeq  uint16
	echoedSeq bool
	results   map[netip.Addr]*pdelay.Result
	done      chan struct{}
	closed    bool
}

func (r *pingRequest) result(addr netip.Addr) *pdelay.Result {
	res, ok := r.results[addr]
	if !ok {
		res = &pdelay.Result{Responder: addr, Timestamp: time.Now()}
		r.results[addr] = res
	}
	return res
}

// complete marks the request as answered. Must be called with the lock held.
func (r *pingRequest) complete() {
	if !r.closed {
		r.closed = true
		close(r.done)
	}
}

// inflight returns the currently running ping, or nil
func (p *SPTP) inflight() *pingRequest {
	p.pinger.mu.Lock()
	defer p.pinger.mu.Unlock()
	return p.pinger.inflight
}

func (p *SPTP) startPing(target netip.Addr) (*pingRequest, error) {
	p.pinger.mu.Lock()
	defer p.pinger.mu.Unlock()
	if p.pinger.inflight != nil {
		return nil, ErrPingInFlight
	}
	_, isGM := p.clients[target]
	r := &pingRequest{
		seq:        p.pinger.nextSequence(),
		target:     target,
		multicast:  target.IsMulticast(),
		targetIsGM: isGM,
		requester:  ptp.PortIdentity{ClockIdentity: p.clockID, PortNumber: pingPDelayPort},
		results:    map[netip.Addr]*pdelay.Result{},
		done:       make(chan struct{}),
	}
	p.pinger.inflight = r
	return r, nil
}

func (p *SPTP) finishPing() {
	p.pinger.mu.Lock()
	defer p.pinger.mu.Unlock()
	p.pinger.inflight = nil
}

// Ping measures latency to target from the event port, so packets leave with
// source port 319 for NICs that only timestamp that port.
func (p *SPTP) Ping(ctx context.Context, target netip.Addr) (pdelay.Results, error) {
	req, err := p.startPing(target)
	if err != nil {
		return nil, err
	}
	defer p.finishPing()

	ctx, cancel := context.WithTimeout(ctx, PingTimeout)
	defer cancel()

	req.Lock()
	req.sentAt = time.Now()
	req.Unlock()
	var t1 time.Time
	if req.multicast {
		t1, err = p.pingMulticast(req)
	} else {
		t1, err = p.pingUnicast(req)
	}
	if err != nil {
		return nil, err
	}

	if req.multicast {
		// we do not know how many peers will answer, so always wait the full timeout
		<-ctx.Done()
	} else {
		select {
		case <-req.done:
		case <-ctx.Done():
		}
	}
	// a cancelled parent means we never actually waited, which is not an empty result
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil, context.Cause(ctx)
	}
	req.Lock()
	defer req.Unlock()
	res := make(pdelay.Results, 0, len(req.results))
	for _, r := range req.results {
		// copy: a late response can still mutate the map-owned Result after we
		// release the lock, which would race the caller reading what we return
		snapshot := *r
		snapshot.T1 = t1
		if !snapshot.Valid() {
			snapshot.Error = errors.New("incomplete response")
		}
		res = append(res, &snapshot)
	}
	return res, nil
}

// sendProbe writes msg to the target from the event port and returns T1
func (p *SPTP) sendProbe(req *pingRequest, msg ptp.Packet) (time.Time, error) {
	if len(p.eventConns) == 0 {
		return time.Time{}, errors.New("no event connection to probe from")
	}
	msgType := msg.MessageType()
	b, err := ptp.Bytes(msg)
	if err != nil {
		return time.Time{}, fmt.Errorf("marshaling %s: %w", msgType, err)
	}
	addr := timestamp.AddrToSockaddr(req.target, ptp.PortEvent)
	// peers reply to the source they saw, and the group would get a link-local one
	var src unix.Sockaddr
	if req.multicast {
		pinned, ok := p.pdelaySrc[req.target.Unmap().Is4()]
		if !ok {
			return time.Time{}, fmt.Errorf("no source address to probe %s from", req.target)
		}
		src = timestamp.AddrToSockaddr(pinned, ptp.PortEvent)
	}
	t1, err := p.eventConns[0].WriteToWithTS(b, src, addr, req.seq)
	if err != nil {
		return time.Time{}, fmt.Errorf("sending %s to %s: %w", msgType, req.target, err)
	}
	log.Debugf("[ping] sent %s seq=%d to %s, T1=%v", msgType, req.seq, req.target, t1)
	return t1, nil
}

// pingMulticast sends a Pdelay_Req to the peer delay group and returns T1
func (p *SPTP) pingMulticast(req *pingRequest) (time.Time, error) {
	return p.sendProbe(req, ptp.ReqPDelay(p.clockID, pingPDelayPort, req.seq))
}

// pingUnicast sends a DelayReq to a single peer and returns T1
func (p *SPTP) pingUnicast(req *pingRequest) (time.Time, error) {
	dr := ReqDelay(p.clockID, p.pinger.portID)
	dr.SetSequence(req.seq)
	return p.sendProbe(req, dr)
}

// unicastStage is the step a unicast exchange has reached. A ping is one
// request and one reply pair, so the order is fixed and each step happens once.
type unicastStage int

const (
	// awaitingReply is waiting for the SYNC or stamped DELAY_REQ carrying T2/T4
	awaitingReply unicastStage = iota
	// awaitingAnnounce is waiting for the ANNOUNCE carrying T3
	awaitingAnnounce
	// exchangeDone is complete; later traffic must not rewrite the measurement
	exchangeDone
)

// collectPingReply routes buf to the in-flight unicast ping and reports whether it
// was consumed, so the listener does not also treat it as sync traffic.
func (p *SPTP) collectPingReply(buf []byte, addr netip.Addr, rxts time.Time) bool {
	msgType, err := ptp.ProbeMsgType(buf)
	if err != nil {
		return false
	}
	req := p.inflight()
	if req == nil || req.multicast || addr != req.target {
		return false
	}

	switch msgType {
	case ptp.MessageSync, ptp.MessageDelayReq:
		// these carry T4, and the general port has no RX timestamp, so leave them
		// for the event port rather than record a zero
		if rxts.IsZero() {
			return false
		}
		reply := &ptp.SyncDelayReq{}
		if err := ptp.FromBytes(buf, reply); err != nil {
			log.Warningf("[%s] parsing ping %s: %v", addr, msgType, err)
			return false
		}
		return req.collectReply(reply, addr, rxts)
	case ptp.MessageAnnounce:
		announce := &ptp.Announce{}
		if err := ptp.FromBytes(buf, announce); err != nil {
			log.Warningf("[%s] parsing ping ANNOUNCE: %v", addr, err)
			return false
		}
		return req.collectAnnounce(announce, addr)
	default:
		return false
	}
}

// collectReply records T2/T4 from the reply opening the exchange. A GM echoes our
// sequence; an sptp peer answers from its own counter, so it is matched on the shape
// handleDelayReq produces -- a DELAY_REQ carrying a stamped OriginTimestamp, which a
// genuine inbound request never has.
func (r *pingRequest) collectReply(reply *ptp.SyncDelayReq, addr netip.Addr, rxts time.Time) bool {
	r.Lock()
	defer r.Unlock()
	msgType := reply.MessageType()
	if r.stage != awaitingReply {
		return false
	}
	echoed := reply.SequenceID == r.seq
	if !echoed {
		replyShaped := msgType == ptp.MessageDelayReq && !reply.OriginTimestamp.Time().IsZero()
		if r.targetIsGM || !replyShaped {
			return false
		}
	} else if msgType == ptp.MessageDelayReq && reply.OriginTimestamp.Time().IsZero() {
		// our own sequence on a bare request is an inbound probe, not a reply
		return false
	}

	res := r.result(addr)
	res.T2 = reply.OriginTimestamp.Time()
	res.T4 = rxts
	r.replySeq, r.echoedSeq, r.stage = reply.SequenceID, echoed, awaitingAnnounce
	return true
}

// collectAnnounce records T3 and completes the exchange. Validation and recording
// share one lock, so a concurrent reply cannot move replySeq in between.
func (r *pingRequest) collectAnnounce(announce *ptp.Announce, addr netip.Addr) bool {
	r.Lock()
	defer r.Unlock()
	if r.stage != awaitingAnnounce {
		return false
	}
	if r.echoedSeq {
		if announce.SequenceID != r.seq {
			return false
		}
	} else if delta := announce.SequenceID - r.replySeq; delta == 0 || delta > announceSeqWindow {
		return false
	}

	res := r.result(addr)
	res.T3 = announce.OriginTimestamp.Time()
	res.SWRTT = time.Since(r.sentAt)
	r.stage = exchangeDone
	r.complete()
	return true
}

// matches reports whether a decoded reply belongs to this ping. Multicast is
// answered by peers we did not name, so only the sequence identifies it there.
func (r *pingRequest) matches(h ptp.Header, addr netip.Addr) bool {
	if r == nil || h.SequenceID != r.seq {
		return false
	}
	return r.multicast || addr == r.target
}

func (r *pingRequest) collectPDelayResp(resp *ptp.PDelayResp, addr netip.Addr, rxts time.Time) {
	if resp.RequestingPortIdentity != r.requester {
		return
	}
	r.Lock()
	defer r.Unlock()
	res := r.result(addr)
	res.CorrectionFieldReq = resp.CorrectionField.Duration()
	res.T2 = resp.RequestReceiptTimestamp.Time()
	res.T4 = rxts
}

// collectPDelayRespFollowUp records T3 and the response-path correction field
func (r *pingRequest) collectPDelayRespFollowUp(resp *ptp.PDelayRespFollowUp, addr netip.Addr) {
	if resp.RequestingPortIdentity != r.requester {
		return
	}
	r.Lock()
	defer r.Unlock()
	res := r.result(addr)
	res.CorrectionFieldResp = resp.CorrectionField.Duration()
	res.T3 = resp.ResponseOriginTimestamp.Time()
	res.SWRTT = time.Since(r.sentAt)
	r.complete()
}
