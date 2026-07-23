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
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/facebook/time/ntp/protocol"
	"github.com/stretchr/testify/require"
)

// gcmSIV and sivCMAC are the two AEAD IDs as they travel on the wire (uint16),
// spelled out here so the tests read against the negotiation, not the crypto.
const (
	gcmSIV  = uint16(protocol.AEADAES128GCMSIV)  // 30
	sivCMAC = uint16(protocol.AEADAESSIVCMAC512) // 17
)

// countingStats is a Stats implementation that tallies calls. Uses atomics so
// the -race detector stays quiet when the server calls it from its own
// goroutine while the test reads the counters.
type countingStats struct {
	handshakes    atomic.Int64
	errors        atomic.Int64
	cookiesIssued atomic.Int64
}

func (s *countingStats) IncHandshakes()         { s.handshakes.Add(1) }
func (s *countingStats) IncErrors()             { s.errors.Add(1) }
func (s *countingStats) AddCookiesIssued(n int) { s.cookiesIssued.Add(int64(n)) }

// newTestTLSConfigs returns a matched server/client pair backed by a throwaway
// self-signed Ed25519 certificate, both pinned to TLS 1.3 and ALPN "ntske/1".
func newTestTLSConfigs(t *testing.T) (server, client *tls.Config) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "ntske-test"},
		NotBefore:    time.Unix(0, 0),
		NotAfter:     time.Unix(1<<31-1, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	require.NoError(t, err)

	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}
	server = &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
		NextProtos:   []string{ALPNProtocol},
	}
	client = &tls.Config{
		InsecureSkipVerify: true, // #nosec G402 -- test-only self-signed cert
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{ALPNProtocol},
	}
	return server, client
}

// runExchange spins up srv.handleConnection over an in-memory net.Pipe, drives
// the client-side TLS handshake with clientTLS, sends request, and returns the
// client tls.Conn plus the records the server wrote back. The server goroutine
// has finished by the time this returns.
func runExchange(t *testing.T, srv *Server, clientTLS *tls.Config, request []Record) (*tls.Conn, []Record) {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	tlsServer := tls.Server(serverConn, srv.TLSConfig)
	tlsClient := tls.Client(clientConn, clientTLS)

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.handleConnection(context.Background(), tlsServer)
	}()

	require.NoError(t, tlsClient.HandshakeContext(context.Background()))

	if len(request) > 0 {
		req, err := MarshalRecords(request)
		require.NoError(t, err)
		_, err = tlsClient.Write(req)
		require.NoError(t, err)
	}

	// Read the server's reply before waiting on the goroutine: net.Pipe writes
	// block until read, so the server cannot finish until we drain the response.
	resp, err := readMessage(tlsClient, maxMessageSize)
	require.NoError(t, err)
	<-done
	return tlsClient, resp
}

// recordsByType groups a record slice by type for convenient assertions.
func recordsByType(records []Record) map[uint16][]Record {
	out := make(map[uint16][]Record)
	for _, r := range records {
		out[r.Type] = append(out[r.Type], r)
	}
	return out
}

// parseUint16s is a test helper that decodes a uint16 body, panicking on error
// because test fixtures are expected to be well-formed. It wraps ParseUint16s
// which returns ErrOddLengthBody for malformed odd-length bodies.
func parseUint16s(b []byte) []uint16 {
	v, err := ParseUint16s(b)
	if err != nil {
		panic(err)
	}
	return v
}

