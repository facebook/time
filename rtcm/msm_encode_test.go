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

package rtcm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// decodedMSM7 holds the fields of an MSM7 message decoded for testing.
type decodedMSM7 struct {
	msgType, stationID uint16
	epoch              uint32
	multiple           bool
	sats               []int // 1-based satellite IDs from the satellite mask
	sigs               []int // 1-based signal IDs from the signal mask
	cells              []bool
	roughInt, roughMod []uint32
	roughRate          []int32
	finePR, finePhase  []int32
	lock, half, cnr    []uint32
	fineRate           []int32
}

// decodeMSM7 fully decodes an MSM7 frame by reading every field sequentially in
// spec order (no hardcoded bit offsets), validating framing and CRC along the
// way. It is the inverse of EncodeMSM7 and is used to verify encoder output.
func decodeMSM7(t *testing.T, frame []byte) decodedMSM7 {
	t.Helper()
	require.NotNil(t, frame)
	require.Equal(t, Preamble, frame[0])

	pl := int(frame[1]&0x03)<<8 | int(frame[2])
	require.Equal(t, HeaderSize+pl+CRCSize, len(frame), "declared length matches frame size")
	stored := uint32(frame[len(frame)-3])<<16 | uint32(frame[len(frame)-2])<<8 | uint32(frame[len(frame)-1])
	require.Equal(t, CRC24Q(frame[:HeaderSize+pl]), stored, "CRC-24Q")

	r := NewBitReader(frame[HeaderSize : HeaderSize+pl])
	var d decodedMSM7
	d.msgType = uint16(r.ReadBits(12))
	d.stationID = uint16(r.ReadBits(12))
	d.epoch = r.ReadBits(30)
	d.multiple = r.ReadBits(1) == 1
	r.ReadBits(3) // IODS
	r.ReadBits(7) // reserved
	r.ReadBits(2) // clock steering
	r.ReadBits(2) // external clock
	r.ReadBits(1) // smoothing indicator
	r.ReadBits(3) // smoothing interval

	for i := range 64 {
		if r.ReadBits(1) == 1 {
			d.sats = append(d.sats, i+1)
		}
	}
	for i := range 32 {
		if r.ReadBits(1) == 1 {
			d.sigs = append(d.sigs, i+1)
		}
	}
	nsat, nsig := len(d.sats), len(d.sigs)
	ncell := 0
	for range nsat * nsig {
		on := r.ReadBits(1) == 1
		d.cells = append(d.cells, on)
		if on {
			ncell++
		}
	}

	// Satellite data.
	for range nsat {
		d.roughInt = append(d.roughInt, r.ReadBits(8))
	}
	for range nsat {
		r.ReadBits(4) // extended satellite info
	}
	for range nsat {
		d.roughMod = append(d.roughMod, r.ReadBits(10))
	}
	for range nsat {
		d.roughRate = append(d.roughRate, r.ReadSignedBits(14))
	}

	// Signal data.
	for range ncell {
		d.finePR = append(d.finePR, r.ReadSignedBits(20))
	}
	for range ncell {
		d.finePhase = append(d.finePhase, r.ReadSignedBits(24))
	}
	for range ncell {
		d.lock = append(d.lock, r.ReadBits(10))
	}
	for range ncell {
		d.half = append(d.half, r.ReadBits(1))
	}
	for range ncell {
		d.cnr = append(d.cnr, r.ReadBits(10))
	}
	for range ncell {
		d.fineRate = append(d.fineRate, r.ReadSignedBits(15))
	}

	// Everything after the last field is byte padding (< 8 bits).
	require.LessOrEqual(t, r.Pos(), pl*8)
	require.Less(t, pl*8-r.Pos(), 8, "field layout exactly fills the payload")
	return d
}

func gpsObs(sv uint8, pr, cp float64, dop float32, cno uint8) RawxObservation {
	return RawxObservation{
		PrMes: pr, CpMes: cp, DoMes: dop, GnssID: GnssGPS, SvID: sv, SigID: 0,
		Locktime: 63000, CNO: cno, PrValid: true, CpValid: true, HalfCyc: true,
	}
}

