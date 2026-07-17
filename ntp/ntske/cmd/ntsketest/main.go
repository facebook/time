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

// Command ntsketest is a smoke-test client for the NTS-KE server. It performs a
// single handshake via ntske.Client and prints a PASS line with the negotiated
// next-protocol, AEAD algorithm, and cookie count.
//
//	ntsketest --addr 127.0.0.1:4460 --ca /tmp/ntske_cert.pem --skip-ntp
//	[ke] PASS: next-proto=NTPv4 aead=30 cookies=8
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/facebook/time/ntp/ntske"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:4460", "NTS-KE server address (host:port)")
	caFile := flag.String("ca", "", "PEM file with the CA/self-signed cert to trust (empty = system roots)")
	skipNTP := flag.Bool("skip-ntp", false, "stop after the NTS-KE handshake and do not attempt the NTPv4 phase")
	timeout := flag.Duration("timeout", 10*time.Second, "overall timeout for the handshake")
	flag.Parse()

	tlsConf, err := ntske.ClientTLSConfig(*caFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ke] FAIL: %v\n", err)
		os.Exit(1)
	}

	client := &ntske.Client{RequestCompliantExport: true, Timeout: *timeout}
	res, err := client.Handshake(context.Background(), *addr, tlsConf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ke] FAIL: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[ke] PASS: next-proto=%s aead=%d cookies=%d\n",
		ntske.NextProtocolName(res.NextProtocol), res.AEAD, len(res.Cookies))
	if res.CompliantExport {
		fmt.Println("[ke]   compliant-128-GCM-SIV-export negotiated")
	}

	if !*skipNTP {
		fmt.Fprintln(os.Stderr, "[ke] note: NTPv4 phase not implemented in milestone 1; re-run with --skip-ntp")
		os.Exit(1)
	}
}
