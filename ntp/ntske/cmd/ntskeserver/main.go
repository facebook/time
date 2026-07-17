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

// Command ntskeserver is a standalone NTS-KE server for local interop testing.
//
//	ntskeserver --addr 127.0.0.1:4460 --cert /tmp/ntske_cert.pem --key /tmp/ntske_key.pem
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"syscall"

	"github.com/facebook/time/ntp/ntske"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:4460", "address to listen on (host:port)")
	certFile := flag.String("cert", "/tmp/ntske_cert.pem", "TLS certificate PEM")
	keyFile := flag.String("key", "/tmp/ntske_key.pem", "TLS private key PEM")
	cookies := flag.Uint("cookies", 8, "number of cookies to issue per handshake")
	flag.Parse()
	if *cookies > math.MaxUint16 {
		slog.Error("invalid --cookies value exceeds uint16 range", "value", *cookies, "max", math.MaxUint16)
		os.Exit(1)
	}

	cert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
	if err != nil {
		slog.Error("load cert/key", "err", err)
		os.Exit(1)
	}

	keystore, err := ntske.NewInMemoryKeystore(ntske.InMemoryKeystoreOptions{})
	if err != nil {
		slog.Error("keystore", "err", err)
		os.Exit(1)
	}

	srv := &ntske.Server{
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
		Keystore:  keystore,
		Cookies:   uint16(*cookies), //nolint:gosec // bounded: math.MaxUint16 (65535) guard above exits
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("NTS-KE server listening", "addr", *addr, "cookies", *cookies)
	if err := srv.ListenAndServe(ctx, *addr); err != nil {
		slog.Error("serve", "err", err)
		os.Exit(1)
	}
}
