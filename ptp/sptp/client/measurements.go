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
	"fmt"
	"math"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	ptp "github.com/facebook/time/ptp/protocol"
)

var errNotEnoughData = fmt.Errorf("not enough data")

// Supported path delay filters
const (
	FilterNone   = ""
	FilterMedian = "median"
	FilterMean   = "mean"
)

// mData is a single measured raw data of GM to OC communication
type mData struct {
	seq uint16
	t1  time.Time     // departure time of Sync packet from GM
	t2  time.Time     // arrival time of Sync packet on OC
	t3  time.Time     // departure time of DelayReq from OC
	t4  time.Time     // arrival time of DelayReq packet on GM
	c2  time.Duration // // correctionFiled of DelayReq
	c1  time.Duration // correctionField of Sync
}

func (d *mData) Complete() bool {
	return !d.t1.IsZero() && !d.t2.IsZero() && !d.t3.IsZero() && !d.t4.IsZero()
}

// MeasurementResult is a single measured datapoint
type MeasurementResult struct {
	Delay             time.Duration
	Offset            time.Duration
	S2CDelay          time.Duration
	C2SDelay          time.Duration
	CorrectionFieldRX time.Duration
	CorrectionFieldTX time.Duration
	Timestamp         time.Time
	Announce          ptp.Announce
	T1                time.Time
	T2                time.Time
	T3                time.Time
	T4                time.Time
}

// measurements abstracts away tracking and calculation of various packet timestamps
type measurements struct {
	sync.Mutex

	cfg              *MeasurementConfig
	currentUTCoffset time.Duration
	data             map[uint16]*mData
	announce         ptp.Announce
	delaysWindow     *slidingWindow
}

func (m *measurements) addAnnounce(announce ptp.Announce) {
	m.Lock()
	defer m.Unlock()
	m.announce = announce
}

func (m *measurements) addT2andCF1(seq uint16, ts time.Time, correction time.Duration) {
	m.Lock()
	defer m.Unlock()
	v, found := m.data[seq]
	if found {
		v.t2 = ts
		v.c1 = correction
	} else {
		m.data[seq] = &mData{seq: seq, t2: ts, c1: correction}
	}
}

func (m *measurements) addT1(seq uint16, ts time.Time) {
	m.Lock()
	defer m.Unlock()
	v, found := m.data[seq]
	if found {
		v.t1 = ts
	} else {
		m.data[seq] = &mData{seq: seq, t1: ts}
	}
}
func (m *measurements) addCF2(seq uint16, correction time.Duration) {
	m.Lock()
	defer m.Unlock()
	v, found := m.data[seq]
	if found {
		v.c2 = correction
	} else {
		m.data[seq] = &mData{seq: seq, c2: correction}
	}
}

func (m *measurements) addT3(seq uint16, ts time.Time) {
	m.Lock()
	defer m.Unlock()
	v, found := m.data[seq]
	if found {
		v.t3 = ts
	} else {
		m.data[seq] = &mData{seq: seq, t3: ts}
	}
}

// addDelayResp stores ts and seq of DELAY_RESP packet and updates history with latest measurements
func (m *measurements) addT4(seq uint16, ts time.Time) {
	m.Lock()
	defer m.Unlock()
	v, found := m.data[seq]
	if found {
		v.t4 = ts
	} else {
		m.data[seq] = &mData{seq: seq, t4: ts}
	}
}

func (m *measurements) delay(newDelay time.Duration) time.Duration {
	lastDelay := m.delaysWindow.lastSample()
	// we want to have at least one sample recorded, even if it doesn't meet the filter, otherwise we'll never sync
	if !math.IsNaN(lastDelay) && (m.cfg.PathDelayDiscardFilterEnabled && m.delaysWindow.Full() && (newDelay < m.cfg.PathDelayDiscardBelow || newDelay > m.cfg.PathDelayDiscardAbove)) {
		log.Warningf("(%s) bad path delay %v is not in (%v, %v) - filtered out", m.announce.GrandmasterIdentity, newDelay, m.cfg.PathDelayDiscardBelow, m.cfg.PathDelayDiscardAbove)
	} else {
		m.delaysWindow.add(float64(newDelay))
	}

	switch m.cfg.PathDelayFilter {
	case FilterMedian:
		return time.Duration(m.delaysWindow.median())
	case FilterMean:
		return time.Duration(m.delaysWindow.mean())
	default:
		return newDelay
	}
}

// we take last complete sample of sync/followup data and last complete sample of delay req/resp data
// to calculate delay and offset
func (m *measurements) latest() (*MeasurementResult, error) {
	m.Lock()
	defer m.Unlock()
	var lastData *mData
	for _, v := range m.data {
		if !v.Complete() {
			continue
		}
		if lastData == nil || v.t2.After(lastData.t2) {
			lastData = v
		}
	}
	if lastData == nil {
		return nil, errNotEnoughData
	}
	// offset = ((t2 − t1 − c1) − (t4 − t3 − c2))/2
	// delay = ((t2 − t1 − c1) + (t4 − t3 − c2))/2
	C2SDelay := lastData.t4.Sub(lastData.t3) - lastData.c2
	S2CDelay := lastData.t2.Sub(lastData.t1) - lastData.c1
	newDelay := (C2SDelay + S2CDelay) / 2
	delay := m.delay(newDelay)
	offset := S2CDelay - delay
	// or this expression of same formula
	// offset := (S2CDelay - C2SDelay)/2
	return &MeasurementResult{
		Delay:             delay,
		Offset:            offset,
		S2CDelay:          S2CDelay,
		C2SDelay:          C2SDelay,
		CorrectionFieldRX: lastData.c1,
		CorrectionFieldTX: lastData.c2,
		Timestamp:         lastData.t2,
		T1:                lastData.t1,
		T2:                lastData.t2,
		T3:                lastData.t3,
		T4:                lastData.t4,
		Announce:          m.announce,
	}, nil
}

func (m *measurements) cleanup() {
	m.Lock()
	defer m.Unlock()
	m.data = map[uint16]*mData{}
}

func newMeasurements(cfg *MeasurementConfig) *measurements {
	return &measurements{
		cfg:          cfg,
		data:         map[uint16]*mData{},
		delaysWindow: newSlidingWindow(cfg.PathDelayFilterLength),
	}
}
