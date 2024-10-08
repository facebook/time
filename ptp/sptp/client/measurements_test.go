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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMeasurementsFullRun(t *testing.T) {
	mcfg := &MeasurementConfig{}
	m := newMeasurements(mcfg)
	var seq uint16 = 1
	t.Run("symmetrical delay, no offset", func(t *testing.T) {
		netDelay := 100 * time.Millisecond
		netDelayBack := netDelay

		// time when we sent out DELAY_REQ (T3), starting the exchange
		timeDelaySent, err := time.Parse(time.RFC3339, "2021-05-21T13:32:05+01:00")
		require.Nil(t, err)
		// time when DELAY_REQ was received by GM (T4)
		timeDelayReceived := timeDelaySent.Add(netDelayBack)

		// time when GM sent us SYNC in response to DELAY_REQ (T1)
		timeSyncSent := timeDelaySent.Add(10 * time.Millisecond)
		timeSyncReceived := timeSyncSent.Add(netDelay)

		// exchange
		m.addT3(seq, timeDelaySent)

		// we get sync back, taking note of T2 and receiving T4 and CF1 in payload

		// time when we received SYNC (T2)
		m.addT2andCF1(seq, timeSyncReceived, 0)
		// sync carries T4 as well
		m.addT4(seq, timeDelayReceived)

		// we get announce as well, with T1 and CF2

		// time when SYNC was actually sent by GM
		m.addT1(seq, timeSyncSent)
		m.addCF2(seq, 0)

		got, err := m.latest()
		require.Nil(t, err)
		want := &MeasurementResult{
			Delay:     netDelay,
			S2CDelay:  netDelay,
			C2SDelay:  netDelayBack,
			Offset:    0,
			Timestamp: timeSyncReceived,
			T1:        timeSyncSent,
			T2:        timeSyncReceived,
			T3:        timeDelaySent,
			T4:        timeDelayReceived,
		}
		require.Equal(t, want, got)
	})

	t.Run("asymmetrical delay, some offset", func(t *testing.T) {
		netDelay := 200 * time.Millisecond
		netDelayBack := 2 * netDelay

		// time when we sent out DELAY_REQ (T3), starting the exchange
		timeDelaySent, err := time.Parse(time.RFC3339, "2021-05-21T13:32:05+01:00")
		require.Nil(t, err)
		// time when DELAY_REQ was received by GM (T4)
		timeDelayReceived := timeDelaySent.Add(netDelayBack)

		// time when GM sent us SYNC in response to DELAY_REQ (T1)
		timeSyncSent := timeDelaySent.Add(10 * time.Millisecond)
		timeSyncReceived := timeSyncSent.Add(netDelay)

		// exchange
		m.addT3(seq, timeDelaySent)

		// we get sync back, taking note of T2 and receiving T4 and CF1 in payload

		// time when we received SYNC (T2)
		m.addT2andCF1(seq, timeSyncReceived, 0)
		// sync carries T4 as well
		m.addT4(seq, timeDelayReceived)

		// we get announce as well, with T1 and CF2

		// time when SYNC was actually sent by GM
		m.addT1(seq, timeSyncSent)
		m.addCF2(seq, 0)

		got, err := m.latest()
		require.Nil(t, err)
		want := &MeasurementResult{
			Delay:     300 * time.Millisecond,
			S2CDelay:  netDelay,
			C2SDelay:  netDelayBack,
			Offset:    -100 * time.Millisecond,
			Timestamp: timeSyncReceived,
			T1:        timeSyncSent,
			T2:        timeSyncReceived,
			T3:        timeDelaySent,
			T4:        timeDelayReceived,
		}
		require.Equal(t, want, got)
	})

	t.Run("asymmetrical delay, some offset and correction", func(t *testing.T) {
		netDelay := 200 * time.Millisecond
		netDelayBack := 2 * netDelay
		netCorrection := 6 * time.Microsecond
		netCorrectionBack := 4 * time.Microsecond

		// time when we sent out DELAY_REQ (T3), starting the exchange
		timeDelaySent, err := time.Parse(time.RFC3339, "2021-05-21T13:32:05+01:00")
		require.Nil(t, err)
		// time when DELAY_REQ was received by GM (T4)
		timeDelayReceived := timeDelaySent.Add(netDelayBack)

		// time when GM sent us SYNC in response to DELAY_REQ (T1)
		timeSyncSent := timeDelaySent.Add(10 * time.Millisecond)
		timeSyncReceived := timeSyncSent.Add(netDelay)

		// exchange
		m.addT3(seq, timeDelaySent)

		// we get sync back, taking note of T2 and receiving T4 and CF1 in payload

		// time when we received SYNC (T2)
		m.addT2andCF1(seq, timeSyncReceived, netCorrection)
		// sync carries T4 as well
		m.addT4(seq, timeDelayReceived)

		// we get announce as well, with T1 and CF2

		// time when SYNC was actually sent by GM
		m.addT1(seq, timeSyncSent)
		m.addCF2(seq, netCorrectionBack)

		got, err := m.latest()
		require.Nil(t, err)
		want := &MeasurementResult{
			Delay:             299995 * time.Microsecond,
			S2CDelay:          netDelay - netCorrection,
			C2SDelay:          netDelayBack - netCorrectionBack,
			Offset:            -100001 * time.Microsecond,
			CorrectionFieldRX: 6 * time.Microsecond,
			CorrectionFieldTX: 4 * time.Microsecond,
			Timestamp:         timeSyncReceived,
			T1:                timeSyncSent,
			T2:                timeSyncReceived,
			T3:                timeDelaySent,
			T4:                timeDelayReceived,
		}
		require.Equal(t, want, got)
	})
}

