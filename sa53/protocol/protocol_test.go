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

package protocol

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseFirmwareVer(t *testing.T) {
	f := &Mac{}

	err := f.parseMacFirmware("[=V1.0.4.0.5ADA4E31,V1.0]\r\n")

	require.NoError(t, err)
	require.Equal(t, 1, f.fwMajor)
	require.Equal(t, 0, f.fwMinor)
	require.Equal(t, 4, f.fwPatch)
	require.Equal(t, "5ADA4E31", f.fwCommit)

	require.Equal(t, "V1.0.4", f.FormatFWVersion())
	require.Equal(t, 0x10004, f.Version())
}

func TestParseValue(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "integer", in: "[=276428]", want: "276428"},
		{name: "negative", in: "[=-1603]", want: "-1603"},
		{name: "float", in: "[=37.250]", want: "37.250"},
		{name: "string", in: "[=V1.5.0]", want: "V1.5.0"},
		{name: "empty value", in: "[=]", want: ""},
		{name: "device error 1", in: "[!1]", wantErr: true},
		{name: "device error 100", in: "[!100]", wantErr: true},
		{name: "missing brackets", in: "276428", wantErr: true},
		{name: "missing equals", in: "[276428]", wantErr: true},
		{name: "too short", in: "[=", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseValue(tc.in)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParseValueDeviceErrorWraps(t *testing.T) {
	_, err := ParseValue("[!100]")
	require.ErrorIs(t, err, ErrDeviceError)
	require.Contains(t, err.Error(), "100")
}

// fakePort is a scripted SerialReadWriter for tests. Each Read call returns
// the next chunk in chunks; SetReadTimeout is recorded.
type fakePort struct {
	chunks  [][]byte
	idx     int
	written []byte
	timeout time.Duration
	// timeoutSequence captures every SetReadTimeout call so tests can
	// assert ordering (e.g. drain installs short then restores).
	timeoutSequence []time.Duration
}

func (f *fakePort) Read(b []byte) (int, error) {
	if f.idx >= len(f.chunks) {
		return 0, nil
	}
	c := f.chunks[f.idx]
	n := copy(b, c)
	if n < len(c) {
		f.chunks[f.idx] = c[n:]
	} else {
		f.idx++
	}
	return n, nil
}

func (f *fakePort) Write(b []byte) (int, error) {
	f.written = append(f.written, b...)
	return len(b), nil
}

func (f *fakePort) SetReadTimeout(d time.Duration) error {
	f.timeout = d
	f.timeoutSequence = append(f.timeoutSequence, d)
	return nil
}

// newTestMac wraps a SerialReadWriter for tests without opening a real port.
func newTestMac(p SerialReadWriter) *Mac {
	return &Mac{port: p}
}

func TestReadResponse(t *testing.T) {
	tests := []struct {
		name    string
		chunks  [][]byte
		want    string
		wantErr bool
	}{
		{
			name:   "single chunk",
			chunks: [][]byte{[]byte("[=276428]\r\n")},
			want:   "[=276428]",
		},
		{
			name:   "split across reads",
			chunks: [][]byte{[]byte("[=27"), []byte("6428]\r\n")},
			want:   "[=276428]",
		},
		{
			name:   "terminator split",
			chunks: [][]byte{[]byte("[=42]\r"), []byte("\n")},
			want:   "[=42]",
		},
		{
			name:    "timeout before terminator",
			chunks:  [][]byte{[]byte("[=incomplete")},
			wantErr: true,
		},
		{
			name:    "empty",
			chunks:  nil,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestMac(&fakePort{chunks: tc.chunks})
			got, err := m.ReadResponse()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestGetSendsExpectedCommand(t *testing.T) {
	fp := &fakePort{chunks: [][]byte{[]byte("[=276428]\r\n")}}
	m := newTestMac(fp)
	got, err := m.Get("DigitalTuning")
	require.NoError(t, err)
	require.Equal(t, "276428", got)
	require.Equal(t, "{get,DigitalTuning}", string(fp.written))
}

func TestGetPropagatesDeviceError(t *testing.T) {
	fp := &fakePort{chunks: [][]byte{[]byte("[!1]\r\n")}}
	m := newTestMac(fp)
	_, err := m.Get("DigitalTuning")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrDeviceError)
}

// errPort always errors on Read.
type errPort struct{ fakePort }

func (e *errPort) Read(_ []byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// triggeredPort is a fakePort that appends extra chunks to its read buffer
// the first time Write is called. Lets tests model a request/response port
// where the response only arrives after the command goes out.
type triggeredPort struct {
	fakePort
	deferredChunks [][]byte
	triggered      bool
}

func (t *triggeredPort) Write(b []byte) (int, error) {
	if !t.triggered {
		t.chunks = append(t.chunks, t.deferredChunks...)
		t.triggered = true
	}
	return t.fakePort.Write(b)
}

func TestGetPropagatesReadError(t *testing.T) {
	m := newTestMac(&errPort{})
	_, err := m.Get("Temperature")
	require.True(t, errors.Is(err, io.ErrUnexpectedEOF))
}

func TestCmdDrainsThenWrites(t *testing.T) {
	// Simulate: stale bytes are pending pre-write (drain should eat them),
	// then the real response arrives only after we send the command.
	fp := &triggeredPort{
		fakePort:       fakePort{chunks: [][]byte{[]byte("stale-garbage")}},
		deferredChunks: [][]byte{[]byte("[=ok]\r\n")},
	}
	m := newTestMac(fp)
	resp, err := m.Cmd("\\{swrev?}")
	require.NoError(t, err)
	require.Equal(t, "[=ok]", resp)
	require.Equal(t, "\\{swrev?}", string(fp.written))
	// Drain installed the short timeout then restored the normal one.
	require.GreaterOrEqual(t, len(fp.timeoutSequence), 2)
	require.Equal(t, drainReadTimeout, fp.timeoutSequence[0])
	require.Equal(t, normalReadTimeout, fp.timeoutSequence[1])
}

func TestDrainRestoresReadTimeout(t *testing.T) {
	fp := &fakePort{}
	m := newTestMac(fp)
	m.Drain()
	require.Equal(t, normalReadTimeout, fp.timeout, "drain must restore the normal read timeout on its way out")
}

func TestSetReadTimeoutPassesThrough(t *testing.T) {
	fp := &fakePort{}
	m := newTestMac(fp)
	require.NoError(t, m.SetReadTimeout(5*time.Second))
	require.Equal(t, 5*time.Second, fp.timeout)
}

// countingWrap counts how many times the wrap function was invoked.
type wrappedPort struct {
	SerialReadWriter
}

func TestWrapPortInstallsInterceptor(t *testing.T) {
	fp := &fakePort{}
	m := newTestMac(fp)
	called := 0
	m.WrapPort(func(p SerialReadWriter) SerialReadWriter {
		called++
		return &wrappedPort{SerialReadWriter: p}
	})
	require.Equal(t, 1, called)
	// Confirm subsequent operations go through the wrapper by checking
	// type identity of the new port.
	_, ok := m.port.(*wrappedPort)
	require.True(t, ok, "WrapPort must replace the underlying port")
}
