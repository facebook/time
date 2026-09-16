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
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/facebook/time/ptp/pdelay"
	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/stretchr/testify/require"
)

// a peer reporting us 3us off, which is past the threshold these tests use.
// Each needs its own responder: Rack counts distinct peers, not exchanges.
func peerResult(n byte) *pdelay.Result {
	base := time.Now()
	const offset = 3 * time.Microsecond
	return &pdelay.Result{
		Responder: netip.AddrFrom4([4]byte{10, 0, 0, n}),
		T1:        base,
		T2:        base.Add(100*time.Nanosecond + offset),
		T3:        base.Add(200 * time.Nanosecond),
		T4:        base.Add(300*time.Nanosecond - offset),
		Timestamp: base,
	}
}

func TestNewCorrector(t *testing.T) {
	require.Nil(t, newCorrector(AsymmetryConfig{}), "disabled yields no corrector")

	for _, tt := range []struct {
		simple bool
		want   string
	}{
		{true, "simple"},
		{false, "complex"},
	} {
		c := newCorrector(AsymmetryConfig{AsymmetryCorrectionEnabled: true, Simple: tt.simple})
		require.NotNil(t, c)
		require.Equal(t, tt.want, c.Name())
	}
}

func TestGetAlternateResponsePortTLV(t *testing.T) {
	require.Nil(t, getAlternateResponsePortTLV(nil))
	require.Nil(t, getAlternateResponsePortTLV(&Client{}), "no delay request")
	require.Nil(t, getAlternateResponsePortTLV(&Client{delayRequest: &ptp.SyncDelayReq{}}), "no TLV")

	c := &Client{delayRequest: ReqDelay(ptp.ClockIdentity(1), 1)}
	require.NotNil(t, getAlternateResponsePortTLV(c))
}

func announceResult(offset time.Duration, class ptp.ClockClass, badDelay bool) *RunResult {
	r := &RunResult{Measurement: &MeasurementResult{Offset: offset, BadDelay: badDelay}}
	r.Measurement.Announce.GrandmasterClockQuality.ClockClass = class
	return r
}

func TestNewGM(t *testing.T) {
	client := &Client{delayRequest: ReqDelay(ptp.ClockIdentity(1), 1)}

	for _, tt := range []struct {
		name      string
		result    *RunResult
		answered  bool
		judgeable bool
	}{
		{"good measurement", announceResult(5*time.Microsecond, ptp.ClockClass6, false), true, true},
		{"bad delay", announceResult(5*time.Microsecond, ptp.ClockClass6, true), true, false},
		{"not a clock source", announceResult(5*time.Microsecond, ptp.ClockClass52, false), true, false},
		{"no measurement", &RunResult{}, true, false},
		{"no result", nil, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := newGM(client, tt.result)
			require.Equal(t, tt.answered, g.Answered, "a backoff result still answers for this tick")
			require.Equal(t, tt.judgeable, g.Judgeable, "an answer we cannot judge is not silence")
		})
	}
}

// a corrector records the decision; only the adapter touches the delay request
func TestApplyPortActions(t *testing.T) {
	client := &Client{delayRequest: ReqDelay(ptp.ClockIdentity(1), 1)}
	tlv := getAlternateResponsePortTLV(client)

	g := newGM(client, nil)
	g.MovePort()
	require.Zero(t, tlv.Offset, "deciding must not mutate the delay request")

	applyPortActions(client, g)
	require.Equal(t, uint16(1), tlv.Offset)

	g = newGM(client, nil)
	g.ResetPort()
	applyPortActions(client, g)
	require.Zero(t, tlv.Offset)

	require.NotPanics(t, func() { applyPortActions(&Client{}, newGM(client, nil)) }, "no TLV to apply to")
}

// a malformed packet can name a server we never configured
func TestCorrectAsymmetryUnknownServer(t *testing.T) {
	known := netip.MustParseAddr("192.168.0.10")
	unknown := netip.MustParseAddr("192.168.0.99")
	p := &SPTP{
		clients:   map[netip.Addr]*Client{known: {delayRequest: ReqDelay(ptp.ClockIdentity(1), 1)}},
		corrector: newCorrector(AsymmetryConfig{AsymmetryCorrectionEnabled: true, AsymmetryThreshold: time.Microsecond}),
	}
	results := map[netip.Addr]*RunResult{unknown: announceResult(9*time.Microsecond, ptp.ClockClass6, false)}

	require.NotPanics(t, func() { p.correctAsymmetry(results, known) })
}

