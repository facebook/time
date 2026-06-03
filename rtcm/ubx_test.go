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
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// buildRawxPayload assembles a synthetic UBX-RXM-RAWX message payload from a
// set of observations, mirroring the u-blox wire layout. The header uses a
// fixed GPS week (2360) and leap-second count (18).
func buildRawxPayload(tow float64, obs []RawxObservation) []byte {
	p := make([]byte, 16+len(obs)*32)
	binary.LittleEndian.PutUint64(p[0:8], math.Float64bits(tow))
	binary.LittleEndian.PutUint16(p[8:10], 2360)
	p[10] = 18
	p[11] = byte(len(obs))
	for i, o := range obs {
		off := 16 + i*32
		binary.LittleEndian.PutUint64(p[off:off+8], math.Float64bits(o.PrMes))
		binary.LittleEndian.PutUint64(p[off+8:off+16], math.Float64bits(o.CpMes))
		binary.LittleEndian.PutUint32(p[off+16:off+20], math.Float32bits(o.DoMes))
		p[off+20] = o.GnssID
		p[off+21] = o.SvID
		p[off+22] = o.SigID
		p[off+23] = o.FreqID
		binary.LittleEndian.PutUint16(p[off+24:off+26], o.Locktime)
		p[off+26] = o.CNO
		var trkStat byte
		if o.PrValid {
			trkStat |= 0x01
		}
		if o.CpValid {
			trkStat |= 0x02
		}
		if o.HalfCyc {
			trkStat |= 0x04
		}
		p[off+30] = trkStat
	}
	return p
}

// buildUBXFrame wraps a payload in a UBX frame (sync, class, id, length,
// payload, Fletcher-8 checksum).
func buildUBXFrame(msg byte, payload []byte) []byte {
	frame := make([]byte, UBXHeaderSize+len(payload)+UBXChecksumLen)
	frame[0] = UBXSync1
	frame[1] = UBXSync2
	frame[2] = UBXClassRXM
	frame[3] = msg
	binary.LittleEndian.PutUint16(frame[4:6], uint16(len(payload)))
	copy(frame[UBXHeaderSize:], payload)
	var ckA, ckB uint8
	for i := 2; i < UBXHeaderSize+len(payload); i++ {
		ckA += frame[i]
		ckB += ckA
	}
	frame[len(frame)-2] = ckA
	frame[len(frame)-1] = ckB
	return frame
}

func TestParseRawxFields(t *testing.T) {
	in := []RawxObservation{
		{
			PrMes: 20000000.5, CpMes: 105000000.25, DoMes: -1234.5,
			GnssID: GnssGPS, SvID: 7, SigID: 0, FreqID: 0,
			Locktime: 12000, CNO: 45, PrValid: true, CpValid: true, HalfCyc: true,
		},
	}
	epoch, err := ParseRawx(buildRawxPayload(296966.995, in))
	require.NoError(t, err)
	require.InDelta(t, 296966.995, epoch.RcvTow, 1e-6)
	require.Equal(t, uint16(2360), epoch.Week)
	require.Equal(t, int8(18), epoch.LeapS)
	require.Len(t, epoch.Observations, 1)

	got := epoch.Observations[0]
	require.InDelta(t, 20000000.5, got.PrMes, 1e-6)
	require.InDelta(t, 105000000.25, got.CpMes, 1e-6)
	require.InDelta(t, -1234.5, float64(got.DoMes), 1e-3)
	require.Equal(t, GnssGPS, got.GnssID)
	require.Equal(t, uint8(7), got.SvID)
	require.Equal(t, uint16(12000), got.Locktime)
	require.Equal(t, uint8(45), got.CNO)
	require.True(t, got.PrValid)
	require.True(t, got.CpValid)
	require.True(t, got.HalfCyc)
}

func TestParseRawxFiltersGhosts(t *testing.T) {
	in := []RawxObservation{
		// Good observation — kept.
		{PrMes: 21000000, CpMes: 1, DoMes: 1, GnssID: GnssGPS, SvID: 1, CNO: 40, PrValid: true},
		// Pseudorange not valid — dropped.
		{PrMes: 21000000, GnssID: GnssGPS, SvID: 2, CNO: 40, PrValid: false},
		// Zero pseudorange — dropped (timing-only "ghost").
		{PrMes: 0, GnssID: GnssGPS, SvID: 3, CNO: 40, PrValid: true},
		// Zero CNR — dropped.
		{PrMes: 21000000, GnssID: GnssGPS, SvID: 4, CNO: 0, PrValid: true},
		// Another good one — kept.
		{PrMes: 22000000, GnssID: GnssGalileo, SvID: 5, CNO: 38, PrValid: true},
	}
	epoch, err := ParseRawx(buildRawxPayload(100.0, in))
	require.NoError(t, err)
	require.Len(t, epoch.Observations, 2, "only valid, nonzero observations survive")
	require.Equal(t, uint8(1), epoch.Observations[0].SvID)
	require.Equal(t, uint8(5), epoch.Observations[1].SvID)
}

func TestParseRawxTooShort(t *testing.T) {
	_, err := ParseRawx(make([]byte, 8))
	require.Error(t, err)
}

func TestParseRawxTruncated(t *testing.T) {
	// Header claims 3 measurements but the buffer only holds one.
	p := buildRawxPayload(100, []RawxObservation{
		{PrMes: 1, GnssID: GnssGPS, SvID: 1, CNO: 40, PrValid: true},
	})
	p[11] = 3 // lie about the measurement count
	_, err := ParseRawx(p)
	require.Error(t, err)
}

func TestParseUBXFrameRoundTrip(t *testing.T) {
	payload := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	frame := buildUBXFrame(UBXMsgRAWX, payload)

	got, frameLen, err := ParseUBXFrame(frame)
	require.NoError(t, err)
	require.Equal(t, len(frame), frameLen)
	require.Equal(t, payload, got)
}

func TestParseUBXFrameBadChecksum(t *testing.T) {
	frame := buildUBXFrame(UBXMsgRAWX, []byte{0xDE, 0xAD})
	frame[len(frame)-1] ^= 0xFF // corrupt checksum
	_, _, err := ParseUBXFrame(frame)
	require.Error(t, err)
}
