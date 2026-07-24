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
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"time"

	"github.com/facebook/time/ntp/protocol"
)

// Client performs the client side of an NTS-KE handshake against a Server.
type Client struct {
	// SupportedAEAD is the client's AEAD preference list, most preferred first.
	// Defaults to [AES-128-GCM-SIV (30), AES-SIV-CMAC-512 (17)] when empty.
	SupportedAEAD []uint16
	// RequestCompliantExport, when true, offers chrony's
	// compliant-128-GCM-SIV-export record in the request.
	RequestCompliantExport bool
	// Timeout bounds the whole exchange (dial + TLS + record read).
	// Defaults to defaultHandshakeTimeout when zero.
	Timeout time.Duration
}

// HandshakeResult is the validated outcome of a successful NTS-KE exchange.
type HandshakeResult struct {
	// NextProtocol is the negotiated next protocol (NextProtocolNTPv4).
	NextProtocol uint16
	// AEAD is the negotiated AEAD algorithm ID.
	AEAD uint16
	// Cookies are the NTS cookies the server issued.
	Cookies [][]byte
	// CompliantExport reports whether the server echoed the chrony
	// compliant-128-GCM-SIV-export record.
	CompliantExport bool
	// C2S and S2C are the directional session keys derived from the TLS session
	// via the RFC 8915 exporter (C2S signs requests, S2C verifies responses).
	C2S []byte
	S2C []byte
}

// ClientTLSConfig builds a TLS 1.3 client config for NTS-KE. When caFile is
// non-empty, only that PEM is trusted (for a self-signed dev cert); otherwise
// the system roots are used.
func ClientTLSConfig(caFile string) (*tls.Config, error) {
	conf := &tls.Config{
		MinVersion: tls.VersionTLS13,
		NextProtos: []string{ALPNProtocol},
	}
	if caFile == "" {
		return conf, nil
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read ca %q: %w", caFile, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("ca %q: no valid certificate found", caFile)
	}
	conf.RootCAs = pool
	return conf, nil
}

