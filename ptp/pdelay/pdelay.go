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

/*
Package pdelay implements PTP Peer Delay measurement for in-rack linearizability checks.

The peer delay mechanism uses Pdelay_Req, Pdelay_Resp, and Pdelay_Resp_Follow_Up
messages to measure the path delay between two PTP nodes. This is particularly
useful for measuring time differences between hosts in the same rack where
symmetric paths can be guaranteed.

Timestamp exchange:
  - T1: Pdelay_Req departure time (requester)
  - T2: Pdelay_Req arrival time (responder)
  - T3: Pdelay_Resp departure time (responder)
  - T4: Pdelay_Resp arrival time (requester)

CorrectionFields compensate for residence time in Transparent Clocks. The
Pdelay_Req never reaches the requester, so the responder relays the correction it
accumulated into the Pdelay_Resp_Follow_Up (IEEE 1588 11.4.2) and sends the
Pdelay_Resp with a zero correctionField, leaving that field to accrue the
response path in flight:
  - CFReq: from PDelay_Resp_Follow_Up (request path: requester->responder)
  - CFResp: from PDelay_Resp (response path: responder->requester)

Path delay = ((T2 - T1 - CFReq) + (T4 - T3 - CFResp)) / 2
Offset = ((T2 - T1 - CFReq) - (T4 - T3 - CFResp)) / 2
*/
package pdelay

import (
	"errors"
	"net/netip"
	"time"
)

// ErrIncompleteResponse marks a Result whose timestamps never all arrived, for
// which Offset and PathDelay both return 0. UnmarshalJSON restores it by
// identity, so errors.Is holds for a Result read back over the wire too.
var ErrIncompleteResponse = errors.New("incomplete response")

// Result represents the result of a peer delay measurement
type Result struct {
	// Responder is the target of the measurement (remote host)
	Responder netip.Addr `json:"responder"`
	// T1 is the Pdelay_Req departure time at requester
	T1 time.Time `json:"t1"`
	// T2 is the Pdelay_Req arrival time at responder
	T2 time.Time `json:"t2"`
	// T3 is the Pdelay_Resp departure time at responder
	T3 time.Time `json:"t3"`
	// T4 is the Pdelay_Resp arrival time at requester
	T4 time.Time `json:"t4"`
	// CorrectionFieldReq is the CF from PDelay_Resp_Follow_Up (request path: requester->responder)
	CorrectionFieldReq time.Duration `json:"cf_req"`
	// CorrectionFieldResp is the CF from PDelay_Resp (response path: responder->requester)
	CorrectionFieldResp time.Duration `json:"cf_resp"`
	// Timestamp is when this measurement was taken
	Timestamp time.Time `json:"timestamp"`
	// SWRTT is the software-measured round trip time. Unlike the hardware
	// timestamps above it cannot be derived from T1..T4, and ptping reports it.
	SWRTT time.Duration `json:"sw_rtt,omitzero"`
	// Error contains any error that occurred during measurement
	Error error `json:"-"`
}

// PathDelay calculates the mean path delay between requester and responder
// PathDelay = ((T2 - T1 - CFReq) + (T4 - T3 - CFResp)) / 2
// The CorrectionFields compensate for residence time in Transparent Clocks
func (r *Result) PathDelay() time.Duration {
	if !r.Valid() {
		return 0
	}
	forward := (r.T2.Sub(r.T1) - r.CorrectionFieldReq)
	backward := (r.T4.Sub(r.T3) - r.CorrectionFieldResp)
	return (forward + backward) / 2
}

// Offset calculates the time offset between requester and responder
// Offset = ((T2 - T1 - CFReq) - (T4 - T3 - CFResp)) / 2
// The CorrectionFields compensate for residence time in Transparent Clocks
func (r *Result) Offset() time.Duration {
	if !r.Valid() {
		return 0
	}
	forward := (r.T2.Sub(r.T1) - r.CorrectionFieldReq)
	backward := (r.T4.Sub(r.T3) - r.CorrectionFieldResp)
	return (forward - backward) / 2
}

// Valid returns true if all timestamps are set
func (r *Result) Valid() bool {
	return !r.T1.IsZero() && !r.T2.IsZero() && !r.T3.IsZero() && !r.T4.IsZero()
}
