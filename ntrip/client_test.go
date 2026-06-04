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

package ntrip

import (
	"bufio"
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSourceHandshakeSuccess(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	ntripClient := NewClient(Config{
		Caster:     "caster.example.com:2101",
		Mountpoint: "/MOUNT01",
		Password:   "secret",
		Username:   "user1",
		UserAgent:  "NTRIP TestAgent/1.0",
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- ntripClient.sourceHandshake(client)
	}()

	// Verify the SOURCE request from the client.
	reader := bufio.NewReader(server)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "SOURCE secret MOUNT01\r\n", line)

	line, err = reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "Source-Agent: NTRIP TestAgent/1.0\r\n", line)

	line, err = reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "STR: \r\n", line)

	line, err = reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "\r\n", line)

	require.NoError(t, <-errCh)
}

func TestSourceHandshakeRequestFormat(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	cfg := Config{
		Caster:     "caster.example.com:2101",
		Mountpoint: "/EXAMPLE_MOUNT",
		Password:   "my$ecr3t",
		Username:   "user1",
		UserAgent:  "NTRIP MyApp/1.0",
	}
	ntripClient := NewClient(cfg)

	errCh := make(chan error, 1)
	go func() {
		errCh <- ntripClient.sourceHandshake(client)
	}()

	// Read the complete request.
	reader := bufio.NewReader(server)
	var request strings.Builder
	for {
		line, err := reader.ReadString('\n')
		require.NoError(t, err)
		request.WriteString(line)
		if strings.TrimSpace(line) == "" {
			break
		}
	}

	expected := "SOURCE my$ecr3t EXAMPLE_MOUNT\r\nSource-Agent: NTRIP MyApp/1.0\r\nSTR: \r\n\r\n"
	require.Equal(t, expected, request.String())
	require.NoError(t, <-errCh)
}

func TestSourceHandshakeMountpointStripsSlash(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	ntripClient := NewClient(Config{
		Caster:     "caster.example.com:2101",
		Mountpoint: "/MOUNT01",
		Password:   "secret",
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- ntripClient.sourceHandshake(client)
	}()

	reader := bufio.NewReader(server)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	// NTRIP v1 SOURCE uses a bare mountpoint with no leading slash, matching
	// str2str/RTKLIB; the configured "/MOUNT01" must be stripped to "MOUNT01".
	require.Equal(t, "SOURCE secret MOUNT01\r\n", line)
	require.NoError(t, <-errCh)
}

func TestSourceHandshakeWriteError(t *testing.T) {
	client, server := net.Pipe()
	server.Close() // Close immediately to cause write error.

	ntripClient := NewClient(Config{
		Caster:     "caster.example.com:2101",
		Mountpoint: "/MOUNT01",
		Password:   "secret",
	})

	err := ntripClient.sourceHandshake(client)
	require.Error(t, err)
	require.Contains(t, err.Error(), "sending SOURCE request")
}

func TestPostHandshakeRequestFormat(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	cfg := Config{
		Caster:     "caster.example.com:2101",
		Mountpoint: "/MOUNT01",
		Password:   "secret",
		Username:   "user1",
		UserAgent:  "NTRIP MyApp/1.0",
		Version:    2,
		Chunked:    true,
	}
	ntripClient := NewClient(cfg)

	errCh := make(chan error, 1)
	go func() {
		errCh <- ntripClient.postHandshake(client)
	}()

	reader := bufio.NewReader(server)
	var requestLine string
	headers := map[string]string{}
	for {
		line, err := reader.ReadString('\n')
		require.NoError(t, err)
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if requestLine == "" {
			requestLine = line
			continue
		}
		k, v, _ := strings.Cut(line, ": ")
		headers[k] = v
	}

	require.Equal(t, "POST /MOUNT01 HTTP/1.1", requestLine)
	require.Equal(t, "caster.example.com:2101", headers["Host"])
	require.Equal(t, "Ntrip/2.0", headers["Ntrip-Version"])
	require.Equal(t, "NTRIP MyApp/1.0", headers["User-Agent"])
	require.Equal(t, "chunked", headers["Transfer-Encoding"])
	want := base64.StdEncoding.EncodeToString([]byte("user1:secret"))
	require.Equal(t, "Basic "+want, headers["Authorization"])
	require.NoError(t, <-errCh)
}

func TestClientWriteChunked(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	c := &Client{
		config: Config{Version: 2, Chunked: true},
		conn:   client,
		logger: slog.Default(),
	}

	data := []byte{0xD3, 0x00, 0x04, 0x3E, 0xD0, 0x00, 0x03} // 7 bytes
	go func() {
		n, err := c.Write(data)
		require.NoError(t, err)
		require.Equal(t, len(data), n)
	}()

	reader := bufio.NewReader(server)
	sizeLine, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "7\r\n", sizeLine) // chunk size in hex

	buf := make([]byte, len(data))
	_, err = io.ReadFull(reader, buf)
	require.NoError(t, err)
	require.Equal(t, data, buf)

	crlf := make([]byte, 2)
	_, err = io.ReadFull(reader, crlf)
	require.NoError(t, err)
	require.Equal(t, "\r\n", string(crlf))
}

