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
package ntske

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"slices"
	"time"

	"github.com/facebook/time/ntp/protocol"
)

// NTS-KE protocol constants
const (
	// ALPNProtocol is the ALPN protocol ID negotiated over TLS.
	ALPNProtocol = "ntske/1"
	// exporterLabel is the RFC 8915 TLS exporter label used for key extraction.
	exporterLabel = "EXPORTER-network-time-security"
	// NextProtocolNTPv4 is the Next Protocol ID for NTPv4.
	NextProtocolNTPv4 uint16 = 0
	// maxMessageSize is the upper bound on a single NTS-KE message.
	maxMessageSize = 64 * 1024
)

// server default
const (
	// defaultCookies is the number of cookies issued per handshake when Server.Cookies is unset.
	defaultCookies = 8
	// maxCookies is the hard upper bound on cookies issued per handshake,
	// regardless of Server.Cookies. It bounds the per-connection cookie-sealing
	// cost and the response size so a large or misconfigured Cookies value
	// cannot be turned into a CPU/bandwidth amplification vector.
	maxCookies = 32
	// defaultHandshakeTimeout is the per-connection deadline applied to the entire
	// NTS-KE exchange when Server.HandshakeTimeout is unset. It bounds TLS handshake,
	// request read, validation, cookie sealing, and response write — not just the
	// TLS handshake.
	defaultHandshakeTimeout = 10 * time.Second
)

// exporter constants direction RFC 8915 5.1
const (
	// directionC2S labels the client-to-server key direction.
	directionC2S byte = 0
	// directionS2C labels the server-to-client key direction.
	directionS2C byte = 1
)

// NTS-KE error codes RFC 8915 7.8
const (
	// errorUnrecognizedCriticalRecord means a critical record had an unrecognized type.
	errorUnrecognizedCriticalRecord uint16 = 0
	// errorBadRequest means the request was malformed.
	errorBadRequest uint16 = 1
	// errorInternalServerError means the server failed to process a valid request.
	errorInternalServerError uint16 = 2
)

// Stats receives per-connection counters emitted by a Server.
type Stats interface {
	IncHandshakes() // a TLS handshake completed with ALPN "ntske/1"
	IncErrors()     // a connection ended in an NTS-KE Error response or failure
}

// Server holds the configuration for an NTS-KE server. The zero value is not
// usable; TLSConfig and Keystore must be set before serving.
type Server struct {
	// TLSConfig is cloned per listener; MinVersion is pinned to TLS 1.3 and
	// NextProtos to "ntske/1".
	TLSConfig *tls.Config
	// Keystore seals NTS cookies returned to clients.
	Keystore Keystore
	// Cookies is the number of NewCookie records to issue per exchange.
	// Defaults to 8 when unset and is capped at maxCookies (32); larger values
	// are clamped to bound per-connection cost.
	Cookies uint16
	// SupportedAEAD is the list of AEAD algorithm IDs the server will negotiate.
	// Defaults to AES-128-GCM-SIV (30) and AES-SIV-CMAC-512 (17) when unset.
	SupportedAEAD []uint16
	// HandshakeTimeout is the per-connection deadline for the entire NTS-KE
	// exchange, not just the TLS handshake. It bounds TLS handshake,
	// request record read, request validation, cookie sealing via Keystore,
	// and response write. Defaults to 10s when unset. Size this to accommodate
	// slow keystore or network paths; TLS-only sizing will cause premature
	// timeouts.
	HandshakeTimeout time.Duration
	// NTPv4Server, if set, is advertised in Server Negotiation records.
	NTPv4Server string
	// NTPv4Port, if non-zero, is advertised in Port Negotiation records.
	NTPv4Port uint16
	// Stats, if non-nil, receives per-connection counters.
	Stats Stats
}

// cookieError wraps an NTS-KE error code so the handler can distinguish a
// protocol rejection (respond with an Error record, code carried here) from a
// transport failure (just drop the connection).
type cookieError struct {
	code uint16
	err  error
}

func (e *cookieError) Error() string { return fmt.Sprintf("ntske: error code %d: %v", e.code, e.err) }
func (e *cookieError) Unwrap() error { return e.err }
func protocolError(code uint16, format string, args ...any) *cookieError {
	return &cookieError{code: code, err: fmt.Errorf(format, args...)}
}

