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
	"errors"
	"fmt"
	"math"
	"slices"
)

const speedOfLight = 299792458.0 // m/s

// GNSS signal wavelengths in meters.
var signalWavelength = map[uint8]map[uint8]float64{
	GnssGPS:     {0: speedOfLight / 1575.42e6, 3: speedOfLight / 1227.60e6, 4: speedOfLight / 1227.60e6, 6: speedOfLight / 1176.45e6, 7: speedOfLight / 1176.45e6},
	GnssGalileo: {0: speedOfLight / 1575.42e6, 1: speedOfLight / 1575.42e6, 5: speedOfLight / 1176.45e6, 6: speedOfLight / 1176.45e6},
	GnssGLONASS: {0: speedOfLight / 1602.0e6, 2: speedOfLight / 1246.0e6},
	GnssBeiDou:  {0: speedOfLight / 1561.098e6, 2: speedOfLight / 1207.14e6},
}

// MSM7 message types per constellation.
var msm7MsgType = map[uint8]uint16{
	GnssGPS:     1077,
	GnssGLONASS: 1087,
	GnssGalileo: 1097,
	GnssBeiDou:  1127,
}

// Signal ID to MSM signal mask bit index mapping (DF395).
// Bit position is 0-based from MSB (bit 0 = signal 1, bit 1 = signal 2, etc.)
// Only include primary signals to ensure single-signal frames.
var signalMaskBit = map[uint8]map[uint8]int{
	GnssGPS:     {0: 1}, // L1C/A only
	GnssGalileo: {0: 1}, // E1C only
	GnssGLONASS: {0: 1}, // L1OF only
	GnssBeiDou:  {0: 1}, // B1I only
}

// ErrNoObservations indicates a constellation had no usable observations to
// encode for the requested epoch. It is an expected outcome (e.g. a
// constellation not currently in view), not a programming error.
var ErrNoObservations = errors.New("no usable observations for constellation")

