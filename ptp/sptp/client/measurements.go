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
	t1 time.Time     // departure time of Sync packet from GM
	t2 time.Time     // arrival time of Sync packet on OC
	t3 time.Time     // departure time of DelayReq from OC
	t4 time.Time     // arrival time of DelayReq packet on GM
	c2 time.Duration // correctionFiled of DelayReq
	c1 time.Duration // correctionField of Sync
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
	BadDelay          bool
}

// measurements abstracts away tracking and calculation of various packet timestamps
type measurements struct {
	sync.Mutex
	cfg              *MeasurementConfig
	currentUTCoffset time.Duration
	data             map[uint16]*mData
	lastData         *mData
	announce         ptp.Announce
	delaysWindow     *slidingWindow
	pathDelay        time.Duration
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
		m.data[seq] = &mData{t2: ts, c1: correction}
	}
}

func (m *measurements) addT1(seq uint16, ts time.Time) {
	m.Lock()
	defer m.Unlock()
	v, found := m.data[seq]
	if found {
		v.t1 = ts
	} else {
		m.data[seq] = &mData{t1: ts}
	}
}
func (m *measurements) addCF2(seq uint16, correction time.Duration) {
	m.Lock()
	defer m.Unlock()
	v, found := m.data[seq]
	if found {
		v.c2 = correction
	} else {
		m.data[seq] = &mData{c2: correction}
	}
}

func (m *measurements) addT3(seq uint16, ts time.Time) {
	m.Lock()
	defer m.Unlock()
	v, found := m.data[seq]
	if found {
		v.t3 = ts
	} else {
		m.data[seq] = &mData{t3: ts}
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
		m.data[seq] = &mData{t4: ts}
	}
}

// delay evaluates the latest path delay and applies filter logic
// It returns false if delay is bad and wasn't used
func (m *measurements) delay(newDelay time.Duration) bool {
	lastDelay := m.delaysWindow.lastSample()
	maxPathDelay := time.Duration(m.cfg.PathDelayDiscardMultiplier) * m.pathDelay
	// we want to have at least one sample recorded, even if it doesn't meet the filter, otherwise we'll never sync
	if math.IsNaN(lastDelay) || !m.cfg.PathDelayDiscardFilterEnabled {
		m.applyDelay(newDelay)
		return true
	}

	// Filter territory
	if newDelay < m.cfg.PathDelayDiscardBelow {
		// Discard below min from the beginning
		log.Warningf("(%s) low path delay %v is not in (%v, %v) - filtered out", m.announce.GrandmasterIdentity, newDelay, m.cfg.PathDelayDiscardBelow, maxPathDelay)
		return false
	} else if newDelay > m.cfg.PathDelayDiscardFrom && newDelay > maxPathDelay && maxPathDelay > m.cfg.PathDelayDiscardBelow && m.delaysWindow.Full() {
		// Ignore spikes above maxPathDelay starting from m.cfg.PathDelayDiscardFrom
		log.Warningf("(%s) high path delay %v is not in (%v, %v) - filtered out", m.announce.GrandmasterIdentity, newDelay, m.cfg.PathDelayDiscardBelow, maxPathDelay)
		return false
	} else if m.lastData != nil && (m.lastData.c1 < 0 || m.lastData.c2 < 0) {
		// Ignore negative CF
		log.Warningf("(%s) bad correction fields: CF1 (sync): %v, CF2 (announce): %v - filtered out", m.announce.GrandmasterIdentity, m.lastData.c1, m.lastData.c2)
		return false
	}

	m.applyDelay(newDelay)
	return true
}

func (m *measurements) applyDelay(newDelay time.Duration) {
	m.delaysWindow.add(float64(newDelay))

	switch m.cfg.PathDelayFilter {
	case FilterMedian:
		m.pathDelay = time.Duration(m.delaysWindow.median())
	case FilterMean:
		m.pathDelay = time.Duration(m.delaysWindow.mean())
	default:
		m.pathDelay = newDelay
	}
}

// we take last complete sample of sync/followup data and last complete sample of delay req/resp data
// to calculate delay and offset
func (m *measurements) latest() (*MeasurementResult, error) {
	m.Lock()
	defer m.Unlock()
	m.lastData = nil
	for _, v := range m.data {
		if !v.Complete() {
			continue
		}
		if m.lastData == nil || v.t2.After(m.lastData.t2) {
			m.lastData = v
		}
	}
	if m.lastData == nil {
		return nil, errNotEnoughData
	}
	// offset = ((t2 − t1 − c1) − (t4 − t3 − c2))/2
	// delay = ((t2 − t1 − c1) + (t4 − t3 − c2))/2
	C2SDelay := m.lastData.t4.Sub(m.lastData.t3) - m.lastData.c2
	S2CDelay := m.lastData.t2.Sub(m.lastData.t1) - m.lastData.c1
	newDelay := (C2SDelay + S2CDelay) / 2
	badDelay := !m.delay(newDelay)
	offset := S2CDelay - m.pathDelay
	// or this expression of same formula
	// offset := (S2CDelay - C2SDelay)/2
	return &MeasurementResult{
		Delay:             m.pathDelay,
		Offset:            offset,
		S2CDelay:          S2CDelay,
		C2SDelay:          C2SDelay,
		CorrectionFieldRX: m.lastData.c1,
		CorrectionFieldTX: m.lastData.c2,
		Timestamp:         m.lastData.t2,
		T1:                m.lastData.t1,
		T2:                m.lastData.t2,
		T3:                m.lastData.t3,
		T4:                m.lastData.t4,
		Announce:          m.announce,
		BadDelay:          badDelay,
	}, nil
}

func (m *measurements) cleanup() {
	m.Lock()
	defer m.Unlock()
	clear(m.data)
}

func newMeasurements(cfg *MeasurementConfig) *measurements {
	return &measurements{
		cfg:          cfg,
		data:         map[uint16]*mData{},
		delaysWindow: newSlidingWindow(cfg.PathDelayFilterLength),
	}
}
