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
	"fmt"
	"io"
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
		UserAgent:  "TestAgent/1.0",
	})

	// Run handshake in a goroutine.
	errCh := make(chan error, 1)
	go func() {
		errCh <- ntripClient.sourceHandshake(client)
	}()

	// Verify the SOURCE request from the client.
	reader := bufio.NewReader(server)
	line, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "SOURCE secret /MOUNT01\r\n", line)

	line, err = reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "Source-Agent: TestAgent/1.0\r\n", line)

	line, err = reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "\r\n", line)

	// Send successful response.
	_, err = server.Write([]byte("ICY 200 OK\r\n\r\n"))
	require.NoError(t, err)

	require.NoError(t, <-errCh)
}

func TestSourceHandshakeRejected(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()

	ntripClient := NewClient(Config{
		Caster:     "caster.example.com:2101",
		Mountpoint: "/MOUNT01",
		Password:   "wrong",
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- ntripClient.sourceHandshake(client)
	}()

	// Consume the SOURCE request.
	reader := bufio.NewReader(server)
	for {
		line, err := reader.ReadString('\n')
		require.NoError(t, err)
		if strings.TrimSpace(line) == "" {
			break
		}
	}

	// Send rejection.
	_, err := server.Write([]byte("ERROR - Bad Password\r\n\r\n"))
	require.NoError(t, err)

	err = <-errCh
	require.Error(t, err)
	require.Contains(t, err.Error(), "caster rejected connection")
	require.Contains(t, err.Error(), "Bad Password")
}

func TestSourceHandshakeConnectionClosed(t *testing.T) {
	client, server := net.Pipe()

	ntripClient := NewClient(Config{
		Caster:     "caster.example.com:2101",
		Mountpoint: "/MOUNT01",
		Password:   "secret",
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- ntripClient.sourceHandshake(client)
	}()

	// Consume the SOURCE request then close.
	reader := bufio.NewReader(server)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		if strings.TrimSpace(line) == "" {
			break
		}
	}
	server.Close()

	err := <-errCh
	require.Error(t, err)
	require.Contains(t, err.Error(), "reading caster response")
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
	}

	require.NoError(t, c.Close())
	require.Nil(t, c.conn)

	// Writing after close should fail.
	_, err := c.Write([]byte("data"))
	require.Error(t, err)
}

func TestClientConnectDialFailure(t *testing.T) {
	c := NewClient(Config{
		Caster:     "localhost:1", // Unlikely to be listening
		Mountpoint: "/M",
		Password:   "p",
	})

	ctx := context.Background()
	err := c.Connect(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "dialing caster")
}

func TestClientConnectFullHandshake(t *testing.T) {
	// Create a TCP listener to simulate a caster.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	c := NewClient(Config{
		Caster:     listener.Addr().String(),
		Mountpoint: "/TEST",
		Password:   "testpass",
	})

	// Accept and respond in a goroutine.
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
		_, _ = conn.Write([]byte("ICY 200 OK\r\n\r\n"))

		// Keep connection alive for the test.
		buf := make([]byte, 1)
		_, _ = conn.Read(buf)
	}()

	ctx := context.Background()
	err = c.Connect(ctx)
	require.NoError(t, err)
	defer c.Close()

	// Verify we can write data.
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

func TestSourceHandshakeMultipleResponseHeaders(t *testing.T) {
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

	// Consume SOURCE request.
	reader := bufio.NewReader(server)
	for {
		line, err := reader.ReadString('\n')
		require.NoError(t, err)
		if strings.TrimSpace(line) == "" {
			break
		}
	}

	// Send response with extra headers (some casters do this).
	response := "ICY 200 OK\r\nServer: NTRIP Caster 2.0\r\nDate: Wed, 15 Apr 2026\r\n\r\n"
	_, err := server.Write([]byte(response))
	require.NoError(t, err)

	require.NoError(t, <-errCh)
}

func TestSourceHandshakeRequestFormat(t *testing.T) {
	// Verify the exact wire format of the SOURCE request.
	client, server := net.Pipe()
	defer server.Close()

	cfg := Config{
		Caster:     "caster.example.com:2101",
		Mountpoint: "/EXAMPLE_MOUNT",
		Password:   "my$ecr3t",
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

	expected := fmt.Sprintf(
		"SOURCE %s %s\r\nSource-Agent: %s\r\n\r\n",
		cfg.Password, cfg.Mountpoint, cfg.UserAgent,
	)
	require.Equal(t, expected, request.String())

	_, _ = server.Write([]byte("ICY 200 OK\r\n\r\n"))
	require.NoError(t, <-errCh)
}