// TestServerIssuesCookies is the end-to-end happy path:
// TLS 1.3 handshake, negotiates AES-128-GCM-SIV, and the server returns the
// requested number of cookies. Each cookie must open under the keystore and
// yield exactly the C2S/S2C keys the client derives independently from the TLS
// session — proving the exporter label and context match on both sides.
func TestServerIssuesCookies(t *testing.T) {
	serverTLS, clientTLS := newTestTLSConfigs(t)
	ks, err := NewInMemoryKeystore(InMemoryKeystoreOptions{})
	require.NoError(t, err)
	stats := &countingStats{}
	srv := &Server{TLSConfig: serverTLS, Keystore: ks, Cookies: 4, Stats: stats}

	tlsClient, resp := runExchange(t, srv, clientTLS, []Record{
		NewNextProtocol(NextProtocolNTPv4),
		NewAEADAlgorithm(gcmSIV),
		NewEndOfMessage(),
	})

	byType := recordsByType(resp)
	require.Len(t, byType[RecordNextProtocol], 1)
	require.Len(t, byType[RecordAEADAlgorithm], 1)
	require.Len(t, byType[RecordEndOfMessage], 1)
	require.Equal(t, []uint16{gcmSIV}, parseUint16s(byType[RecordAEADAlgorithm][0].Body))
	require.Equal(t, RecordEndOfMessage, resp[len(resp)-1].Type, "EOM must be last")

	cookies := byType[RecordNewCookie]
	require.Len(t, cookies, 4)

	// The client derives the same keys the server sealed into every cookie.
	cs := tlsClient.ConnectionState()
	wantC2S, err := cs.ExportKeyingMaterial(exporterLabel, exporterContext(gcmSIV, directionC2S), 16)
	require.NoError(t, err)
	wantS2C, err := cs.ExportKeyingMaterial(exporterLabel, exporterContext(gcmSIV, directionS2C), 16)
	require.NoError(t, err)

	for _, c := range cookies {
		id, c2s, s2c, err := ks.OpenCookie(c.Body)
		require.NoError(t, err)
		require.Equal(t, protocol.AEADAES128GCMSIV, id)
		require.Equal(t, wantC2S, c2s)
		require.Equal(t, wantS2C, s2c)
	}

	require.Equal(t, int64(1), stats.handshakes.Load())
	require.Equal(t, int64(0), stats.errors.Load())
	require.Equal(t, int64(4), stats.cookiesIssued.Load())
}

// TestServerDefaultCookieCount checks that a Server with Cookies unset issues
// the documented default of 8.
func TestServerDefaultCookieCount(t *testing.T) {
	serverTLS, clientTLS := newTestTLSConfigs(t)
	ks, err := NewInMemoryKeystore(InMemoryKeystoreOptions{})
	require.NoError(t, err)
	srv := &Server{TLSConfig: serverTLS, Keystore: ks}

	_, resp := runExchange(t, srv, clientTLS, []Record{
		NewNextProtocol(NextProtocolNTPv4),
		NewAEADAlgorithm(gcmSIV),
		NewEndOfMessage(),
	})
	require.Len(t, recordsByType(resp)[RecordNewCookie], 8)
}

// TestServerClampsCookieCount checks that a Cookies value above the 32-cookie
// upper bound is clamped, so a large or misconfigured value cannot be turned
// into a per-handshake cookie-sealing / response-size amplification vector.
func TestServerClampsCookieCount(t *testing.T) {
	serverTLS, clientTLS := newTestTLSConfigs(t)
	ks, err := NewInMemoryKeystore(InMemoryKeystoreOptions{})
	require.NoError(t, err)
	srv := &Server{TLSConfig: serverTLS, Keystore: ks, Cookies: 1000}

	_, resp := runExchange(t, srv, clientTLS, []Record{
		NewNextProtocol(NextProtocolNTPv4),
		NewAEADAlgorithm(gcmSIV),
		NewEndOfMessage(),
	})
	require.Len(t, recordsByType(resp)[RecordNewCookie], 32)
}