// ListenAndServe listens on the TCP address addr and serves NTS-KE connections
// until ctx is cancelled. It returns the error that stopped the accept loop; a
// clean shutdown via ctx cancellation returns nil.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	if s.TLSConfig == nil {
		return errors.New("ntske: TLSConfig is required")
	}
	if s.Keystore == nil {
		return errors.New("ntske: Keystore is required")
	}
	// Clone so we never mutate the caller's config, then pin the two invariants
	// the protocol requires: TLS 1.3 and ALPN "ntske/1".
	tlsConf := s.TLSConfig.Clone()
	tlsConf.MinVersion = tls.VersionTLS13
	tlsConf.NextProtos = []string{ALPNProtocol}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("ntske: listen %q: %w", addr, err)
	}
	tlsLn := tls.NewListener(ln, tlsConf)
	// Cancelling ctx unblocks Accept by closing the listener; the resulting
	// net.ErrClosed is treated as a clean stop below.
	context.AfterFunc(ctx, func() { _ = tlsLn.Close() })
	for {
		conn, err := tlsLn.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				//nolint:nilerr // ctx cancellation/ErrClosed is a clean shutdown, not a failure
				return nil
			}
			return fmt.Errorf("ntske: accept: %w", err)
		}
		go s.handleConnection(ctx, conn)
	}
}

// handleConnection runs the full NTS-KE exchange for one client. It is the main
// per-connection routine and never returns an error to the caller: every exit
// path either sends the client an Error record or closes the connection, and
// records the outcome in Stats.
func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer func() { _ = conn.Close() }()
	// (1) Bound the whole exchange with one deadline derived from the timeout.
	deadline := time.Now().Add(s.handshakeTimeout())
	_ = conn.SetDeadline(deadline)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	// (2) The listener produced a *tls.Conn; the handshake is lazy, so drive it
	// explicitly to fail fast and to expose ConnectionState before any I/O.
	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		slog.Error("ntske: connection is not TLS", "remote", conn.RemoteAddr())
		s.incErrors()
		return
	}
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		slog.Warn("ntske: TLS handshake failed", "remote", conn.RemoteAddr(), "err", err)
		s.incErrors()
		return
	}
	// (3) Enforce ALPN: a client that did not select "ntske/1" is not speaking
	// this protocol, so there is no meaningful Error record to send — just drop.
	cs := tlsConn.ConnectionState()
	if cs.NegotiatedProtocol != ALPNProtocol {
		slog.Warn("ntske: wrong ALPN", "remote", conn.RemoteAddr(), "alpn", cs.NegotiatedProtocol)
		s.incErrors()
		return
	}
	s.incHandshakes()
	// (4) Read the request records up to End of Message, maxCapped at 64 KiB.
	records, err := readMessage(tlsConn, maxMessageSize)
	if err != nil {
		slog.Warn("ntske: reading request", "remote", conn.RemoteAddr(), "err", err)
		s.incErrors()
		return
	}
	// (5) Validate the request and choose the AEAD algorithm. A protocol-level
	// failure carries an error code we relay to the client; anything else drops.
	aeadID, compliantExport, err := s.validateRequest(records)
	if err != nil {
		var ce *cookieError
		if errors.As(err, &ce) { //nolint:modernize // Go 1.25 compatibility: avoid errors.AsType which requires Go 1.26
			s.writeError(tlsConn, ce.code)
		}
		slog.Warn("ntske: invalid request", "remote", conn.RemoteAddr(), "err", err)
		s.incErrors()
		return
	}
	// (6) Derive C2S/S2C keys from the TLS session and seal the cookies.
	response, err := s.buildResponse(cs, aeadID, compliantExport)
	if err != nil {
		s.writeError(tlsConn, errorInternalServerError)
		slog.Error("ntske: building response", "remote", conn.RemoteAddr(), "err", err)
		s.incErrors()
		return
	}
	// (7) Marshal and write the response records back to the client.
	if err := s.writeRecords(tlsConn, response); err != nil {
		slog.Warn("ntske: writing response", "remote", conn.RemoteAddr(), "err", err)
		s.incErrors()
	}
}