func TestMeasurementsPathDelayFilter(t *testing.T) {
	mcfg := &MeasurementConfig{
		PathDelayFilterLength:         4,
		PathDelayFilter:               FilterNone,
		PathDelayDiscardFilterEnabled: true,
		PathDelayDiscardBelow:         100 * time.Millisecond,
		PathDelayDiscardMultiplier:    1000,
	}
	m := newMeasurements(mcfg)
	var seq uint16 = 1
	netDelay := 200 * time.Millisecond
	netDelayBack := 2 * netDelay
	netCorrection := 6 * time.Microsecond
	netCorrectionBack := 4 * time.Microsecond

	// time when we sent out DELAY_REQ (T3), starting the exchange
	timeDelaySent, err := time.Parse(time.RFC3339, "2021-05-21T13:32:05+01:00")
	require.Nil(t, err)
	// time when DELAY_REQ was received by GM (T4)
	timeDelayReceived := timeDelaySent.Add(netDelayBack)

	// time when GM sent us SYNC in response to DELAY_REQ (T1)
	timeSyncSent := timeDelaySent.Add(10 * time.Millisecond)
	timeSyncReceived := timeSyncSent.Add(netDelay)

	// exchange
	m.addT3(seq, timeDelaySent)

	// we get sync back, taking note of T2 and receiving T4 and CF1 in payload

	// time when we received SYNC (T2)
	m.addT2andCF1(seq, timeSyncReceived, netCorrection)
	// sync carries T4 as well
	m.addT4(seq, timeDelayReceived)

	// we get announce as well, with T1 and CF2

	// time when SYNC was actually sent by GM
	m.addT1(seq, timeSyncSent)
	m.addCF2(seq, netCorrectionBack)

	got, err := m.latest()
	require.Nil(t, err)
	want := &MeasurementResult{
		Delay:             299995 * time.Microsecond,
		S2CDelay:          netDelay - netCorrection,
		C2SDelay:          netDelayBack - netCorrectionBack,
		Offset:            -100001 * time.Microsecond,
		CorrectionFieldRX: 6 * time.Microsecond,
		CorrectionFieldTX: 4 * time.Microsecond,
		Timestamp:         timeSyncReceived,
		T1:                timeSyncSent,
		T2:                timeSyncReceived,
		T3:                timeDelaySent,
		T4:                timeDelayReceived,
	}
	require.Equal(t, want, got, "initial measurements check")

	// now let's add more data so we see filtering work
	for i := 0; i < 5; i++ {
		seq++
		if i%2 == 0 {
			netDelay = 200 * time.Millisecond
			netDelayBack = 200 * time.Millisecond
		} else {
			netDelay = 150 * time.Millisecond
			netDelayBack = 250 * time.Millisecond
		}
		timeDelaySent = timeDelaySent.Add(time.Second)
		timeDelayReceived = timeDelaySent.Add(netDelayBack)
		timeSyncSent = timeDelaySent.Add(10 * time.Millisecond)
		timeSyncReceived = timeSyncSent.Add(netDelay)

		m.addT3(seq, timeDelaySent)
		m.addT2andCF1(seq, timeSyncReceived, netCorrection)
		m.addT4(seq, timeDelayReceived)
		m.addT1(seq, timeSyncSent)
		m.addCF2(seq, netCorrectionBack)
	}
	got, err = m.latest()
	require.Nil(t, err)
	want = &MeasurementResult{
		Delay:             199995 * time.Microsecond,
		S2CDelay:          netDelay - netCorrection,
		C2SDelay:          netDelayBack - netCorrectionBack,
		Offset:            -1 * time.Microsecond,
		CorrectionFieldRX: 6 * time.Microsecond,
		CorrectionFieldTX: 4 * time.Microsecond,
		Timestamp:         timeSyncReceived,
		T1:                timeSyncSent,
		T2:                timeSyncReceived,
		T3:                timeDelaySent,
		T4:                timeDelayReceived,
	}
	require.Equal(t, want, got, "measurements after 6 more exchanges")

	// now the same with sliding window filtering
	// nothing changes with median filter, as it was all stable
	m.cfg.PathDelayFilter = FilterMedian
	got, err = m.latest()
	require.Nil(t, err)
	want = &MeasurementResult{
		Delay:             199995 * time.Microsecond,
		S2CDelay:          netDelay - netCorrection,
		C2SDelay:          netDelayBack - netCorrectionBack,
		Offset:            -1 * time.Microsecond,
		CorrectionFieldRX: 6 * time.Microsecond,
		CorrectionFieldTX: 4 * time.Microsecond,
		Timestamp:         timeSyncReceived,
		T1:                timeSyncSent,
		T2:                timeSyncReceived,
		T3:                timeDelaySent,
		T4:                timeDelayReceived,
	}
	require.Equal(t, want, got, "measurements with median path delay filter")

	// mean filter
	m.cfg.PathDelayFilter = FilterMean
	got, err = m.latest()
	require.Nil(t, err)
	want = &MeasurementResult{
		Delay:             224995 * time.Microsecond,
		S2CDelay:          netDelay - netCorrection,
		C2SDelay:          netDelayBack - netCorrectionBack,
		Offset:            -25001 * time.Microsecond,
		CorrectionFieldRX: 6 * time.Microsecond,
		CorrectionFieldTX: 4 * time.Microsecond,
		Timestamp:         timeSyncReceived,
		T1:                timeSyncSent,
		T2:                timeSyncReceived,
		T3:                timeDelaySent,
		T4:                timeDelayReceived,
	}
	require.Equal(t, want, got, "measurements with mean path delay filter")

	// now add really bad sample so it gets dropped
	timeDelaySent = timeDelaySent.Add(time.Second)
	// simulate broken TX timestamp
	netDelayBack = -100 * time.Millisecond
	timeDelayReceived = timeDelaySent.Add(netDelayBack)
	timeSyncSent = timeDelayReceived.Add(10 * time.Millisecond)
	timeSyncReceived = timeSyncSent.Add(netDelay)

	m.addT3(seq, timeDelaySent)
	m.addT2andCF1(seq, timeSyncReceived, netCorrection)
	m.addT4(seq, timeDelayReceived)
	m.addT1(seq, timeSyncSent)
	m.addCF2(seq, netCorrectionBack)

	got, err = m.latest()
	require.Nil(t, err)
	want = &MeasurementResult{
		Delay:             224995 * time.Microsecond,
		S2CDelay:          netDelay - netCorrection,
		C2SDelay:          netDelayBack - netCorrectionBack,
		Offset:            -25001 * time.Microsecond,
		CorrectionFieldRX: 6 * time.Microsecond,
		CorrectionFieldTX: 4 * time.Microsecond,
		Timestamp:         timeSyncReceived,
		T1:                timeSyncSent,
		T2:                timeSyncReceived,
		T3:                timeDelaySent,
		T4:                timeDelayReceived,
	}
	require.Equal(t, want, got, "measurements with mean path delay filter and skipped path delay sample")

	m.cfg.PathDelayDiscardMultiplier = 3
	// now add really bad sample so it gets dropped
	timeDelaySent = timeDelaySent.Add(time.Second)
	// simulate broken RX timestamp
	netDelayBack = 400 * time.Millisecond
	timeDelayReceived = timeDelaySent.Add(netDelayBack)
	timeSyncSent = timeDelayReceived.Add(10 * time.Millisecond)
	timeSyncReceived = timeSyncSent.Add(netDelay)

	m.addT3(seq, timeDelaySent)
	m.addT2andCF1(seq, timeSyncReceived, netCorrection)
	m.addT4(seq, timeDelayReceived)
	m.addT1(seq, timeSyncSent)
	m.addCF2(seq, netCorrectionBack)

	got, err = m.latest()
	require.Nil(t, err)
	want = &MeasurementResult{
		Delay:             224995 * time.Microsecond,
		S2CDelay:          netDelay - netCorrection,
		C2SDelay:          netDelayBack - netCorrectionBack,
		Offset:            -25001 * time.Microsecond,
		CorrectionFieldRX: 6 * time.Microsecond,
		CorrectionFieldTX: 4 * time.Microsecond,
		Timestamp:         timeSyncReceived,
		T1:                timeSyncSent,
		T2:                timeSyncReceived,
		T3:                timeDelaySent,
		T4:                timeDelayReceived,
	}
	require.Equal(t, want, got, "measurements with mean path delay filter and skipped path delay sample")
}