// TestServerAEADPreferenceOrder verifies that when the client lists several
// algorithms the server supports, the server honours the client's first
// preference rather than its own default order.
func TestServerAEADPreferenceOrder(t *testing.T) {
	serverTLS, clientTLS := newTestTLSConfigs(t)
	ks, err := NewInMemoryKeystore(InMemoryKeystoreOptions{})
	require.NoError(t, err)
	// Default SupportedAEAD is [30, 17]; client prefers 17 first.
	srv := &Server{TLSConfig: serverTLS, Keystore: ks, Cookies: 1}

	_, resp := runExchange(t, srv, clientTLS, []Record{
		NewNextProtocol(NextProtocolNTPv4),
		NewAEADAlgorithm(sivCMAC, gcmSIV),
		NewEndOfMessage(),
	})
	aead := recordsByType(resp)[RecordAEADAlgorithm]
	require.Len(t, aead, 1)
	require.Equal(t, []uint16{sivCMAC}, parseUint16s(aead[0].Body), "client's first preference wins")
}

// TestServerCompliant128GCMExport verifies the compliant-export negotiation:
// the server echoes a Compliant128GCMExport record only when the client offers
// it AND the negotiated AEAD is AES-128-GCM-SIV. If the client omits the record,
// or a non-GCM-SIV algorithm is chosen, no such record appears in the response.
func TestServerCompliant128GCMExport(t *testing.T) {
	cases := []struct {
		name    string
		request []Record
		want    bool
	}{
		{
			name: "offered and GCM-SIV chosen: echoed",
			request: []Record{
				NewNextProtocol(NextProtocolNTPv4),
				NewAEADAlgorithm(gcmSIV),
				NewCompliant128GCMExport(),
				NewEndOfMessage(),
			},
			want: true,
		},
		{
			name: "offered but SIV-CMAC chosen: not echoed",
			request: []Record{
				NewNextProtocol(NextProtocolNTPv4),
				NewAEADAlgorithm(sivCMAC),
				NewCompliant128GCMExport(),
				NewEndOfMessage(),
			},
			want: false,
		},
		{
			name: "not offered, GCM-SIV chosen: not echoed",
			request: []Record{
				NewNextProtocol(NextProtocolNTPv4),
				NewAEADAlgorithm(gcmSIV),
				NewEndOfMessage(),
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			serverTLS, clientTLS := newTestTLSConfigs(t)
			ks, err := NewInMemoryKeystore(InMemoryKeystoreOptions{})
			require.NoError(t, err)
			srv := &Server{TLSConfig: serverTLS, Keystore: ks, Cookies: 1}

			_, resp := runExchange(t, srv, clientTLS, tc.request)
			echoed := recordsByType(resp)[RecordCompliant128GCMExport]
			if tc.want {
				require.Len(t, echoed, 1)
				require.False(t, echoed[0].Critical)
				require.Empty(t, echoed[0].Body)
			} else {
				require.Empty(t, echoed)
			}
		})
	}
}

// TestServerRejectsWrongALPN checks that a client which completes TLS 1.3 but
// does not select "ntske/1" is dropped without a response and counted as an
// error, and that no handshake is credited.
func TestServerRejectsWrongALPN(t *testing.T) {
	serverTLS, clientTLS := newTestTLSConfigs(t)
	clientTLS.NextProtos = nil // offer no ALPN, so negotiation yields ""
	ks, err := NewInMemoryKeystore(InMemoryKeystoreOptions{})
	require.NoError(t, err)
	stats := &countingStats{}
	srv := &Server{TLSConfig: serverTLS, Keystore: ks, Stats: stats}

	serverConn, clientConn := net.Pipe()
	tlsServer := tls.Server(serverConn, serverTLS)
	tlsClient := tls.Client(clientConn, clientTLS)

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.handleConnection(context.Background(), tlsServer)
	}()

	require.NoError(t, tlsClient.HandshakeContext(context.Background()))
	<-done

	require.Equal(t, int64(0), stats.handshakes.Load(), "wrong ALPN must not credit a handshake")
	require.Equal(t, int64(1), stats.errors.Load())
	require.Equal(t, int64(0), stats.cookiesIssued.Load())
}