// EncodeMSM7 generates an RTCM MSM7 frame for one constellation from RAWX
// observations. Only observations matching gnssID are included. stationID and
// epochMs are encoded into the MSM header. It returns ErrNoObservations if the
// constellation has no usable observations this epoch, or an error if gnssID is
// not a supported MSM constellation.
func EncodeMSM7(stationID uint16, gnssID uint8, epochMs uint32, obs []RawxObservation) ([]byte, error) {
	msgType, ok := msm7MsgType[gnssID]
	if !ok {
		return nil, fmt.Errorf("unsupported constellation: %d", gnssID)
	}

	// Filter observations for this constellation with valid signal mapping.
	var filtered []RawxObservation
	for i := range obs {
		if obs[i].GnssID != gnssID {
			continue
		}
		// Skip ghost observations. u-blox receivers report satellites they
		// track for timing without RF acquisition as zero-valued measurements
		// (PrMes=0, CpMes=0, CNO=0). Encoding them yields all-zero MSM cells
		// that prevent a caster from computing a position. A correct MSM
		// encoder never emits cells for untracked signals. This encoder is the
		// single output chokepoint, so it rejects them regardless of the caller.
		if !obs[i].PrValid || obs[i].PrMes <= 0 || obs[i].CNO == 0 {
			continue
		}
		// Only include observations with a known signal mask bit mapping.
		if bits, ok := signalMaskBit[gnssID]; ok {
			if _, ok := bits[obs[i].SigID]; !ok {
				continue // unmapped signal ID, skip
			}
		}
		filtered = append(filtered, obs[i])
	}
	if len(filtered) == 0 {
		return nil, ErrNoObservations
	}

	// Determine unique satellites and signals.
	satSet := map[uint8]bool{}
	sigSet := map[uint8]bool{}
	for _, o := range filtered {
		satSet[o.SvID] = true
		sigSet[o.SigID] = true
	}

	sats := sortedKeys(satSet)
	sigs := sortedKeys(sigSet)

	// Build satellite mask (64 bits).
	var satMaskHi, satMaskLo uint32
	for _, sv := range sats {
		bit := int(sv) - 1 // SVs are 1-based
		if bit < 32 {
			satMaskHi |= 1 << (31 - bit)
		} else if bit < 64 {
			satMaskLo |= 1 << (63 - bit)
		}
	}

	// Build signal mask (32 bits).
	var sigMask uint32
	for _, sig := range sigs {
		if bits, ok := signalMaskBit[gnssID]; ok {
			if bit, ok := bits[sig]; ok {
				sigMask |= 1 << (31 - bit)
			}
		}
	}

	// Build cell mask and cell list.
	numSat := len(sats)
	type cell struct {
		satIdx int
		sigIdx int
		obs    RawxObservation
	}
	var cells []cell
	var cellMaskBits []bool

	// Map svID → satIdx, sigID → sigIdx
	satIdx := map[uint8]int{}
	for i, sv := range sats {
		satIdx[sv] = i
	}
	sigIdx := map[uint8]int{}
	for i, sig := range sigs {
		sigIdx[sig] = i
	}

	// Build cell mask row by row (sat-major)
	obsMap := map[[2]uint8]RawxObservation{}
	for _, o := range filtered {
		obsMap[[2]uint8{o.SvID, o.SigID}] = o
	}

	for _, sv := range sats {
		for _, sig := range sigs {
			if o, exists := obsMap[[2]uint8{sv, sig}]; exists {
				cellMaskBits = append(cellMaskBits, true)
				cells = append(cells, cell{satIdx: satIdx[sv], sigIdx: sigIdx[sig], obs: o})
			} else {
				cellMaskBits = append(cellMaskBits, false)
			}
		}
	}

	// Compute per-satellite rough range (ms) and rough phase range rate (m/s).
	type satInfo struct {
		roughRangeMs float64
		roughRateMps float64
	}
	satData := make([]satInfo, numSat)
	for _, c := range cells {
		if satData[c.satIdx].roughRangeMs == 0 {
			rangeMs := c.obs.PrMes / speedOfLight * 1000.0 // convert m to light-ms
			satData[c.satIdx].roughRangeMs = rangeMs
			// Rough phase range rate (DF399) is the satellite range rate in
			// integer m/s: -Doppler[Hz] * wavelength[m]. The raw Doppler in Hz
			// is NOT the range rate; encoding it directly produces a nonsensical
			// phase range rate that makes casters reject the measurements.
			wavelength := getWavelength(gnssID, c.obs.SigID, c.obs.FreqID)
			satData[c.satIdx].roughRateMps = math.Round(-float64(c.obs.DoMes) * wavelength)
		}
	}

	// --- Encode MSM7 ---
	w := NewBitWriter(512)

	// Header
	w.WriteBits(uint32(msgType), 12)
	w.WriteBits(uint32(stationID), 12)
	w.WriteBits(epochMs, 30)
	w.WriteBits(0, 1) // multiple message: no
	w.WriteBits(0, 3) // IODS
	w.WriteBits(0, 7) // reserved
	w.WriteBits(0, 2) // clock steering
	w.WriteBits(0, 2) // ext clock
	w.WriteBits(0, 1) // smoothing
	w.WriteBits(0, 3) // smoothing interval
	w.WriteBits(satMaskHi, 32)
	w.WriteBits(satMaskLo, 32)
	w.WriteBits(sigMask, 32)

	// Cell mask
	for _, bit := range cellMaskBits {
		if bit {
			w.WriteBits(1, 1)
		} else {
			w.WriteBits(0, 1)
		}
	}

	// Satellite data: rough range integer ms (8 bits per sat)
	for i := range numSat {
		intMs := uint32(math.Floor(satData[i].roughRangeMs))
		if intMs > 254 {
			intMs = 255 // invalid marker
		}
		w.WriteBits(intMs, 8)
	}

	// Satellite data: extended info (4 bits per sat)
	for range numSat {
		w.WriteBits(0, 4)
	}

	// Satellite data: rough range modulo (10 bits per sat)
	for i := range numSat {
		fracMs := satData[i].roughRangeMs - math.Floor(satData[i].roughRangeMs)
		mod := min(uint32(math.Round(fracMs*1024.0)), 1023)
		w.WriteBits(mod, 10)
	}

	// Satellite data: rough phase range rate (14 bits signed per sat, m/s)
	for i := range numSat {
		rate := int32(satData[i].roughRateMps)
		if rate > 8191 {
			rate = 8191
		} else if rate < -8191 {
			rate = -8191
		}
		w.WriteSignedBits(rate, 14)
	}

	// Signal data: fine pseudorange (20 bits signed per cell)
	for _, c := range cells {
		rangeMs := c.obs.PrMes / speedOfLight * 1000.0
		roughMs := math.Floor(satData[c.satIdx].roughRangeMs) +
			math.Round((satData[c.satIdx].roughRangeMs-math.Floor(satData[c.satIdx].roughRangeMs))*1024.0)/1024.0
		fineMs := rangeMs - roughMs
		// Scale: 2^-29 ms → value = fineMs / 2^-29 = fineMs * 2^29
		val := int32(math.Round(fineMs * (1 << 29)))
		if val > (1<<19 - 1) {
			val = 1<<19 - 1
		} else if val < -(1 << 19) {
			val = -(1 << 19)
		}
		w.WriteSignedBits(val, 20)
	}

	// Signal data: fine phase range (24 bits signed per cell)
	for _, c := range cells {
		if !c.obs.CpValid {
			w.WriteSignedBits(-1<<23, 24) // invalid marker
			continue
		}
		wavelength := getWavelength(gnssID, c.obs.SigID, c.obs.FreqID)
		rangeMs := c.obs.PrMes / speedOfLight * 1000.0
		roughMs := math.Floor(satData[c.satIdx].roughRangeMs) +
			math.Round((satData[c.satIdx].roughRangeMs-math.Floor(satData[c.satIdx].roughRangeMs))*1024.0)/1024.0
		// Phase in ms: cpMes * wavelength / speedOfLight * 1000
		phaseMs := c.obs.CpMes * wavelength / speedOfLight * 1000.0
		finePhaseMs := phaseMs - roughMs
		// Wrap to ±0.5 ms range
		for finePhaseMs > 0.5 {
			finePhaseMs -= 1.0
		}
		for finePhaseMs < -0.5 {
			finePhaseMs += 1.0
		}
		_ = rangeMs
		// Scale: 2^-31 ms
		val := int32(math.Round(finePhaseMs * (1 << 31)))
		if val > (1<<23 - 1) {
			val = 1<<23 - 1
		} else if val < -(1<<23 - 1) {
			val = -(1<<23 - 1)
		}
		w.WriteSignedBits(val, 24)
	}

	// Signal data: lock time indicator (10 bits per cell)
	for _, c := range cells {
		w.WriteBits(uint32(lockTimeMsToExt(c.obs.Locktime)), 10)
	}

	// Signal data: half-cycle ambiguity indicator (DF420, 1 bit per cell).
	// DF420 = 1 means a half-cycle ambiguity is present, i.e. the carrier
	// phase is NOT fully resolved. The u-blox HalfCyc flag is the opposite
	// ("half cycle valid"), so it must be inverted here. Writing it directly
	// flags every observation as ambiguous, making casters discard all
	// carrier phase ("lack usable measurements").
	for _, c := range cells {
		if c.obs.HalfCyc {
			w.WriteBits(0, 1)
		} else {
			w.WriteBits(1, 1)
		}
	}

	// Signal data: CNR (10 bits per cell, 0.0625 dB-Hz units)
	for _, c := range cells {
		val := min(
			// convert 1 dB-Hz to 0.0625 dB-Hz
			uint32(c.obs.CNO)*16, 1023)
		w.WriteBits(val, 10)
	}

	// Signal data: fine phase range rate (15 bits signed per cell)
	for _, c := range cells {
		// Doppler in Hz → phase range rate = -doppler * wavelength (m/s)
		// Fine rate = total rate - rough rate, in 0.0001 m/s units
		wavelength := getWavelength(gnssID, c.obs.SigID, c.obs.FreqID)
		totalRate := -float64(c.obs.DoMes) * wavelength
		fineRate := totalRate - satData[c.satIdx].roughRateMps
		// Scale: 0.0001 m/s
		val := int32(math.Round(fineRate * 10000.0))
		if val > (1<<14 - 1) {
			val = 1<<14 - 1
		} else if val < -(1 << 14) {
			val = -(1 << 14)
		}
		w.WriteSignedBits(val, 15)
	}

	// Build complete frame.
	payload := w.Bytes()
	frameLen := HeaderSize + len(payload) + CRCSize
	frame := make([]byte, frameLen)
	frame[0] = Preamble
	frame[1] = byte((len(payload) >> 8) & 0x03)
	frame[2] = byte(len(payload) & 0xFF)
	copy(frame[HeaderSize:], payload)

	putCRC(frame)

	return frame, nil
}

