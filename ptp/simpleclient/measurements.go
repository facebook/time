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

package simpleclient

import (
	"fmt"
	"sync"
	"time"
)

// mDataSync is a single measured raw data of GM to OC communication
type mDataSync struct {
	seq uint16
	t1  time.Time     // departure time of Sync packet from GM
	t2  time.Time     // arrival time of Sync packet on OC
	c1  time.Duration // correctionField of Sync
	c2  time.Duration // correctionFiled of FollowUp
}

// mDataDelay is a single measured raw data of OC to GM communication
type mDataDelay struct {
	seq uint16
	t3  time.Time     // departure time of DelayReq from OC
	t4  time.Time     // arrival time of DelayReq packet on GM
	c3  time.Duration // // correctionFiled of DelayReq
}

// MeasurementResult is a single measured datapoint
type MeasurementResult struct {
	Delay              time.Duration
	Offset             time.Duration
	ServerToClientDiff time.Duration
	ClientToServerDiff time.Duration
	Timestamp          time.Time
}

// measurements abstracts away tracking and calculation of various packet timestamps
type measurements struct {
	sync.Mutex

	currentUTCoffset time.Duration
	serverToClient   map[uint16]*mDataSync
	clientToServer   map[uint16]*mDataDelay
}

// addSync stores ts and seq of SYNC packet
func (m *measurements) addSync(seq uint16, ts time.Time, correction time.Duration) {
	m.Lock()
	defer m.Unlock()
	v, found := m.serverToClient[seq]
	if found {
		v.t2 = ts
		v.c1 = correction
	} else {
		m.serverToClient[seq] = &mDataSync{seq: seq, t2: ts, c1: correction}
	}
}

// addFollowUp stores ts and seq of FOLLOW_UP packet
func (m *measurements) addFollowUp(seq uint16, ts time.Time, correction time.Duration) {
	m.Lock()
	defer m.Unlock()
	v, found := m.serverToClient[seq]
	if found {
		v.t1 = ts
		v.c2 = correction
	} else {
		m.serverToClient[seq] = &mDataSync{seq: seq, t1: ts, c2: correction}
	}
}

// addDelayReq stores ts and seq of DELAY_REQ packet
func (m *measurements) addDelayReq(seq uint16, ts time.Time) {
	m.Lock()
	defer m.Unlock()
	v, found := m.clientToServer[seq]
	if found {
		v.t3 = ts
	} else {
		m.clientToServer[seq] = &mDataDelay{seq: seq, t3: ts}
	}
}

// addDelayResp stores ts and seq of DELAY_RESP packet and updates history with latest measurements
func (m *measurements) addDelayResp(seq uint16, ts time.Time, correction time.Duration) {
	m.Lock()
	defer m.Unlock()
	v, found := m.clientToServer[seq]
	if found {
		v.t4 = ts
		v.c3 = correction
	} else {
		m.clientToServer[seq] = &mDataDelay{seq: seq, t4: ts, c3: correction}
	}
}

// we take last complete sample of sync/followup data and last complete sample of delay req/resp data
// to calculate delay and offset
func (m *measurements) latest() (*MeasurementResult, error) {
	var lastServerToClient *mDataSync
	var lastClientToServer *mDataDelay
	for _, v := range m.serverToClient {
		if v.t1.IsZero() || v.t2.IsZero() {
			continue
		}
		if lastServerToClient == nil || v.t2.After(lastServerToClient.t2) {
			lastServerToClient = v
		}
	}
	for _, v := range m.clientToServer {
		if v.t3.IsZero() || v.t4.IsZero() {
			continue
		}
		if lastClientToServer == nil || v.t4.After(lastClientToServer.t4) {
			lastClientToServer = v
		}
	}
	if lastServerToClient == nil {
		return nil, fmt.Errorf("no sync/followup data yet")
	}
	if lastClientToServer == nil {
		return nil, fmt.Errorf("no delay data yet")
	}
	// offset = ((t2 − t1 − c1 − c2) − (t4 − t3 − c3))/2
	// delay = ((t2 − t1 − c1 − c2) + (t4 − t3 − c3))/2
	clientToServerDiff := lastClientToServer.t4.Sub(lastClientToServer.t3) - lastClientToServer.c3
	serverToClientDiff := lastServerToClient.t2.Sub(lastServerToClient.t1) - lastServerToClient.c1 - lastServerToClient.c2
	delay := (clientToServerDiff + serverToClientDiff) / 2
	offset := serverToClientDiff - delay
	// or this expression of same formula
	// offset := (serverToClientDiff - clientToServerDiff)/2
	return &MeasurementResult{
		Delay:              delay,
		Offset:             offset,
		ServerToClientDiff: serverToClientDiff,
		ClientToServerDiff: clientToServerDiff,
		Timestamp:          lastClientToServer.t4,
	}, nil
}

func newMeasurements() *measurements {
	return &measurements{
		serverToClient: map[uint16]*mDataSync{},
		clientToServer: map[uint16]*mDataDelay{},
	}
}
