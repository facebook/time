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

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/facebook/time/rtcm"
	"github.com/stretchr/testify/require"
)

func TestParseStationID(t *testing.T) {
	tests := map[string]uint16{
		"MOUNT01":             1,  // trailing "01" -> 1
		"STATION42":           42, // trailing digits
		"REF000":              0,  // trailing "000" -> 0
		"NoDigitsHere":        1,  // no trailing digits -> default
		"":                    1,  // empty -> default
		"4095":                1,  // all digits, no non-digit prefix -> default
		"CleanlyEvidentIbex1": 1,  // trailing "1"
	}
	for in, want := range tests {
		require.Equalf(t, want, parseStationID(in), "parseStationID(%q)", in)
	}
}

func TestIsWriteError(t *testing.T) {
	require.True(t, isWriteError(casterWrite("MSM7", errors.New("broken pipe"))))
	// Still detected when wrapped further upstream.
	require.True(t, isWriteError(fmt.Errorf("runOnce: %w", casterWrite("1033", os.ErrClosed))))
	require.False(t, isWriteError(errors.New("socket closed (EOF)")))
	require.False(t, isWriteError(fmt.Errorf("reading from socket: %w", os.ErrDeadlineExceeded)))
}

func TestSetMSMMultipleBit(t *testing.T) {
	obs := []rtcm.RawxObservation{
		{PrMes: 20000000, CpMes: 105000000, DoMes: -100, GnssID: rtcm.GnssGPS, SvID: 5, SigID: 0, CNO: 45, PrValid: true, CpValid: true},
	}
	frame, err := rtcm.EncodeMSM7(1, rtcm.GnssGPS, 100000, obs)
	require.NoError(t, err)

	// DF393 (multiple message bit) is payload bit 54; the encoder leaves it 0.
	require.Equal(t, uint32(0), multipleBit(frame))

	setMSMMultipleBit(frame)
	require.Equal(t, uint32(1), multipleBit(frame), "multiple-message bit set")

	// CRC must be recomputed and valid.
	pl := int(frame[1]&0x03)<<8 | int(frame[2])
	stored := uint32(frame[len(frame)-3])<<16 | uint32(frame[len(frame)-2])<<8 | uint32(frame[len(frame)-1])
	require.Equal(t, rtcm.CRC24Q(frame[:rtcm.HeaderSize+pl]), stored)
}

// multipleBit reads DF393 (payload bit 54: after msgnum(12)+staid(12)+epoch(30)).
func multipleBit(frame []byte) uint32 {
	r := rtcm.NewBitReader(frame[rtcm.HeaderSize:])
	r.Skip(54)
	return r.ReadBits(1)
}

func TestSetMSMMultipleBitShortFrameNoPanic(t *testing.T) {
	require.NotPanics(t, func() { setMSMMultipleBit([]byte{0xD3, 0x00, 0x05}) })
}

func TestSetupLogger(t *testing.T) {
	for _, lvl := range []string{"debug", "info", "warn", "error", "unknown"} {
		require.NotNil(t, setupLogger(lvl), "level %q", lvl)
	}
}

func TestParseStreamCleanFile(t *testing.T) {
	// A file of back-to-back valid RTCM3 frames parses without error.
	a := rtcm.Encode1033(rtcm.AntennaDescriptor{StationID: 1, AntennaType: "A"})
	b := rtcm.Encode1033(rtcm.AntennaDescriptor{StationID: 2, AntennaType: "BB"})
	path := filepath.Join(t.TempDir(), "stream.rtcm3")
	require.NoError(t, os.WriteFile(path, append(append([]byte{}, a...), b...), 0o644))
	require.NoError(t, parseStream(path))
}

func TestParseStreamMissingFile(t *testing.T) {
	require.Error(t, parseStream(filepath.Join(t.TempDir(), "does-not-exist")))
}
