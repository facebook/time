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

// mData is a single measured raw data
type mData struct {
	seq       uint16
	sendTS    time.Time
	receiveTS time.Time
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
	serverToClient   map[uint16]*mData
	clientToServer   map[uint16]*mData
}

// addSync stores ts and seq of SYNC packet
func (m *measurements) addSync(seq uint16, ts time.Time) {
	m.Lock()
	defer m.Unlock()
	v, found := m.serverToClient[seq]
	if found {
		v.receiveTS = ts
	} else {
		m.serverToClient[seq] = &mData{seq: seq, receiveTS: ts}
	}
}

// addFollowUp stores ts and seq of FOLLOW_UP packet
func (m *measurements) addFollowUp(seq uint16, ts time.Time) {
	m.Lock()
	defer m.Unlock()
	v, found := m.serverToClient[seq]
	if found {
		v.sendTS = ts
	} else {
		m.serverToClient[seq] = &mData{seq: seq, sendTS: ts}
	}
}

// addDelayReq stores ts and seq of DELAY_REQ packet
func (m *measurements) addDelayReq(seq uint16, ts time.Time) {
	m.Lock()
	defer m.Unlock()
	v, found := m.clientToServer[seq]
	if found {
		v.sendTS = ts
	} else {
		m.clientToServer[seq] = &mData{seq: seq, sendTS: ts}
	}
}

// addDelayResp stores ts and seq of DELAY_RESP packet and updates history with latest measurements
func (m *measurements) addDelayResp(seq uint16, ts time.Time) {
	m.Lock()
	defer m.Unlock()
	v, found := m.clientToServer[seq]
	if found {
		v.receiveTS = ts
	} else {
		m.clientToServer[seq] = &mData{seq: seq, receiveTS: ts}
	}
}

// we take last complete sample of sync/followup data and last complete sample of delay req/resp data
// to calculate delay and offset
func (m *measurements) latest() (*MeasurementResult, error) {
	var lastServerToClient *mData
	var lastClientToServer *mData
	for _, v := range m.serverToClient {
		if v.receiveTS.IsZero() || v.sendTS.IsZero() {
			continue
		}
		if lastServerToClient == nil || v.receiveTS.After(lastServerToClient.receiveTS) {
			lastServerToClient = v
		}
	}
	for _, v := range m.clientToServer {
		if v.receiveTS.IsZero() || v.sendTS.IsZero() {
			continue
		}
		if lastClientToServer == nil || v.receiveTS.After(lastClientToServer.receiveTS) {
			lastClientToServer = v
		}
	}
	if lastServerToClient == nil {
		return nil, fmt.Errorf("no sync/followup data yet")
	}
	if lastClientToServer == nil {
		return nil, fmt.Errorf("no delay data yet")
	}
	clientToServerDiff := lastClientToServer.receiveTS.Sub(lastClientToServer.sendTS)
	serverToClientDiff := lastServerToClient.receiveTS.Sub(lastServerToClient.sendTS)
	delay := (clientToServerDiff + serverToClientDiff) / 2
	offset := serverToClientDiff - delay
	// or this expression of same formula
	// offset := (serverToClientDiff - clientToServerDiff)/2
	return &MeasurementResult{
		Delay:              delay,
		Offset:             offset,
		ServerToClientDiff: serverToClientDiff,
		ClientToServerDiff: clientToServerDiff,
		Timestamp:          lastClientToServer.receiveTS,
	}, nil
}

func newMeasurements() *measurements {
	return &measurements{
		serverToClient: map[uint16]*mData{},
		clientToServer: map[uint16]*mData{},
	}
}