func TestClientWriteNotConnected(t *testing.T) {
	c := NewClient(Config{Caster: "localhost:2101", Mountpoint: "/M", Password: "p"})
	_, err := c.Write([]byte("data"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not connected")
}

func TestClientWriteForwarding(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	c := &Client{
		config: Config{Caster: "localhost:2101", Mountpoint: "/M", Password: "p"},
		conn:   client,
		logger: slog.Default(),
	}

	data := []byte{0xD3, 0x00, 0x04, 0x3E, 0xD0, 0x00, 0x03}
	go func() {
		_, err := c.Write(data)
		require.NoError(t, err)
	}()

	buf := make([]byte, len(data))
	_, err := io.ReadFull(server, buf)
	require.NoError(t, err)
	require.Equal(t, data, buf)
}

func TestClientCloseNil(t *testing.T) {
	c := NewClient(Config{Caster: "localhost:2101", Mountpoint: "/M", Password: "p"})
	require.NoError(t, c.Close())
}

func TestClientClose(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	c := &Client{
		config: Config{Caster: "localhost:2101", Mountpoint: "/M", Password: "p"},
		conn:   client,
		logger: slog.Default(),
	}

	require.NoError(t, c.Close())
	require.Nil(t, c.conn)

	_, err := c.Write([]byte("data"))
	require.Error(t, err)
}

func TestClientConnectDialFailure(t *testing.T) {
	c := NewClient(Config{
		Caster:     "localhost:1",
		Mountpoint: "/M",
		Password:   "p",
	})

	ctx := context.Background()
	err := c.Connect(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "dialing caster")
}

func TestClientConnectFullHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	c := NewClient(Config{
		Caster:     listener.Addr().String(),
		Mountpoint: "/TEST",
		Password:   "testpass",
	})

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		// Read SOURCE request.
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if strings.TrimSpace(line) == "" {
				break
			}
		}
		// Send success response.
		_, _ = conn.Write([]byte("ICY 200 OK\r\n"))

		// Keep connection alive for the test.
		buf := make([]byte, 1)
		_, _ = conn.Read(buf)
	}()

	ctx := context.Background()
	err = c.Connect(ctx)
	require.NoError(t, err)
	defer c.Close()

	_, err = c.Write([]byte("test data"))
	require.NoError(t, err)
}

func TestProxyConfigInvalidCertPath(t *testing.T) {
	c := NewClient(Config{
		Caster:     "localhost:2101",
		Mountpoint: "/M",
		Password:   "p",
	}, WithProxy(ProxyConfig{
		Address:  "proxy:8082",
		CertFile: "/nonexistent/cert.pem",
		KeyFile:  "/nonexistent/key.pem",
	}))

	ctx := context.Background()
	err := c.Connect(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "loading proxy TLS certificate")
}

func TestWithLogger(t *testing.T) {
	customLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	c := NewClient(Config{
		Caster:     "localhost:2101",
		Mountpoint: "/M",
		Password:   "p",
	}, WithLogger(customLogger))
	require.Same(t, customLogger, c.logger)
}

func TestDefaultUserAgent(t *testing.T) {
	c := NewClient(Config{
		Caster:     "localhost:2101",
		Mountpoint: "/M",
		Password:   "p",
	})
	require.Equal(t, "NTRIP ntripper/1.0", c.config.UserAgent)
}
