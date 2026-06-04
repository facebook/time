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

// Package ntrip provides an NTRIP client for pushing RTCM correction data
// to an NTRIP caster. It implements both the NTRIP v1 SOURCE protocol and the
// NTRIP v2 (HTTP POST) protocol, and supports connecting through an HTTP
// CONNECT proxy with TLS client certificate authentication.
package ntrip

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"
)

const defaultUserAgent = "NTRIP ntripper/1.0"

// Config holds the NTRIP caster connection parameters.
type Config struct {
	// Caster is the address of the NTRIP caster (host:port).
	Caster string
	// Mountpoint is the caster mountpoint (e.g., "/MOUNT01").
	Mountpoint string
	// Password is the NTRIP SOURCE / Basic auth password.
	Password string
	// Username is the NTRIP username (used for v2 Basic auth).
	Username string
	// UserAgent is the source agent string sent to the caster.
	// Defaults to "NTRIP ntripper/1.0" if empty.
	UserAgent string
	// Version selects the NTRIP protocol version: 1 (SOURCE) or 2 (HTTP POST).
	// Defaults to 1 if zero.
	Version int
	// Chunked, when true (NTRIP v2 only), sends the body using HTTP chunked
	// transfer encoding, as required by the NTRIP 2.0 NtripServer standard.
	Chunked bool
}

// ProxyConfig holds the HTTP CONNECT proxy parameters.
type ProxyConfig struct {
	// Address is the proxy address (host:port).
	Address string
	// CertFile is the path to the PEM-encoded TLS client certificate.
	CertFile string
	// KeyFile is the path to the PEM-encoded TLS client private key.
	KeyFile string
}

// Option configures a Client.
type Option func(*Client)

// WithProxy configures the client to connect through an HTTP CONNECT proxy.
func WithProxy(cfg ProxyConfig) Option {
	return func(c *Client) {
		c.proxy = &cfg
	}
}

// WithLogger sets the logger for the client.
func WithLogger(logger *slog.Logger) Option {
	return func(c *Client) {
		c.logger = logger
	}
}

// WithDump tees every byte sent to the caster into w, for offline inspection
// of the exact RTCM stream the caster receives.
func WithDump(w io.Writer) Option {
	return func(c *Client) {
		c.dump = w
	}
}

// Client is an NTRIP v1 SOURCE client that pushes RTCM data to a caster.
type Client struct {
	proxy  *ProxyConfig
	logger *slog.Logger
	conn   net.Conn
	dump   io.Writer
	config Config
}

// NewClient creates a new NTRIP client with the given configuration.
func NewClient(cfg Config, opts ...Option) *Client {
	if cfg.UserAgent == "" {
		cfg.UserAgent = defaultUserAgent
	}
	c := &Client{
		config: cfg,
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Connect establishes a connection to the NTRIP caster. If a proxy is
// configured, it first establishes an HTTP CONNECT tunnel through the proxy
// with TLS client certificate authentication. Then it performs the NTRIP
// handshake (v1 SOURCE or v2 HTTP POST) and waits for the caster's
// acceptance response before allowing data writes.
func (c *Client) Connect(ctx context.Context) error {
	conn, err := c.dial(ctx)
	if err != nil {
		return fmt.Errorf("dialing caster: %w", err)
	}

	if c.config.Version == 2 {
		err = c.postHandshake(conn)
	} else {
		err = c.sourceHandshake(conn)
	}
	if err != nil {
		conn.Close()
		return fmt.Errorf("NTRIP handshake: %w", err)
	}

	// Wait for caster response before allowing data writes.
	// v1 casters reply "ICY 200 OK", v2 casters reply "HTTP/1.1 200 OK".
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		conn.Close()
		return fmt.Errorf("setting read deadline: %w", err)
	}
	// TCP may split the response across segments, so read the full status line
	// rather than relying on whatever a single Read happens to return.
	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		conn.Close()
		return fmt.Errorf("reading caster response: %w", err)
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		conn.Close()
		return fmt.Errorf("clearing read deadline: %w", err)
	}
	resp := strings.TrimSpace(statusLine)
	c.logger.Info("received from caster", "data", resp)
	// The status line is "ICY 200 OK" (v1) or "HTTP/1.1 200 OK" (v2); the code
	// is the second field. Check it exactly rather than substring-matching
	// "200", which would also match "1200", "200ms", etc.
	fields := strings.Fields(resp)
	if len(fields) < 2 || fields[1] != "200" {
		conn.Close()
		return fmt.Errorf("caster rejected: %s", resp)
	}

	c.conn = conn

	// Drain any further responses from the caster in the background, reusing the
	// reader so bytes buffered past the status line are not lost.
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				c.logger.Info("received from caster", "data", string(buf[:n]))
			}
			if err != nil {
				c.logger.Debug("caster read closed", "error", err)
				return
			}
		}
	}()

	return nil
}