// getWavelength returns the signal wavelength for a given GNSS/signal combination.
func getWavelength(gnssID, sigID, freqID uint8) float64 {
	if gnssID == GnssGLONASS {
		// GLONASS FDMA: frequency depends on slot
		k := int(freqID) - 7 // slot offset
		if sigID == 0 {
			return speedOfLight / (1602.0e6 + float64(k)*562.5e3)
		}
		return speedOfLight / (1246.0e6 + float64(k)*437.5e3)
	}
	if sigs, ok := signalWavelength[gnssID]; ok {
		if wl, ok := sigs[sigID]; ok {
			return wl
		}
	}
	return speedOfLight / 1575.42e6 // default GPS L1
}

// lockTimeMsToExt converts lock time in ms to the 10-bit extended indicator.
func lockTimeMsToExt(ms uint16) uint16 {
	switch {
	case ms < 64:
		return ms
	case ms < 128:
		return 64 + (ms-64)/2
	case ms < 256:
		return 96 + (ms-128)/4
	case ms < 512:
		return 128 + (ms-256)/8
	case ms < 1024:
		return 160 + (ms-512)/16
	case ms < 2048:
		return 192 + (ms-1024)/32
	case ms < 4096:
		return 224 + (ms-2048)/64
	case ms < 8192:
		return 256 + (ms-4096)/128
	case ms < 16384:
		return 288 + (ms-8192)/256
	case ms < 32768:
		return 320 + (ms-16384)/512
	default:
		return 352 + (ms-32768)/1024
	}
}

func sortedKeys(m map[uint8]bool) []uint8 {
	keys := make([]uint8, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
