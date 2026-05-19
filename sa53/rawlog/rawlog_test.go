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

package rawlog

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoggerWritesTaggedLines(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(&buf)
	l.TX([]byte("ping"))
	l.RX([]byte("pong"))
	l.Mark("done")
	require.NoError(t, l.Close())

	got := buf.String()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	require.Len(t, lines, 4, "expected 4 lines: start MARK, TX, RX, done MARK")

	require.Contains(t, lines[0], "MARK")
	require.Contains(t, lines[0], `"sa53 poll start"`)

	require.Contains(t, lines[1], "TX")
	require.Contains(t, lines[1], `"ping"`)
	require.Contains(t, lines[1], "70696e67") // "ping" in hex

	require.Contains(t, lines[2], "RX")
	require.Contains(t, lines[2], `"pong"`)

	require.Contains(t, lines[3], "MARK")
	require.Contains(t, lines[3], `"done"`)
}

// fakePort is a minimal SerialReadWriter for the LoggingPort test.
type fakePort struct {
	readChunks [][]byte
	idx        int
	written    []byte
}

func (f *fakePort) Read(b []byte) (int, error) {
	if f.idx >= len(f.readChunks) {
		return 0, nil
	}
	c := f.readChunks[f.idx]
	n := copy(b, c)
	f.idx++
	return n, nil
}

func (f *fakePort) Write(b []byte) (int, error) {
	f.written = append(f.written, b...)
	return len(b), nil
}

func (f *fakePort) SetReadTimeout(time.Duration) error { return nil }

func TestLoggingPortTeesBytes(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(&buf)
	fp := &fakePort{readChunks: [][]byte{[]byte("[=42]\r\n")}}
	lp := NewLoggingPort(fp, l)

	_, err := lp.Write([]byte("{get,X}"))
	require.NoError(t, err)

	out := make([]byte, 16)
	n, err := lp.Read(out)
	require.NoError(t, err)
	require.Equal(t, "[=42]\r\n", string(out[:n]))

	require.NoError(t, l.Close())
	got := buf.String()
	require.Contains(t, got, `TX "{get,X}"`)
	require.Contains(t, got, `RX "[=42]\r\n"`)
	require.Equal(t, "{get,X}", string(fp.written))
}

func TestLoggingPortDoesNotLogEmptyReads(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger(&buf)
	fp := &fakePort{} // returns (0, nil)
	lp := NewLoggingPort(fp, l)

	out := make([]byte, 16)
	n, err := lp.Read(out)
	require.NoError(t, err)
	require.Zero(t, n)

	require.NoError(t, l.Close())
	got := buf.String()
	require.NotContains(t, got, "RX")
}
