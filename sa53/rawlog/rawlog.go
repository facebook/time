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

// Package rawlog records the exact bytes sent to and received from the SA53
// over the serial port. Each line is a timestamped TX/RX/MARK entry, useful
// while validating new protocol commands or chasing wedged-port symptoms.
//
// Lifecycle: caller owns the underlying io.Writer (typically a file), the
// Logger owns its bufio buffer. Logger.Close flushes; the file is closed by
// the caller on its own defer.
package rawlog

import (
	"bufio"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/facebook/time/sa53/protocol"
)

// Logger writes timestamped TX/RX/MARK entries to the wrapped writer.
type Logger struct {
	mu sync.Mutex
	w  *bufio.Writer
}

// NewLogger wraps w in a buffered writer and emits a "MARK sa53 poll start"
// line so the file always begins with a recognizable header.
func NewLogger(w io.Writer) *Logger {
	l := &Logger{w: bufio.NewWriter(w)}
	l.line("MARK", []byte("sa53 poll start"))
	return l
}

func (l *Logger) line(tag string, b []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.w, "%d %s %q %x\n", time.Now().UnixNano(), tag, b, b)
}

// Mark records a free-form annotation. Useful for marking sample boundaries.
func (l *Logger) Mark(msg string) { l.line("MARK", []byte(msg)) }

// TX records bytes written to the SA53.
func (l *Logger) TX(b []byte) { l.line("TX", b) }

// RX records bytes read from the SA53.
func (l *Logger) RX(b []byte) { l.line("RX", b) }

// Close flushes the internal buffer. The underlying writer is not closed
// here; the caller owns its lifecycle.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Flush()
}

// LoggingPort wraps a SerialReadWriter so every byte is also recorded to the
// Logger. Read/Write/SetReadTimeout are pass-through; the side-effect is the
// log entries.
type LoggingPort struct {
	p   protocol.SerialReadWriter
	log *Logger
}

// NewLoggingPort returns a SerialReadWriter that tees TX and RX bytes into
// l. l must not be nil; callers that don't want logging should not wrap.
func NewLoggingPort(p protocol.SerialReadWriter, l *Logger) *LoggingPort {
	return &LoggingPort{p: p, log: l}
}

// Read passes through to the underlying port and records non-empty reads.
func (lp *LoggingPort) Read(b []byte) (int, error) {
	n, err := lp.p.Read(b)
	if n > 0 {
		lp.log.RX(b[:n])
	}
	return n, err
}

// Write passes through to the underlying port and records every write.
func (lp *LoggingPort) Write(b []byte) (int, error) {
	lp.log.TX(b)
	return lp.p.Write(b)
}

// SetReadTimeout passes through to the underlying port.
func (lp *LoggingPort) SetReadTimeout(d time.Duration) error {
	return lp.p.SetReadTimeout(d)
}