// TestServerErrorResponses exercises the two protocol rejections the client can
// observe: an unrecognized critical record yields Error(0), and a request whose
// AEAD list intersects nothing the server supports yields Error(1).
func TestServerErrorResponses(t *testing.T) {
	cases := []struct {
		name    string
		request []Record
		want    uint16
	}{
		{
			name: "unknown critical record",
			request: []Record{
				NewNextProtocol(NextProtocolNTPv4),
				NewAEADAlgorithm(gcmSIV),
				{Critical: true, Type: 999, Body: nil},
				NewEndOfMessage(),
			},
			want: errorUnrecognizedCriticalRecord,
		},
		{
			name: "no mutually supported AEAD",
			request: []Record{
				NewNextProtocol(NextProtocolNTPv4),
				NewAEADAlgorithm(0xBEEF),
				NewEndOfMessage(),
			},
			want: errorBadRequest,
		},
		{
			name: "next protocol without NTPv4",
			request: []Record{
				NewNextProtocol(0x0007),
				NewAEADAlgorithm(gcmSIV),
				NewEndOfMessage(),
			},
			want: errorBadRequest,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			serverTLS, clientTLS := newTestTLSConfigs(t)
			ks, err := NewInMemoryKeystore(InMemoryKeystoreOptions{})
			require.NoError(t, err)
			srv := &Server{TLSConfig: serverTLS, Keystore: ks}

			_, resp := runExchange(t, srv, clientTLS, tc.request)
			require.Len(t, resp, 2)
			require.Equal(t, RecordError, resp[0].Type)
			require.Equal(t, []uint16{tc.want}, parseUint16s(resp[0].Body))
			require.Equal(t, RecordEndOfMessage, resp[1].Type)
		})
	}
}

// TestListenAndServeRequiresConfig checks the two guard clauses that reject an
// unusable Server before any listener is created.
func TestListenAndServeRequiresConfig(t *testing.T) {
	ks, err := NewInMemoryKeystore(InMemoryKeystoreOptions{})
	require.NoError(t, err)
	serverTLS, _ := newTestTLSConfigs(t)

	t.Run("missing TLSConfig", func(t *testing.T) {
		srv := &Server{Keystore: ks}
		err := srv.ListenAndServe(context.Background(), "127.0.0.1:0")
		require.ErrorContains(t, err, "TLSConfig is required")
	})

	t.Run("missing Keystore", func(t *testing.T) {
		srv := &Server{TLSConfig: serverTLS}
		err := srv.ListenAndServe(context.Background(), "127.0.0.1:0")
		require.ErrorContains(t, err, "Keystore is required")
	})
}

// TestListenAndServeListenError checks that a listen failure (here, an
// out-of-range port) is surfaced rather than swallowed.
func TestListenAndServeListenError(t *testing.T) {
	ks, err := NewInMemoryKeystore(InMemoryKeystoreOptions{})
	require.NoError(t, err)
	serverTLS, _ := newTestTLSConfigs(t)
	srv := &Server{TLSConfig: serverTLS, Keystore: ks}

	err = srv.ListenAndServe(context.Background(), "127.0.0.1:99999999")
	require.Error(t, err)
	require.ErrorContains(t, err, "listen")
}