func ghostObs(sv uint8) RawxObservation {
	return RawxObservation{GnssID: GnssGPS, SvID: sv, SigID: 0, PrValid: true}
}

func TestEncodeMSM7ValidFrame(t *testing.T) {
	const stationID, epochMs = uint16(7), uint32(288093000)
	obs := []RawxObservation{
		gpsObs(4, 20960834.6, 110135576.123, -3722.4, 43),
		gpsObs(8, 18864369.0, 99121456.789, -1825.4, 49),
		gpsObs(9, 19008414.2, 99878123.456, -2474.4, 45),
		gpsObs(21, 23242403.8, 122120987.654, -1072.4, 40),
		gpsObs(27, 21312585.2, 111983456.321, -3452.4, 42),
		ghostObs(1), // timing-only track, must be excluded
	}

	frame, err := EncodeMSM7(stationID, GnssGPS, epochMs, obs)
	require.NoError(t, err)

	d := decodeMSM7(t, frame)
	require.Equal(t, TypeGPSMSM7, d.msgType)
	require.Equal(t, stationID, d.stationID)
	require.Equal(t, epochMs, d.epoch)
	require.False(t, d.multiple, "encoder leaves the multiple-message bit clear")
	require.Equal(t, []int{4, 8, 9, 21, 27}, d.sats, "ghost SV1 excluded")
	require.Equal(t, []int{2}, d.sigs, "GPS L1 C/A is RTCM signal 2")
	require.Len(t, d.cells, 5)
	require.NotContains(t, d.sats, 1)
}

func TestEncodeMSM7NoObservationsForConstellation(t *testing.T) {
	obs := []RawxObservation{gpsObs(5, 20000000, 105000000, -1000, 40)}
	frame, err := EncodeMSM7(1, GnssGalileo, 100000, obs) // ask for Galileo
	require.NoError(t, err)
	require.Nil(t, frame, "no frame when the constellation has no observations")
}

func TestEncodeMSM7ExcludesGhostCells(t *testing.T) {
	obs := []RawxObservation{
		gpsObs(4, 20960834.6, 110135576.123, -3722.4, 43),
		ghostObs(1),
		gpsObs(8, 18864369.0, 99121456.789, -1825.4, 49),
		ghostObs(14),
		gpsObs(9, 19008414.2, 99878123.456, -2474.4, 45),
		// Nonzero pseudorange but flagged invalid — also a ghost.
		{PrMes: 21000000, GnssID: GnssGPS, SvID: 30, SigID: 0, PrValid: false},
	}
	frame, err := EncodeMSM7(1, GnssGPS, 288093000, obs)
	require.NoError(t, err)

	d := decodeMSM7(t, frame)
	require.Equal(t, []int{4, 8, 9}, d.sats)
	require.Len(t, d.cells, 3)
	for _, c := range d.cells {
		require.True(t, c, "single-signal frame has a cell for every satellite")
	}
	// No emitted cell may carry an "untracked" value.
	for i, ri := range d.roughInt {
		require.NotEqual(t, uint32(0), ri, "sat %d rough range is real", i)
		require.NotEqual(t, uint32(255), ri, "sat %d rough range not the invalid marker", i)
	}
	for i, cnr := range d.cnr {
		require.NotEqual(t, uint32(0), cnr, "cell %d carries real signal strength", i)
	}
}

func TestEncodeMSM7AllGhostsYieldNilFrame(t *testing.T) {
	obs := []RawxObservation{ghostObs(1), ghostObs(14)}
	frame, err := EncodeMSM7(1, GnssGPS, 288093000, obs)
	require.NoError(t, err)
	require.Nil(t, frame)
}

