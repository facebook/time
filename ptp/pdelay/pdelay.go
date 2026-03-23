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

CorrectionFields compensate for residence time in Transparent Clocks:
  - CFReq: from PDelay_Resp (request path: requester→responder)
  - CFResp: from PDelay_Resp_Follow_Up (response path: responder→requester)

Path delay = ((T2 - T1 - CFReq) + (T4 - T3 - CFResp)) / 2
Offset = ((T2 - T1 - CFReq) - (T4 - T3 - CFResp)) / 2
*/
package pdelay

import (
	"net/netip"
	"time"
)

// Result represents the result of a peer delay measurement
type Result struct {
	// Requester is the initiator of the measurement (local host name)
	Requester string
	// Responder is the target of the measurement (remote host)
	Responder netip.Addr
	// T1 is the Pdelay_Req departure time at requester
	T1 time.Time
	// T2 is the Pdelay_Req arrival time at responder
	T2 time.Time
	// T3 is the Pdelay_Resp departure time at responder
	T3 time.Time
	// T4 is the Pdelay_Resp arrival time at requester
	T4 time.Time
	// CorrectionFieldReq is the CF from PDelay_Resp (request path: requester→responder)
	CorrectionFieldReq time.Duration
	// CorrectionFieldResp is the CF from PDelay_Resp_Follow_Up (response path: responder→requester)
	CorrectionFieldResp time.Duration
	// RequesterGM is the best GM identity of the requester
	RequesterGM string
	// Timestamp is when this measurement was taken
	Timestamp time.Time
	// Error contains any error that occurred during measurement
	Error error
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
