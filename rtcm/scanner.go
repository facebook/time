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
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Scanner reads RTCM3 frames from a byte stream. It handles byte-level
// synchronization by scanning for the 0xD3 preamble, validating reserved
// bits and CRC-24Q checksums, and automatically resyncing after corrupted
// or non-RTCM data.
//
// Usage:
//
//	scanner := rtcm.NewScanner(reader)
//	for scanner.Scan() {
//	    frame := scanner.Frame()
//	    // process frame
//	}
//	if err := scanner.Err(); err != nil {
//	    // handle error
//	}
type Scanner struct {
	reader *bufio.Reader
	frame  Frame
	err    error
}

// NewScanner creates a new Scanner that reads RTCM3 frames from r.
func NewScanner(r io.Reader) *Scanner {
	// Buffer must hold at least MaxFrameSize for Peek to work.
	return &Scanner{
		reader: bufio.NewReaderSize(r, MaxFrameSize*2),
	}
}

// Scan reads the next RTCM3 frame from the stream. It returns true if a
// frame was successfully read, or false on error or EOF. After Scan returns
// false, the Err method returns any error that occurred (nil on clean EOF).
func (s *Scanner) Scan() bool {
	for {
		// Scan for preamble byte. ReadByte consumes it.
		b, err := s.reader.ReadByte()
		if err != nil {
			if err != io.EOF {
				s.err = fmt.Errorf("reading preamble: %w", err)
			}
			return false
		}
		if b != Preamble {
			continue
		}

		// Peek at the rest of the header (2 bytes) without consuming.
		headerRest, err := s.reader.Peek(2)
		if err != nil {
			if err != io.EOF && !errors.Is(err, io.ErrUnexpectedEOF) {
				s.err = fmt.Errorf("reading header: %w", err)
			}
			return false
		}

		// Reserved bits (upper 6 bits of byte after preamble) must be zero.
		if headerRest[0]&0xFC != 0 {
			continue
		}

		payloadLen := int(binary.BigEndian.Uint16(headerRest) & 0x03FF)
		// Total remaining bytes after preamble: 2 (header rest) + payload + 3 (CRC).
		remaining := 2 + payloadLen + CRCSize

		// Peek at the entire frame body (header rest + payload + CRC).
		body, err := s.reader.Peek(remaining)
		if err != nil {
			if errors.Is(err, bufio.ErrBufferFull) {
				// Frame larger than buffer — skip this preamble.
				continue
			}
			if err != io.EOF && !errors.Is(err, io.ErrUnexpectedEOF) {
				s.err = fmt.Errorf("reading frame body: %w", err)
			}
			return false
		}

		// Assemble the full frame for validation.
		frameData := make([]byte, 1+remaining)
		frameData[0] = Preamble
		copy(frameData[1:], body)

		frame, parseErr := ParseFrame(frameData)
		if parseErr != nil {
			// CRC mismatch or parse error. The preamble was already consumed
			// by ReadByte, and the peeked bytes remain in the buffer.
			// Continue scanning for the next preamble.
			continue
		}

		// Frame is valid — consume the peeked bytes.
		if _, err := s.reader.Discard(remaining); err != nil {
			s.err = fmt.Errorf("discarding consumed frame: %w", err)
			return false
		}
		s.frame = frame
		return true
	}
}

// Frame returns the most recently scanned frame. It is only valid after
// Scan returns true.
func (s *Scanner) Frame() Frame {
	return s.frame
}

// Err returns the first non-EOF error encountered by the Scanner.
func (s *Scanner) Err() error {
	return s.err
}