// Handshake dials addr over TLS 1.3, runs the NTS-KE exchange, and returns the
// validated result. tlsConf is cloned; MinVersion is pinned to TLS 1.3 and ALPN
// to "ntske/1" so callers cannot accidentally weaken the transport.
func (c *Client) Handshake(ctx context.Context, addr string, tlsConf *tls.Config) (*HandshakeResult, error) {
	if tlsConf == nil {
		return nil, errors.New("ntske: TLSConfig is required")
	}
	tlsConf = tlsConf.Clone()
	tlsConf.MinVersion = tls.VersionTLS13
	tlsConf.NextProtos = []string{ALPNProtocol}

	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()

	dialer := tls.Dialer{Config: tlsConf}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("ntske: dial %q: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	tlsConn := conn.(*tls.Conn)
	if deadline, ok := ctx.Deadline(); ok {
		_ = tlsConn.SetDeadline(deadline)
	}
	if proto := tlsConn.ConnectionState().NegotiatedProtocol; proto != ALPNProtocol {
		return nil, fmt.Errorf("ntske: server negotiated ALPN %q, want %q", proto, ALPNProtocol)
	}

	if err := c.writeRequest(tlsConn); err != nil {
		return nil, fmt.Errorf("ntske: write request: %w", err)
	}
	records, err := readMessage(tlsConn, maxMessageSize)
	if err != nil {
		return nil, fmt.Errorf("ntske: read response: %w", err)
	}
	res, err := c.interpret(records)
	if err != nil {
		return nil, err
	}
	// Derive the directional session keys from the TLS session using the same
	// exporter label and context the server uses, so both ends agree on C2S/S2C.
	keyLen, err := aeadIDToKeyLen(protocol.AEADAlgorithm(res.AEAD))
	if err != nil {
		return nil, fmt.Errorf("ntske: key length for aead %d: %w", res.AEAD, err)
	}
	cs := tlsConn.ConnectionState()
	if res.C2S, err = exportKey(cs, res.AEAD, directionC2S, keyLen); err != nil {
		return nil, fmt.Errorf("ntske: derive C2S: %w", err)
	}
	if res.S2C, err = exportKey(cs, res.AEAD, directionS2C, keyLen); err != nil {
		return nil, fmt.Errorf("ntske: derive S2C: %w", err)
	}
	return res, nil
}

// writeRequest sends the NTS-KE request: Next Protocol NTPv4, the AEAD
// preference list, optionally the compliant-export record, and End of Message.
func (c *Client) writeRequest(w io.Writer) error {
	records := []Record{
		NewNextProtocol(NextProtocolNTPv4),
		NewAEADAlgorithm(c.supportedAEAD()...),
	}
	if c.RequestCompliantExport {
		records = append(records, Record{Type: RecordCompliant128GCMExport})
	}
	records = append(records, NewEndOfMessage())

	b, err := MarshalRecords(records)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// interpret walks the response records, enforces the mandatory fields, and
// validates that the negotiated AEAD is one the client actually offered.
func (c *Client) interpret(records []Record) (*HandshakeResult, error) {
	var (
		res          HandshakeResult
		sawNextProto bool
		sawAEAD      bool
	)
	for _, r := range records {
		switch r.Type {
		case RecordError:
			code, err := ParseUint16s(r.Body)
			if err != nil {
				return nil, fmt.Errorf("server Error record with malformed body: %w", err)
			}
			return nil, fmt.Errorf("server returned Error record, code=%v", code)
		case RecordNextProtocol:
			ids, err := ParseUint16s(r.Body)
			if err != nil {
				return nil, fmt.Errorf("malformed Next Protocol body: %w", err)
			}
			if len(ids) == 0 {
				return nil, errors.New("empty Next Protocol record")
			}
			res.NextProtocol = ids[0]
			sawNextProto = true
		case RecordAEADAlgorithm:
			ids, err := ParseUint16s(r.Body)
			if err != nil {
				return nil, fmt.Errorf("malformed AEAD body: %w", err)
			}
			if len(ids) == 0 {
				return nil, errors.New("empty AEAD record")
			}
			res.AEAD = ids[0]
			sawAEAD = true
		case RecordNewCookie:
			res.Cookies = append(res.Cookies, r.Body)
		case RecordCompliant128GCMExport:
			res.CompliantExport = true
		}
	}

	switch {
	case !sawNextProto:
		return nil, errors.New("response missing Next Protocol record")
	case res.NextProtocol != NextProtocolNTPv4:
		return nil, fmt.Errorf("server selected next-proto %d, want NTPv4 (%d)", res.NextProtocol, NextProtocolNTPv4)
	case !sawAEAD:
		return nil, errors.New("response missing AEAD record")
	case !slices.Contains(c.supportedAEAD(), res.AEAD):
		return nil, fmt.Errorf("server selected AEAD %d not offered by client", res.AEAD)
	case len(res.Cookies) == 0:
		return nil, errors.New("response contained no cookies")
	}
	return &res, nil
}

// supportedAEAD returns the configured preference list or the default
// [AES-128-GCM-SIV (30), AES-SIV-CMAC-512 (17)] when unset.
func (c *Client) supportedAEAD() []uint16 {
	if len(c.SupportedAEAD) > 0 {
		return c.SupportedAEAD
	}
	return []uint16{
		uint16(protocol.AEADAES128GCMSIV),  // 30
		uint16(protocol.AEADAESSIVCMAC512), // 17
	}
}

// timeout returns the configured timeout or defaultHandshakeTimeout when unset.
func (c *Client) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return defaultHandshakeTimeout
}

// NextProtocolName maps an NTS-KE Next Protocol ID to a human-readable name,
// falling back to "unknown(<id>)" for anything other than NTPv4.
func NextProtocolName(id uint16) string {
	if id == NextProtocolNTPv4 {
		return "NTPv4"
	}
	return fmt.Sprintf("unknown(%d)", id)
}