// the corrector works on a copy, so its verdict has to land back on the client
func TestCorrectAsymmetryWritesBack(t *testing.T) {
	best := netip.MustParseAddr("192.168.0.10")
	other := netip.MustParseAddr("192.168.0.11")
	p := &SPTP{
		clients: map[netip.Addr]*Client{
			best:  {delayRequest: ReqDelay(ptp.ClockIdentity(1), 1)},
			other: {delayRequest: ReqDelay(ptp.ClockIdentity(2), 1)},
		},
		corrector: newCorrector(AsymmetryConfig{
			AsymmetryCorrectionEnabled: true,
			AsymmetryThreshold:         time.Microsecond,
			MaxPortChanges:             4,
		}),
	}
	results := map[netip.Addr]*RunResult{
		best:  announceResult(0, ptp.ClockClass6, false),
		other: announceResult(9*time.Microsecond, ptp.ClockClass6, false),
	}

	require.Equal(t, 1, p.correctAsymmetry(results, best))
	require.True(t, p.clients[other].asymmetric, "verdict must reach the client")
	require.Equal(t, 1, p.clients[other].asymmetryCounter, "streak must reach the client")
}

// the complex path clears stale state on GMs it stops searching, so a GM that
// produced no result this tick still has to be visible to it
func TestCorrectAsymmetryCoversSilentClients(t *testing.T) {
	best := netip.MustParseAddr("192.168.0.10")
	silent := netip.MustParseAddr("192.168.0.11")
	loud := netip.MustParseAddr("192.168.0.12")

	p := &SPTP{
		clients: map[netip.Addr]*Client{
			best:   {delayRequest: ReqDelay(ptp.ClockIdentity(1), 1)},
			silent: {delayRequest: ReqDelay(ptp.ClockIdentity(2), 1), asymmetric: true},
			loud:   {delayRequest: ReqDelay(ptp.ClockIdentity(3), 1)},
		},
		corrector: newCorrector(AsymmetryConfig{
			AsymmetryCorrectionEnabled: true,
			AsymmetryThreshold:         time.Microsecond,
			MaxPortChanges:             2,
		}),
	}
	// the silent GM was left mid-search with an offset past the limit
	getAlternateResponsePortTLV(p.clients[silent]).Offset = 5
	getAlternateResponsePortTLV(p.clients[loud]).Offset = 5

	// only the loud GM reports; the silent one is absent from results entirely
	p.correctAsymmetry(map[netip.Addr]*RunResult{
		best: announceResult(0, ptp.ClockClass6, false),
		loud: announceResult(9*time.Microsecond, ptp.ClockClass6, false),
	}, best)

	require.Zero(t, getAlternateResponsePortTLV(p.clients[silent]).Offset,
		"a GM with no result must still have its stale port offset cleared")
	require.False(t, p.clients[silent].asymmetric, "and its stale flag cleared")
}

func TestNewCorrectorRack(t *testing.T) {
	c := newCorrector(AsymmetryConfig{AsymmetryCorrectionEnabled: true, Rack: true})
	require.Equal(t, "rack", c.Name())

	// rack wins over simple, so a host cannot silently run two arms
	c = newCorrector(AsymmetryConfig{AsymmetryCorrectionEnabled: true, Rack: true, Simple: true})
	require.Equal(t, "rack", c.Name())
}

func TestObservePeersOnlyFeedsRack(t *testing.T) {
	results := pdelay.Results{peerResult(1), peerResult(2), peerResult(3)}
	best := netip.MustParseAddr("192.168.0.10")
	clients := map[netip.Addr]*Client{best: {delayRequest: ReqDelay(ptp.ClockIdentity(1), 1)}}

	p := &SPTP{
		clients: clients,
		corrector: newCorrector(AsymmetryConfig{
			AsymmetryCorrectionEnabled: true,
			Rack:                       true,
			AsymmetryThreshold:         time.Microsecond,
			MaxPortChanges:             4,
		}),
	}
	p.observePeers(results)
	require.Equal(t, 1, p.correctAsymmetry(map[netip.Addr]*RunResult{best: announceResult(0, ptp.ClockClass6, false)}, best))

	// simple acts on grandmasters only, so a peers-only round is a no-op for it
	q := &SPTP{
		clients:   map[netip.Addr]*Client{best: {delayRequest: ReqDelay(ptp.ClockIdentity(1), 1)}},
		corrector: newCorrector(AsymmetryConfig{AsymmetryCorrectionEnabled: true, Simple: true, AsymmetryThreshold: time.Microsecond}),
	}
	require.NotPanics(t, func() { q.observePeers(results) })
}

func TestObservePeersSkipsUnusable(t *testing.T) {
	best := netip.MustParseAddr("192.168.0.10")
	bad := peerResult(1)
	bad.Error = errors.New("incomplete response")

	p := &SPTP{
		clients:   map[netip.Addr]*Client{best: {delayRequest: ReqDelay(ptp.ClockIdentity(1), 1)}},
		corrector: newCorrector(AsymmetryConfig{AsymmetryCorrectionEnabled: true, Rack: true, AsymmetryThreshold: time.Microsecond}),
	}
	p.observePeers(pdelay.Results{bad, nil})
	// one bad exchange must not become a reading, let alone a zero one
	require.Zero(t, p.correctAsymmetry(map[netip.Addr]*RunResult{best: announceResult(0, ptp.ClockClass6, false)}, best))
}
