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
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/facebook/time/ntp/protocol"
	"github.com/stretchr/testify/require"
)

// clientTestCert returns a self-signed Ed25519 cert/key (PEM) valid for
// 127.0.0.1, so the real client can verify it against a CA file.
func clientTestCert(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Unix(0, 0),
		NotAfter:     time.Unix(1<<31-1, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	require.NoError(t, err)
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}

// clientFreePort binds an ephemeral port, closes it, and returns the address so
// a server can listen on it.
func clientFreePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

// startTestKEServer spins up an in-process Server on an ephemeral port and
// returns its address plus the CA PEM the client should trust.
func startTestKEServer(t *testing.T, cookies uint16) (addr string, caPEM []byte) {
	t.Helper()
	certPEM, keyPEM := clientTestCert(t)
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)
	ks, err := NewInMemoryKeystore(InMemoryKeystoreOptions{})
	require.NoError(t, err)
	srv := &Server{
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
		Keystore:  ks,
		Cookies:   cookies,
	}
	addr = clientFreePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = srv.ListenAndServe(ctx, addr) }()

	// wait until the listener accepts before returning
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c, derr := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if derr == nil {
			_ = c.Close()
			return addr, certPEM
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server at %s never became ready", addr)
	return "", nil
}

func TestClientHandshake(t *testing.T) {
	addr, caPEM := startTestKEServer(t, 8)
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caFile, caPEM, 0o600))

	tlsConf, err := ClientTLSConfig(caFile)
	require.NoError(t, err)

	res, err := (&Client{}).Handshake(context.Background(), addr, tlsConf)
	require.NoError(t, err)
	require.Equal(t, NextProtocolNTPv4, res.NextProtocol)
	require.Equal(t, uint16(protocol.AEADAES128GCMSIV), res.AEAD)
	require.Len(t, res.Cookies, 8)
}

func TestClientHandshakeUntrustedCert(t *testing.T) {
	addr, _ := startTestKEServer(t, 8)
	otherPEM, _ := clientTestCert(t) // a CA that does not contain the server's cert
	caFile := filepath.Join(t.TempDir(), "other.pem")
	require.NoError(t, os.WriteFile(caFile, otherPEM, 0o600))

	tlsConf, err := ClientTLSConfig(caFile)
	require.NoError(t, err)
	_, err = (&Client{}).Handshake(context.Background(), addr, tlsConf)
	require.Error(t, err)
}

func TestClientTLSConfig(t *testing.T) {
	t.Run("no CA uses system roots", func(t *testing.T) {
		conf, err := ClientTLSConfig("")
		require.NoError(t, err)
		require.Nil(t, conf.RootCAs)
		require.Equal(t, uint16(tls.VersionTLS13), conf.MinVersion)
		require.Equal(t, []string{ALPNProtocol}, conf.NextProtos)
	})
	t.Run("valid CA", func(t *testing.T) {
		certPEM, _ := clientTestCert(t)
		caFile := filepath.Join(t.TempDir(), "ca.pem")
		require.NoError(t, os.WriteFile(caFile, certPEM, 0o600))
		conf, err := ClientTLSConfig(caFile)
		require.NoError(t, err)
		require.NotNil(t, conf.RootCAs)
	})
	t.Run("missing file", func(t *testing.T) {
		_, err := ClientTLSConfig(filepath.Join(t.TempDir(), "nope.pem"))
		require.Error(t, err)
	})
	t.Run("garbage PEM", func(t *testing.T) {
		caFile := filepath.Join(t.TempDir(), "junk.pem")
		require.NoError(t, os.WriteFile(caFile, []byte("not a pem"), 0o600))
		_, err := ClientTLSConfig(caFile)
		require.Error(t, err)
	})
}

func TestClientInterpret(t *testing.T) {
	cookie := func(n byte) Record { return NewCookie([]byte{n, n, n, n}) }
	c := &Client{} // default offers [30, 17]

	t.Run("valid", func(t *testing.T) {
		res, err := c.interpret([]Record{
			NewNextProtocol(NextProtocolNTPv4),
			NewAEADAlgorithm(uint16(protocol.AEADAES128GCMSIV)),
			cookie(1), cookie(2),
		})
		require.NoError(t, err)
		require.Equal(t, uint16(protocol.AEADAES128GCMSIV), res.AEAD)
		require.Len(t, res.Cookies, 2)
		require.False(t, res.CompliantExport)
	})
	t.Run("compliant export echoed", func(t *testing.T) {
		res, err := c.interpret([]Record{
			NewNextProtocol(NextProtocolNTPv4),
			NewAEADAlgorithm(uint16(protocol.AEADAES128GCMSIV)),
			{Type: RecordCompliant128GCMExport},
			cookie(1),
		})
		require.NoError(t, err)
		require.True(t, res.CompliantExport)
	})
	t.Run("server error record", func(t *testing.T) {
		_, err := c.interpret([]Record{NewError(1)})
		require.Error(t, err)
	})
	t.Run("missing next-proto", func(t *testing.T) {
		_, err := c.interpret([]Record{
			NewAEADAlgorithm(uint16(protocol.AEADAES128GCMSIV)), cookie(1),
		})
		require.Error(t, err)
	})
	t.Run("wrong next-proto", func(t *testing.T) {
		_, err := c.interpret([]Record{
			NewNextProtocol(7),
			NewAEADAlgorithm(uint16(protocol.AEADAES128GCMSIV)), cookie(1),
		})
		require.Error(t, err)
	})
	t.Run("unoffered AEAD rejected", func(t *testing.T) {
		_, err := c.interpret([]Record{
			NewNextProtocol(NextProtocolNTPv4),
			NewAEADAlgorithm(9999), // not in [30, 17]
			cookie(1),
		})
		require.Error(t, err)
	})
	t.Run("no cookies", func(t *testing.T) {
		_, err := c.interpret([]Record{
			NewNextProtocol(NextProtocolNTPv4),
			NewAEADAlgorithm(uint16(protocol.AEADAES128GCMSIV)),
		})
		require.Error(t, err)
	})
}