func TestMeasurementsCleanup(t *testing.T) {
	mcfg := &MeasurementConfig{}
	m := newMeasurements(mcfg)
	m.data[123] = &mData{
		t2: time.Now(),
	}
	m.data[0] = &mData{
		t1: time.Now(),
	}
	require.Equal(t, 2, len(m.data))
	m.cleanup()
	require.Equal(t, 0, len(m.data))
}

func TestMDataComplete(t *testing.T) {
	d := mData{}
	require.False(t, d.Complete())
	d.t1 = time.Now()
	require.False(t, d.Complete())
	d.t2 = time.Now()
	require.False(t, d.Complete())
	d.t3 = time.Now()
	require.False(t, d.Complete())
	d.t4 = time.Now()
	require.True(t, d.Complete())
}

func TestBadDelay(t *testing.T) {
	mcfg := &MeasurementConfig{
		PathDelayFilterLength:         2,
		PathDelayFilter:               FilterMedian,
		PathDelayDiscardFilterEnabled: true,
		PathDelayDiscardBelow:         0,
		PathDelayDiscardMultiplier:    3,
	}
	m := newMeasurements(mcfg)

	m.delay(time.Millisecond)
	require.Equal(t, time.Millisecond, m.pathDelay)

	m.delay(3 * time.Millisecond)
	require.Equal(t, 2*time.Millisecond, m.pathDelay)

	// We add a negative delay and it's being ignored
	m.delay(-time.Second)
	require.Equal(t, 2*time.Millisecond, m.pathDelay)

	// We add a huge delay and it's being ignored
	m.delay(10 * time.Millisecond)
	require.Equal(t, 2*time.Millisecond, m.pathDelay)

	// We force filter to work only from high values
	m.cfg.PathDelayDiscardFrom = 11 * time.Millisecond

	// Triggering on > PathDelayDiscardFrom
	m.delay(13 * time.Millisecond)
	require.Equal(t, 2*time.Millisecond, m.pathDelay)

	// Ignoring < PathDelayDiscardFrom
	m.delay(11 * time.Millisecond)
	require.Equal(t, 7*time.Millisecond, m.pathDelay)
}

func TestBadCF(t *testing.T) {
	mcfg := &MeasurementConfig{
		PathDelayFilterLength:         2,
		PathDelayFilter:               FilterMedian,
		PathDelayDiscardFilterEnabled: true,
		PathDelayDiscardBelow:         0,
		PathDelayDiscardMultiplier:    3,
	}
	m := newMeasurements(mcfg)

	m.delay(time.Millisecond)
	require.Equal(t, time.Millisecond, m.pathDelay)

	m.delay(3 * time.Millisecond)
	require.Equal(t, 2*time.Millisecond, m.pathDelay)

	m.lastData = &mData{}

	// Valid delay, bad CF1
	m.lastData.c1 = -time.Millisecond
	m.delay(time.Millisecond)
	require.Equal(t, 2*time.Millisecond, m.pathDelay)

	// Valid delay, bad CF2
	m.lastData.c2 = -time.Millisecond
	m.delay(time.Millisecond)
	require.Equal(t, 2*time.Millisecond, m.pathDelay)
}