// Close closes the connection to the caster.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// Write sends raw RTCM data to the caster. In NTRIP v2 chunked mode each call
// is wrapped in a single HTTP chunk. The dump (if any) always receives the raw
// RTCM bytes, not the chunk framing, so captures remain valid RTCM3 streams.
func (c *Client) Write(p []byte) (int, error) {
	if c.conn == nil {
		return 0, fmt.Errorf("not connected")
	}
	if c.dump != nil {
		_, _ = c.dump.Write(p)
	}
	if c.config.Version == 2 && c.config.Chunked {
		buf := make([]byte, 0, len(p)+16)
		buf = append(buf, fmt.Sprintf("%X\r\n", len(p))...)
		buf = append(buf, p...)
		buf = append(buf, '\r', '\n')
		if _, err := c.conn.Write(buf); err != nil {
			return 0, err
		}
		return len(p), nil
	}
	return c.conn.Write(p)
}

// dial establishes a TCP connection to the caster, optionally through a proxy.
func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	if c.proxy != nil {
		return c.dialViaProxy(ctx)
	}
	d := net.Dialer{KeepAlive: 30 * time.Second}
	return d.DialContext(ctx, "tcp", c.config.Caster)
}

// dialViaProxy establishes a connection through an HTTP CONNECT proxy with
// TLS client certificate authentication, creating a raw TCP tunnel.
func (c *Client) dialViaProxy(ctx context.Context) (net.Conn, error) {
	cert, err := tls.LoadX509KeyPair(c.proxy.CertFile, c.proxy.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("loading proxy TLS certificate: %w", err)
	}

	host, _, err := net.SplitHostPort(c.proxy.Address)
	if err != nil {
		return nil, fmt.Errorf("parsing proxy address: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ServerName:   host,
	}

	d := net.Dialer{KeepAlive: 30 * time.Second}
	rawConn, err := d.DialContext(ctx, "tcp", c.proxy.Address)
	if err != nil {
		return nil, fmt.Errorf("connecting to proxy: %w", err)
	}

	tlsConn := tls.Client(rawConn, tlsConfig)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("proxy TLS handshake: %w", err)
	}

	connectReq := fmt.Sprintf(
		"CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n",
		c.config.Caster, c.config.Caster,
	)
	if _, err := tlsConn.Write([]byte(connectReq)); err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("sending CONNECT request: %w", err)
	}

	reader := bufio.NewReader(tlsConn)
	line, err := reader.ReadString('\n')
	if err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("reading proxy response: %w", err)
	}
	if !strings.Contains(line, "200") {
		tlsConn.Close()
		return nil, fmt.Errorf("proxy CONNECT failed: %s", strings.TrimSpace(line))
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			tlsConn.Close()
			return nil, fmt.Errorf("reading proxy response headers: %w", err)
		}
		if strings.TrimSpace(line) == "" {
			break
		}
	}

	c.logger.Debug("proxy tunnel established", "proxy", c.proxy.Address)

	return &bufferedConn{
		reader: reader,
		Conn:   tlsConn,
	}, nil
}

// bufferedConn wraps a net.Conn to use a bufio.Reader for reads.
type bufferedConn struct {
	reader *bufio.Reader
	net.Conn
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

// sourceHandshake sends the NTRIP v1 SOURCE request and streams raw RTCM
// data after the headers. The request uses a bare mountpoint (no leading
// slash) plus a STR line, matching the de-facto format produced by RTKLIB's
// str2str that NTRIP casters accept.
func (c *Client) sourceHandshake(conn net.Conn) error {
	mount := strings.TrimPrefix(c.config.Mountpoint, "/")
	req := fmt.Sprintf(
		"SOURCE %s %s\r\nSource-Agent: %s\r\nSTR: \r\n\r\n",
		c.config.Password, mount, c.config.UserAgent,
	)

	c.logger.Info("sending SOURCE request",
		"caster", c.config.Caster,
		"mountpoint", mount,
	)
	if _, err := conn.Write([]byte(req)); err != nil {
		return fmt.Errorf("sending SOURCE request: %w", err)
	}

	return nil
}

// postHandshake sends the NTRIP v2 (HTTP POST) server request. The caster keeps
// the connection open and ingests the request body as an RTCM stream. Some
// casters that advertise "Ntrip-Version: Ntrip/2.0" require this and do not
// enter stream-ingestion mode for a bare v1 SOURCE request.
func (c *Client) postHandshake(conn net.Conn) error {
	mount := strings.TrimPrefix(c.config.Mountpoint, "/")
	auth := base64.StdEncoding.EncodeToString(
		[]byte(c.config.Username + ":" + c.config.Password),
	)

	var b strings.Builder
	fmt.Fprintf(&b, "POST /%s HTTP/1.1\r\n", mount)
	fmt.Fprintf(&b, "Host: %s\r\n", c.config.Caster)
	fmt.Fprintf(&b, "Ntrip-Version: Ntrip/2.0\r\n")
	fmt.Fprintf(&b, "User-Agent: %s\r\n", c.config.UserAgent)
	fmt.Fprintf(&b, "Authorization: Basic %s\r\n", auth)
	// A streaming source keeps the connection open; do not announce close.
	fmt.Fprintf(&b, "Connection: keep-alive\r\n")
	if c.config.Chunked {
		fmt.Fprintf(&b, "Transfer-Encoding: chunked\r\n")
	}
	fmt.Fprintf(&b, "\r\n")

	c.logger.Info("sending NTRIP v2 POST request",
		"caster", c.config.Caster,
		"mountpoint", mount,
		"chunked", c.config.Chunked,
	)
	if _, err := conn.Write([]byte(b.String())); err != nil {
		return fmt.Errorf("sending POST request: %w", err)
	}

	return nil
}
