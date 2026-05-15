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
// to an NTRIP caster. It implements the NTRIP v1 SOURCE protocol and
// supports connecting through an HTTP CONNECT proxy with TLS client
// certificate authentication.
package ntrip

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"strings"
)

const defaultUserAgent = "NTRIP rtcm/1.0"

// Config holds the NTRIP caster connection parameters.
type Config struct {
	// Caster is the address of the NTRIP caster (host:port).
	Caster string
	// Mountpoint is the caster mountpoint (e.g., "/MOUNT01").
	Mountpoint string
	// Password is the NTRIP v1 SOURCE password.
	Password string
	// Username is an optional username for identification.
	Username string
	// UserAgent is the source agent string sent to the caster.
	// Defaults to "NTRIP rtcm/1.0" if empty.
	UserAgent string
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

// Client is an NTRIP v1 SOURCE client that pushes RTCM data to a caster.
// It implements io.WriteCloser.
type Client struct {
	config Config
	proxy  *ProxyConfig
	logger *slog.Logger
	conn   net.Conn
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
// with TLS client certificate authentication. Then it performs the NTRIP v1
// SOURCE handshake.
func (c *Client) Connect(ctx context.Context) error {
	conn, err := c.dial(ctx)
	if err != nil {
		return fmt.Errorf("dialing caster: %w", err)
	}

	if err := c.sourceHandshake(conn); err != nil {
		conn.Close()
		return fmt.Errorf("NTRIP SOURCE handshake: %w", err)
	}

	c.conn = conn
	c.logger.Info("connected to NTRIP caster",
		"caster", c.config.Caster,
		"mountpoint", c.config.Mountpoint,
	)
	return nil
}

// Write sends RTCM data to the caster. The client must be connected.
func (c *Client) Write(p []byte) (int, error) {
	if c.conn == nil {
		return 0, fmt.Errorf("not connected")
	}
	return c.conn.Write(p)
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

// dial establishes a TCP connection to the caster, optionally through a proxy.
func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	if c.proxy != nil {
		return c.dialViaProxy(ctx)
	}
	var d net.Dialer
	return d.DialContext(ctx, "tcp", c.config.Caster)
}

// dialViaProxy establishes a connection through an HTTP CONNECT proxy with
// TLS client certificate authentication.
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

	var d net.Dialer
	rawConn, err := d.DialContext(ctx, "tcp", c.proxy.Address)
	if err != nil {
		return nil, fmt.Errorf("connecting to proxy: %w", err)
	}

	tlsConn := tls.Client(rawConn, tlsConfig)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("proxy TLS handshake: %w", err)
	}

	// Send HTTP CONNECT request.
	connectReq := fmt.Sprintf(
		"CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n",
		c.config.Caster, c.config.Caster,
	)
	if _, err := tlsConn.Write([]byte(connectReq)); err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("sending CONNECT request: %w", err)
	}

	// Read proxy response.
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

	// Consume remaining response headers.
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

	// Return a connection that reads from the buffered reader (which may
	// have consumed bytes beyond the CONNECT response) and writes directly.
	return &bufferedConn{
		reader: reader,
		Conn:   tlsConn,
	}, nil
}

// sourceHandshake performs the NTRIP v1 SOURCE handshake.
// Protocol:
//
//	Client sends:  SOURCE <password> <mountpoint>\r\n
//	               Source-Agent: <useragent>\r\n
//	               \r\n
//	Server sends:  ICY 200 OK\r\n
//	               \r\n
func (c *Client) sourceHandshake(conn net.Conn) error {
	req := fmt.Sprintf(
		"SOURCE %s %s\r\nSource-Agent: %s\r\n\r\n",
		c.config.Password, c.config.Mountpoint, c.config.UserAgent,
	)
	if _, err := conn.Write([]byte(req)); err != nil {
		return fmt.Errorf("sending SOURCE request: %w", err)
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading caster response: %w", err)
	}

	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "ICY 200") {
		return fmt.Errorf("caster rejected connection: %s", line)
	}

	// Consume remaining response headers until empty line.
	for {
		hdr, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading caster response headers: %w", err)
		}
		if strings.TrimSpace(hdr) == "" {
			break
		}
	}

	return nil
}

// bufferedConn wraps a net.Conn to use a bufio.Reader for reads.
// This is needed after proxy CONNECT because the bufio.Reader may have
// buffered data beyond the HTTP response that we need to read.
type bufferedConn struct {
	reader *bufio.Reader
	net.Conn
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}