// TestListenAndServe drives the real accept loop over a loopback listener:
// a client completes the full NTS-KE exchange against the served port, and
// cancelling the context cleanly stops the loop with a nil return.
func TestListenAndServe(t *testing.T) {
	// Grab a free loopback port, then release it for the server to rebind.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := probe.Addr().String()
	require.NoError(t, probe.Close())

	serverTLS, clientTLS := newTestTLSConfigs(t)
	ks, err := NewInMemoryKeystore(InMemoryKeystoreOptions{})
	require.NoError(t, err)
	stats := &countingStats{}
	srv := &Server{TLSConfig: serverTLS, Keystore: ks, Cookies: 2, Stats: stats}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe(ctx, addr) }()

	// The accept loop starts asynchronously; retry until the port is bound.
	var conn *tls.Conn
	require.Eventually(t, func() bool {
		c, derr := tls.Dial("tcp", addr, clientTLS)
		if derr != nil {
			return false
		}
		conn = c
		return true
	}, 2*time.Second, 10*time.Millisecond)
	defer func() { _ = conn.Close() }()

	req, err := MarshalRecords([]Record{
		NewNextProtocol(NextProtocolNTPv4),
		NewAEADAlgorithm(gcmSIV),
		NewEndOfMessage(),
	})
	require.NoError(t, err)
	_, err = conn.Write(req)
	require.NoError(t, err)

	resp, err := readMessage(conn, maxMessageSize)
	require.NoError(t, err)
	require.Len(t, recordsByType(resp)[RecordNewCookie], 2)
	require.Equal(t, int64(1), stats.handshakes.Load())

	// Cancelling the context closes the listener; the accept loop returns nil.
	cancel()
	select {
	case err := <-errCh:
		require.NoError(t, err, "ctx cancellation is a clean shutdown")
	case <-time.After(2 * time.Second):
		t.Fatal("ListenAndServe did not return after context cancellation")
	}
}

// TestValidateRequest unit-tests the pure negotiation logic, including the
// missing-EOM branch that readMessage otherwise guarantees end-to-end.
func TestValidateRequest(t *testing.T) {
	srv := &Server{} // uses default SupportedAEAD [30, 17]

	t.Run("happy path returns chosen AEAD", func(t *testing.T) {
		id, _, err := srv.validateRequest([]Record{
			NewNextProtocol(NextProtocolNTPv4),
			NewAEADAlgorithm(sivCMAC, gcmSIV),
			NewEndOfMessage(),
		})
		require.NoError(t, err)
		require.Equal(t, sivCMAC, id)
	})

	t.Run("unknown non-critical record is ignored", func(t *testing.T) {
		_, _, err := srv.validateRequest([]Record{
			NewNextProtocol(NextProtocolNTPv4),
			NewAEADAlgorithm(gcmSIV),
			{Critical: false, Type: 999},
			NewEndOfMessage(),
		})
		require.NoError(t, err)
	})

	t.Run("reports compliant-export offer", func(t *testing.T) {
		_, offered, err := srv.validateRequest([]Record{
			NewNextProtocol(NextProtocolNTPv4),
			NewAEADAlgorithm(gcmSIV),
			NewCompliant128GCMExport(),
			NewEndOfMessage(),
		})
		require.NoError(t, err)
		require.True(t, offered)
	})

	t.Run("no compliant-export offer when absent", func(t *testing.T) {
		_, offered, err := srv.validateRequest([]Record{
			NewNextProtocol(NextProtocolNTPv4),
			NewAEADAlgorithm(gcmSIV),
			NewEndOfMessage(),
		})
		require.NoError(t, err)
		require.False(t, offered)
	})

	errCases := []struct {
		name    string
		records []Record
		code    uint16
	}{
		{"missing EOM", []Record{NewNextProtocol(NextProtocolNTPv4), NewAEADAlgorithm(gcmSIV)}, errorBadRequest},
		{"missing next protocol", []Record{NewAEADAlgorithm(gcmSIV), NewEndOfMessage()}, errorBadRequest},
		{"missing AEAD", []Record{NewNextProtocol(NextProtocolNTPv4), NewEndOfMessage()}, errorBadRequest},
		{"duplicate next protocol", []Record{
			NewNextProtocol(NextProtocolNTPv4), NewNextProtocol(NextProtocolNTPv4),
			NewAEADAlgorithm(gcmSIV), NewEndOfMessage(),
		}, errorBadRequest},
		{"duplicate AEAD", []Record{
			NewNextProtocol(NextProtocolNTPv4),
			NewAEADAlgorithm(gcmSIV), NewAEADAlgorithm(sivCMAC),
			NewEndOfMessage(),
		}, errorBadRequest},
		{"unknown critical", []Record{
			NewNextProtocol(NextProtocolNTPv4), NewAEADAlgorithm(gcmSIV),
			{Critical: true, Type: 999}, NewEndOfMessage(),
		}, errorUnrecognizedCriticalRecord},
		{"odd length next protocol", []Record{
			{Critical: true, Type: RecordNextProtocol, Body: []byte{0x00}},
			NewAEADAlgorithm(gcmSIV), NewEndOfMessage(),
		}, errorBadRequest},
		{"odd length AEAD", []Record{
			NewNextProtocol(NextProtocolNTPv4),
			{Type: RecordAEADAlgorithm, Body: []byte{0x00, 0x1e, 0xFF}},
			NewEndOfMessage(),
		}, errorBadRequest},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := srv.validateRequest(tc.records)
			var ce *cookieError
			require.ErrorAs(t, err, &ce)
			require.Equal(t, tc.code, ce.code)
		})
	}
}