func TestEncodeMSM7PhaseRangeRate(t *testing.T) {
	// DF399 must carry the satellite range rate in m/s (-Doppler*wavelength),
	// not the raw Doppler in Hz. Encode a known Doppler and reconstruct it from
	// the rough (DF399) + fine (DF404) phase range rate.
	const doppler float32 = -519.0 // Hz, satellite receding
	obs := []RawxObservation{gpsObs(5, 20000000, 105000000, doppler, 45)}

	frame, err := EncodeMSM7(1, GnssGPS, 100000, obs)
	require.NoError(t, err)
	d := decodeMSM7(t, frame)
	require.Len(t, d.roughRate, 1)
	require.Len(t, d.fineRate, 1)

	wavelength := speedOfLight / 1575.42e6
	totalRate := float64(d.roughRate[0]) + float64(d.fineRate[0])*0.0001 // m/s
	gotDoppler := -totalRate / wavelength
	require.InDelta(t, float64(doppler), gotDoppler, 1.0)
	require.Positive(t, d.roughRate[0], "receding satellite has a positive range rate")
}

func TestEncodeMSM7HalfCycleInverted(t *testing.T) {
	// DF420 (half-cycle ambiguity) is the inverse of the u-blox "half cycle
	// valid" flag: resolved -> 0, unresolved -> 1.
	resolved := gpsObs(5, 20000000, 105000000, -100, 45)
	resolved.HalfCyc = true
	d := decodeMSM7(t, mustEncode(t, resolved))
	require.Equal(t, uint32(0), d.half[0], "resolved half cycle -> DF420 0")

	unresolved := gpsObs(6, 20000000, 105000000, -100, 45)
	unresolved.HalfCyc = false
	d = decodeMSM7(t, mustEncode(t, unresolved))
	require.Equal(t, uint32(1), d.half[0], "unresolved half cycle -> DF420 1")
}

func TestEncodeMSM7GLONASS(t *testing.T) {
	// GLONASS is FDMA: the wavelength depends on the frequency channel. The
	// encoder must still produce a valid frame with real measurements.
	obs := []RawxObservation{
		{PrMes: 19500000, CpMes: 104000000, DoMes: 1500, GnssID: GnssGLONASS, SvID: 3, SigID: 0, FreqID: 5, CNO: 44, PrValid: true, CpValid: true},
		{PrMes: 21000000, CpMes: 112000000, DoMes: -2000, GnssID: GnssGLONASS, SvID: 7, SigID: 0, FreqID: 1, CNO: 41, PrValid: true, CpValid: true},
	}
	frame, err := EncodeMSM7(1, GnssGLONASS, 100000, obs)
	require.NoError(t, err)
	d := decodeMSM7(t, frame)
	require.Equal(t, TypeGLONASSMSM7, d.msgType)
	require.Equal(t, []int{3, 7}, d.sats)
	for _, cnr := range d.cnr {
		require.NotZero(t, cnr)
	}
}

func TestEncodeMSM7FiltersByConstellation(t *testing.T) {
	obs := []RawxObservation{
		gpsObs(4, 20000000, 105000000, -100, 45),
		{PrMes: 22000000, CpMes: 115000000, DoMes: -200, GnssID: GnssGalileo, SvID: 11, SigID: 0, CNO: 40, PrValid: true, CpValid: true},
		gpsObs(9, 21000000, 110000000, -300, 43),
	}
	d := decodeMSM7(t, mustEncodeAll(t, obs))
	require.Equal(t, []int{4, 9}, d.sats, "only GPS satellites encoded")
}

func TestEncodeMSM7Deterministic(t *testing.T) {
	obs := []RawxObservation{
		gpsObs(10, 22000000, 115600000, -2000, 45),
		gpsObs(15, 24000000, 126100000, -1500, 42),
	}
	a, err := EncodeMSM7(1, GnssGPS, 200000000, obs)
	require.NoError(t, err)
	b, err := EncodeMSM7(1, GnssGPS, 200000000, obs)
	require.NoError(t, err)
	require.Equal(t, a, b, "encoding is deterministic")
}

func mustEncode(t *testing.T, obs RawxObservation) []byte {
	t.Helper()
	frame, err := EncodeMSM7(1, GnssGPS, 100000, []RawxObservation{obs})
	require.NoError(t, err)
	require.NotNil(t, frame)
	return frame
}

func mustEncodeAll(t *testing.T, obs []RawxObservation) []byte {
	t.Helper()
	frame, err := EncodeMSM7(1, GnssGPS, 100000, obs)
	require.NoError(t, err)
	require.NotNil(t, frame)
	return frame
}