// validateRequest checks the client's records against RFC 8915 §4.1 and returns
// the negotiated AEAD algorithm ID. It requires a Next Protocol record naming
// NTPv4, an AEAD record whose list intersects SupportedAEAD, and a terminating
// End of Message; any unrecognized critical record is rejected with error code
// 0. The chosen algorithm is the client's first preference the server supports.
// Duplicate Next Protocol or AEAD Algorithm records are rejected with BadRequest
// to avoid silent overwrite and to signal the error back to the client.
func (s *Server) validateRequest(records []Record) (aeadID uint16, compliantExport bool, err error) {
	var (
		sawNextProto bool
		sawEOM       bool
		sawAEAD      bool
		clientAEAD   []uint16
		ntpv4        bool
	)
	for _, r := range records {
		switch r.Type {
		case RecordEndOfMessage:
			sawEOM = true
		case RecordNextProtocol:
			if sawNextProto {
				return 0, false, protocolError(errorBadRequest,
					"duplicate Next Protocol Negotiation record")
			}
			sawNextProto = true
			var ids []uint16
			ids, err = ParseUint16s(r.Body)
			if err != nil {
				return 0, false, protocolError(errorBadRequest, "malformed Next Protocol body: %v", err)
			}
			for _, id := range ids {
				if id == NextProtocolNTPv4 {
					ntpv4 = true
				}
			}
		case RecordAEADAlgorithm:
			if sawAEAD {
				return 0, false, protocolError(errorBadRequest,
					"duplicate AEAD Algorithm Negotiation record")
			}
			sawAEAD = true
			clientAEAD, err = ParseUint16s(r.Body)
			if err != nil {
				return 0, false, protocolError(errorBadRequest, "malformed AEAD body: %v", err)
			}
		case RecordCompliant128GCMExport:
			compliantExport = true
		case RecordError, RecordWarning, RecordNewCookie,
			RecordServerNegotiation, RecordPortNegotiation:
			// Records a client may legally send or we simply ignore server-side.
		default:
			// Unknown record: only fatal if the Critical Bit is set (RFC 8915 §4).
			if r.Critical {
				return 0, false, protocolError(errorUnrecognizedCriticalRecord,
					"unknown critical record type %d", r.Type)
			}
		}
	}
	switch {
	case !sawEOM:
		return 0, false, protocolError(errorBadRequest, "missing End of Message record")
	case !sawNextProto || !ntpv4:
		return 0, false, protocolError(errorBadRequest, "no NTPv4 in Next Protocol Negotiation")
	case !sawAEAD:
		return 0, false, protocolError(errorBadRequest, "missing AEAD Algorithm Negotiation")
	case len(clientAEAD) == 0:
		return 0, false, protocolError(errorBadRequest, "empty AEAD Algorithm Negotiation body")
	}
	// Pick the client's first preference that we support. Client order wins, and
	// we only offer an algorithm we can actually derive keys for (aeadIDToKeyLen
	// is the source of truth), so a misconfigured SupportedAEAD yields a clean
	// rejection here instead of an internal error later in buildResponse.
	for _, id := range clientAEAD {
		if !slices.Contains(s.supportedAEAD(), id) {
			continue
		}
		if _, err := aeadIDToKeyLen(protocol.AEADAlgorithm(id)); err != nil {
			continue
		}
		return id, compliantExport, nil
	}
	return 0, false, protocolError(errorBadRequest, "no mutually supported AEAD algorithm")
}

// buildResponse derives the directional keys for the negotiated algorithm and
// assembles the NTS-KE response: Next Protocol, AEAD, an optional
// Compliant128GCMExport echo, optional NTP server/port hints, N cookies, and
// End of Message. The compliant-export record is echoed only when the client
// offered it and the negotiated algorithm is AES-128-GCM-SIV.
func (s *Server) buildResponse(cs tls.ConnectionState, aeadID uint16, compliantExport bool) ([]Record, error) {
	// aeadIDToKeyLen lives in keystore.go (same package) — reuse it instead of
	// a duplicate lookup. It takes protocol.AEADAlgorithm, so convert the wire
	// uint16 at the boundary.
	keyLen, err := aeadIDToKeyLen(protocol.AEADAlgorithm(aeadID))
	if err != nil {
		return nil, err
	}
	c2s, err := exportKey(cs, aeadID, directionC2S, keyLen)
	if err != nil {
		return nil, err
	}
	s2c, err := exportKey(cs, aeadID, directionS2C, keyLen)
	if err != nil {
		return nil, err
	}

	records := []Record{
		NewNextProtocol(NextProtocolNTPv4),
		NewAEADAlgorithm(aeadID),
	}
	// Echo the compliant-export record back so the client knows the server agreed:
	// the record is a negotiation, not a one-way announcement. Both sides must
	// independently derive the compliant AES-128-GCM-SIV export context, so the
	// client only switches to it after seeing our echo. Without the echo the client
	// falls back to the default context and the two ends disagree on key material.
	if compliantExport && aeadID == uint16(protocol.AEADAES128GCMSIV) {
		records = append(records, NewCompliant128GCMExport())
	}
	if s.NTPv4Server != "" {
		records = append(records, NewServerNegotiation(s.NTPv4Server))
	}
	if s.NTPv4Port != 0 {
		records = append(records, NewPortNegotiation(s.NTPv4Port))
	}
	for range s.cookieCount() {
		cookie, err := s.Keystore.SealCookie(protocol.AEADAlgorithm(aeadID), c2s, s2c)
		if err != nil {
			return nil, fmt.Errorf("sealing cookie: %w", err)
		}
		records = append(records, NewCookie(cookie))
	}
	return append(records, NewEndOfMessage()), nil
}