// TestExporterContext pins the exact 5-octet RFC 8915 §4.3 context bytes for
// each direction: [NextProto:2][AEAD:2][direction:1].
func TestExporterContext(t *testing.T) {
	require.Equal(t, []byte{0x00, 0x00, 0x00, 0x1e, 0x00}, exporterContext(gcmSIV, directionC2S))
	require.Equal(t, []byte{0x00, 0x00, 0x00, 0x1e, 0x01}, exporterContext(gcmSIV, directionS2C))
}

// TestParseUint16sRejectsOddLength checks the decoder round-trips MarshalUint16s and rejects
// odd-length bodies with ErrOddLengthBody per RFC 8915.
func TestParseUint16sRejectsOddLength(t *testing.T) {
	vals, err := ParseUint16s([]byte{0, 1, 0, 30})
	require.NoError(t, err)
	require.Equal(t, []uint16{1, 30}, vals)

	vals, err = ParseUint16s([]byte{0, 1, 0xFF})
	require.ErrorIs(t, err, ErrOddLengthBody)
	require.Nil(t, vals)

	vals, err = ParseUint16s(nil)
	require.NoError(t, err)
	require.Empty(t, vals)
}

// TestReadMessage covers framing: records are read up to (and including) End of
// Message, trailing bytes after EOM are left unread, the byte cap is enforced,
// and truncated headers/bodies surface the records.go sentinels.
func TestReadMessage(t *testing.T) {
	valid, err := MarshalRecords([]Record{
		NewNextProtocol(NextProtocolNTPv4),
		NewAEADAlgorithm(gcmSIV),
		NewEndOfMessage(),
	})
	require.NoError(t, err)

	t.Run("reads to EOM and ignores trailing bytes", func(t *testing.T) {
		buf := append(bytes.Clone(valid), 0xDE, 0xAD, 0xBE, 0xEF)
		got, err := readMessage(bytes.NewReader(buf), maxMessageSize)
		require.NoError(t, err)
		require.Len(t, got, 3)
		require.Equal(t, RecordEndOfMessage, got[2].Type)
	})

	t.Run("enforces the byte cap", func(t *testing.T) {
		_, err := readMessage(bytes.NewReader(valid), 4)
		require.ErrorIs(t, err, ErrBodyTooLarge)
	})

	t.Run("truncated header", func(t *testing.T) {
		_, err := readMessage(bytes.NewReader([]byte{0x00, 0x01}), maxMessageSize)
		require.ErrorIs(t, err, ErrHeaderTruncated)
	})

	t.Run("truncated body", func(t *testing.T) {
		// Header claims a 10-octet body but only 2 follow.
		_, err := readMessage(bytes.NewReader([]byte{0x80, 0x01, 0x00, 0x0A, 0x00, 0x00}), maxMessageSize)
		require.ErrorIs(t, err, ErrBodyTruncated)
	})
}
