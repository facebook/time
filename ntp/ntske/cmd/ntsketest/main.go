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

// Command ntsketest is a smoke-test client for the NTS server. It runs an NTS-KE
// handshake and, unless --skip-ntp is set, a full NTS-protected NTPv4 round-trip.
//
//	ntsketest --addr 127.0.0.1:4460 --ca /tmp/ntske_cert.pem
//	[ke] PASS: next-proto=NTPv4 aead=30 cookies=8
//	[ntp] PASS: fresh-cookies=1
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/facebook/time/ntp/ntske"
	"github.com/facebook/time/ntp/protocol"
	"github.com/facebook/time/ntp/protocol/nts"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:4460", "NTS-KE server address (host:port)")
	ntpAddr := flag.String("ntp-addr", "", "NTPv4 server address (host:port); defaults to the KE host on :123")
	caFile := flag.String("ca", "", "PEM file with the CA/self-signed cert to trust (empty = system roots)")
	skipNTP := flag.Bool("skip-ntp", false, "stop after the NTS-KE handshake and do not attempt the NTPv4 phase")
	timeout := flag.Duration("timeout", 10*time.Second, "overall timeout for the whole run (KE handshake + NTPv4)")
	flag.Parse()

	tlsConf, err := ntske.ClientTLSConfig(*caFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ke] FAIL: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	client := &ntske.Client{RequestCompliantExport: true, Timeout: *timeout}
	res, err := client.Handshake(ctx, *addr, tlsConf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ke] FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[ke] PASS: next-proto=%s aead=%d cookies=%d\n",
		ntske.NextProtocolName(res.NextProtocol), res.AEAD, len(res.Cookies))
	if res.CompliantExport {
		fmt.Println("[ke]   compliant-128-GCM-SIV-export negotiated")
	}
	if *skipNTP {
		return
	}

	if err := runNTPv4(ctx, *addr, *ntpAddr, res); err != nil {
		fmt.Fprintf(os.Stderr, "[ntp] FAIL: %v\n", err)
		os.Exit(1)
	}
}

// runNTPv4 sends one NTS-protected NTPv4 request using a cookie from the KE
// handshake and verifies the response authenticator under the S2C key.
func runNTPv4(ctx context.Context, keAddr, ntpAddr string, res *ntske.HandshakeResult) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("no time left after KE handshake: %w", err)
	}
	if ntpAddr == "" {
		host, _, err := net.SplitHostPort(keAddr)
		if err != nil {
			return fmt.Errorf("deriving ntp host from %q: %w", keAddr, err)
		}
		ntpAddr = net.JoinHostPort(host, "123")
	}

	if len(res.Cookies) == 0 {
		return errors.New("KE handshake returned no cookies")
	}

	uid := make([]byte, nts.MinUniqueIdentifierLen)
	if _, err := rand.Read(uid); err != nil {
		return fmt.Errorf("generating unique identifier: %w", err)
	}
	reqBytes, err := nts.BuildNTSRequest(protocol.Packet{Settings: 0x23}, nts.RequestParams{
		AEAD:     protocol.AEADAlgorithm(res.AEAD),
		C2S:      res.C2S,
		Cookie:   res.Cookies[0],
		UniqueID: uid,
	})
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}

	conn, err := net.Dial("udp", ntpAddr)
	if err != nil {
		return fmt.Errorf("dial %q: %w", ntpAddr, err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	if _, err := conn.Write(reqBytes); err != nil {
		return fmt.Errorf("send: %w", err)
	}
	buf := make([]byte, 1500)
	n, err := conn.Read(buf)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	_, fresh, err := nts.VerifyNTSResponse(buf[:n], protocol.AEADAlgorithm(res.AEAD), res.S2C, uid)
	if err != nil {
		return err
	}
	fmt.Printf("[ntp] PASS: fresh-cookies=%d\n", len(fresh))
	return nil
}