// writeError sends a critical Error record followed by End of Message (best effort).
func (s *Server) writeError(w io.Writer, code uint16) {
	_ = s.writeRecords(w, []Record{NewError(code), NewEndOfMessage()})
}

// writeRecords marshals records to the NTS-KE wire format and writes them.
func (s *Server) writeRecords(w io.Writer, records []Record) error {
	b, err := MarshalRecords(records)
	if err != nil {
		return fmt.Errorf("ntske: marshal response: %w", err)
	}
	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("ntske: write response: %w", err)
	}
	return nil
}

// readMessage reads NTS-KE records from r until an End of Message record, never
// consuming more than maxCap octets in total.
func readMessage(r io.Reader, maxCap int) ([]Record, error) {
	// buf grows on demand as records are read; total bounds it by maxCap.
	var buf []byte
	header := make([]byte, recordHeaderLen)
	total := 0
	for {
		if _, err := io.ReadFull(r, header); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrHeaderTruncated, err)
		}
		bodyLen := int(binary.BigEndian.Uint16(header[2:4]))
		total += recordHeaderLen + bodyLen
		// Check cap immediately after computing total and reject before growing
		// buf so the bound is tight and obvious.
		if total > maxCap {
			return nil, fmt.Errorf("%w: message exceeds %d octets", ErrBodyTooLarge, maxCap)
		}
		buf = append(buf, header...)
		body := make([]byte, bodyLen)
		if _, err := io.ReadFull(r, body); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrBodyTruncated, err)
		}
		buf = append(buf, body...)

		typeWord := binary.BigEndian.Uint16(header[0:2])
		if typeWord&typeMask == RecordEndOfMessage {
			return Parse(buf)
		}
	}
}

// exporterContext builds the 5-octet RFC 8915 §4.3 exporter context:
// [Next Protocol:2 BE][AEAD Algorithm:2 BE][direction:1].
func exporterContext(aeadID uint16, direction byte) []byte {
	ctx := make([]byte, 5)
	binary.BigEndian.PutUint16(ctx[0:2], NextProtocolNTPv4)
	binary.BigEndian.PutUint16(ctx[2:4], aeadID)
	ctx[4] = direction
	return ctx
}

// exportKey derives one directional NTS-KE key from the TLS session using the
// RFC 8915 exporter label and the 5-octet context for the given direction.
func exportKey(cs tls.ConnectionState, aeadID uint16, direction byte, keyLen int) ([]byte, error) {
	key, err := cs.ExportKeyingMaterial(exporterLabel, exporterContext(aeadID, direction), keyLen)
	if err != nil {
		return nil, fmt.Errorf("exporting key (direction %d): %w", direction, err)
	}
	return key, nil
}

// --- default accessors ---
func (s *Server) cookieCount() uint16 {
	n := s.Cookies
	if n == 0 {
		n = defaultCookies
	}
	return min(n, maxCookies)
}
func (s *Server) supportedAEAD() []uint16 {
	if len(s.SupportedAEAD) > 0 {
		return s.SupportedAEAD
	}
	return []uint16{
		uint16(protocol.AEADAES128GCMSIV),  // 30
		uint16(protocol.AEADAESSIVCMAC512), // 17
	}
}

// handshakeTimeout returns the effective per-connection deadline duration.
// Despite the historical name, this bounds the entire NTS-KE exchange:
// TLS handshake, record read, validation, cookie sealing, and response write.
func (s *Server) handshakeTimeout() time.Duration {
	if s.HandshakeTimeout > 0 {
		return s.HandshakeTimeout
	}
	return defaultHandshakeTimeout
}
func (s *Server) incHandshakes() {
	if s.Stats != nil {
		s.Stats.IncHandshakes()
	}
}
func (s *Server) incErrors() {
	if s.Stats != nil {
		s.Stats.IncErrors()
	}
}
