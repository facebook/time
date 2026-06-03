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

// MsgType indicates the type of message returned by MixedScanner.
type MsgType int

const (
	MsgRTCM3 MsgType = iota
	MsgUBX
)

// MixedScanner reads both RTCM3 frames and UBX frames from a byte stream.
// It detects the frame type by preamble (0xD3 for RTCM3, 0xB5 for UBX).
type MixedScanner struct {
	reader  *bufio.Reader
	err     error
	ubx     []byte // raw UBX payload when msgType == MsgUBX
	frame   Frame  // valid when msgType == MsgRTCM3
	msgType MsgType
	ubxCls  byte
	ubxMsg  byte
}

// NewMixedScanner creates a scanner that reads both RTCM3 and UBX frames.
func NewMixedScanner(r io.Reader) *MixedScanner {
	return &MixedScanner{
		reader: bufio.NewReaderSize(r, MaxFrameSize*2),
	}
}

// Scan reads the next frame from the stream (either RTCM3 or UBX).
func (s *MixedScanner) Scan() bool {
	for {
		b, err := s.reader.ReadByte()
		if err != nil {
			if err != io.EOF {
				s.err = fmt.Errorf("reading preamble: %w", err)
			}
			return false
		}

		switch b {
		case Preamble: // 0xD3 — RTCM3
			if s.scanRTCM() {
				return true
			}
		case UBXSync1: // 0xB5 — potential UBX
			if s.scanUBX() {
				return true
			}
		}
	}
}

// scanRTCM attempts to read an RTCM3 frame after the preamble was consumed.
func (s *MixedScanner) scanRTCM() bool {
	headerRest, err := s.reader.Peek(2)
	if err != nil {
		if err != io.EOF && !errors.Is(err, io.ErrUnexpectedEOF) {
			s.err = fmt.Errorf("reading RTCM header: %w", err)
		}
		return false
	}

	if headerRest[0]&0xFC != 0 {
		return false
	}

	payloadLen := int(binary.BigEndian.Uint16(headerRest) & 0x03FF)
	remaining := 2 + payloadLen + CRCSize

	body, err := s.reader.Peek(remaining)
	if err != nil {
		if errors.Is(err, bufio.ErrBufferFull) {
			return false
		}
		if err != io.EOF && !errors.Is(err, io.ErrUnexpectedEOF) {
			s.err = fmt.Errorf("reading RTCM body: %w", err)
		}
		return false
	}

	frameData := make([]byte, 1+remaining)
	frameData[0] = Preamble
	copy(frameData[1:], body)

	frame, parseErr := ParseFrame(frameData)
	if parseErr != nil {
		return false
	}

	if _, err := s.reader.Discard(remaining); err != nil {
		s.err = fmt.Errorf("discarding RTCM frame: %w", err)
		return false
	}

	s.msgType = MsgRTCM3
	s.frame = frame
	return true
}

// scanUBX attempts to read a UBX frame after 0xB5 was consumed.
func (s *MixedScanner) scanUBX() bool {
	// Need sync2 + class + id + len(2) = 5 more bytes to determine frame size.
	header, err := s.reader.Peek(5)
	if err != nil {
		return false
	}

	if header[0] != UBXSync2 {
		return false // not a valid UBX frame
	}

	cls := header[1]
	msg := header[2]
	payloadLen := int(binary.LittleEndian.Uint16(header[3:5]))
	remaining := 5 + payloadLen + UBXChecksumLen // sync2 + class + id + len + payload + checksum

	frameBody, err := s.reader.Peek(remaining)
	if err != nil {
		return false
	}

	// Verify checksum over class + id + len + payload.
	var ckA, ckB uint8
	for i := 1; i < 5+payloadLen; i++ { // start at class (index 1 in peeked data, which is header[1])
		ckA += frameBody[i]
		ckB += ckA
	}
	if ckA != frameBody[remaining-2] || ckB != frameBody[remaining-1] {
		return false
	}

	// Extract payload.
	payload := make([]byte, payloadLen)
	copy(payload, frameBody[5:5+payloadLen])

	if _, err := s.reader.Discard(remaining); err != nil {
		s.err = fmt.Errorf("discarding UBX frame: %w", err)
		return false
	}

	s.msgType = MsgUBX
	s.ubx = payload
	s.ubxCls = cls
	s.ubxMsg = msg
	return true
}

// Type returns the type of the last scanned message.
func (s *MixedScanner) Type() MsgType {
	return s.msgType
}

// Frame returns the RTCM3 frame (valid only when Type() == MsgRTCM3).
func (s *MixedScanner) Frame() Frame {
	return s.frame
}

// UBXPayload returns the UBX payload (valid only when Type() == MsgUBX).
func (s *MixedScanner) UBXPayload() []byte {
	return s.ubx
}

// UBXClass returns the UBX message class.
func (s *MixedScanner) UBXClass() byte {
	return s.ubxCls
}

// UBXMsgID returns the UBX message ID.
func (s *MixedScanner) UBXMsgID() byte {
	return s.ubxMsg
}

// Err returns the first non-EOF error.
func (s *MixedScanner) Err() error {
	return s.err
}
