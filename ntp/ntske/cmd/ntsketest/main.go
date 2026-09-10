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

// Command ntsketest tests an NTS server. By default it runs one NTS-KE
// handshake and, unless --skip-ntp is set, one NTS-protected NTPv4 round-trip.
// The --requests and --concurrency flags enable bounded load testing.
//
//	ntsketest --addr 127.0.0.1:4460 --ca /tmp/ntske_cert.pem
//	[ke] PASS: next-proto=NTPv4 aead=30 cookies=8
//	[ntp] PASS: fresh-cookies=1
package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/facebook/time/ntp/ntske"
	"github.com/facebook/time/ntp/protocol"
	"github.com/facebook/time/ntp/protocol/nts"
)

var errLoadDurationExpired = errors.New("NTS load duration expired")

type testConfig struct {
	addr              string
	ntpAddr           string
	skipNTP           bool
	freshKEPerRequest bool
	timeout           time.Duration
	duration          time.Duration
	requests          int
	concurrency       int
	tlsConfig         *tls.Config
	printResult       bool
}

type ntsClient interface {
	handshake(context.Context, testConfig) (*ntske.HandshakeResult, error)
	exchange(context.Context, testConfig, string, *ntske.HandshakeResult, []byte) ([][]byte, error)
}

type realNTSClient struct{}

type workerSession struct {
	handshake *ntske.HandshakeResult
	ntpAddr   string
	cookies   [][]byte
}

func main() {
	cfg := testConfig{
		addr:        "127.0.0.1:4460",
		timeout:     10 * time.Second,
		requests:    1,
		concurrency: 1,
	}
	var caFile string
	flag.StringVar(&cfg.addr, "addr", cfg.addr, "NTS-KE server address (host:port)")
	flag.StringVar(&cfg.ntpAddr, "ntp-addr", "", "NTPv4 server address (host:port); defaults to the KE host on :123")
	flag.StringVar(&caFile, "ca", "", "PEM file with the CA/self-signed cert to trust (empty = system roots)")
	flag.BoolVar(&cfg.skipNTP, "skip-ntp", false, "stop after the NTS-KE handshake and do not attempt the NTPv4 phase")
	flag.BoolVar(
		&cfg.freshKEPerRequest,
		"fresh-ke-per-request",
		false,
		"perform a new NTS-KE handshake for every authenticated NTP request",
	)
	flag.DurationVar(&cfg.timeout, "timeout", cfg.timeout, "timeout for each KE handshake and NTPv4 exchange")
	flag.DurationVar(&cfg.duration, "duration", 0, "overall load deadline (zero = no overall deadline)")
	flag.IntVar(&cfg.requests, "requests", cfg.requests, "number of complete NTS exchanges to run")
	flag.IntVar(&cfg.concurrency, "concurrency", cfg.concurrency, "maximum number of concurrent NTS exchanges")
	flag.Parse()

	if cfg.requests <= 0 {
		slog.Error("invalid request count", "requests", cfg.requests)
		os.Exit(1)
	}
	if cfg.concurrency <= 0 {
		slog.Error("invalid concurrency", "concurrency", cfg.concurrency)
		os.Exit(1)
	}
	if cfg.duration < 0 {
		slog.Error("invalid duration", "duration", cfg.duration)
		os.Exit(1)
	}

	logLevel := slog.LevelInfo
	if cfg.requests == 1 {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))

	tlsConf, err := ntske.ClientTLSConfig(caFile)
	if err != nil {
		slog.Error("[ke] FAIL", "err", err)
		os.Exit(1)
	}
	cfg.tlsConfig = tlsConf
	cfg.printResult = cfg.requests == 1

	startedAt := time.Now()
	completed, err := runLoad(context.Background(), cfg, realNTSClient{})
	if err != nil {
		slog.Error("NTS load FAIL", "completed", completed, "requests", cfg.requests, "err", err)
		os.Exit(1)
	}
	if cfg.requests > 1 {
		elapsed := time.Since(startedAt)
		fmt.Printf("NTS load PASS: verified=%d workers=%d elapsed=%s rate=%.2f/s\n",
			completed,
			min(cfg.concurrency, cfg.requests),
			elapsed.Round(time.Millisecond),
			float64(completed)/elapsed.Seconds(),
		)
	}
}

func runLoad(parent context.Context, cfg testConfig, client ntsClient) (int, error) {
	if cfg.requests <= 0 {
		return 0, fmt.Errorf("requests must be positive: %d", cfg.requests)
	}
	if cfg.concurrency <= 0 {
		return 0, fmt.Errorf("concurrency must be positive: %d", cfg.concurrency)
	}

	deadlineCtx := parent
	cancelDeadline := func() {}
	if cfg.duration > 0 {
		deadlineCtx, cancelDeadline = context.WithTimeoutCause(parent, cfg.duration, errLoadDurationExpired)
	}
	defer cancelDeadline()
	ctx, cancel := context.WithCancel(deadlineCtx)
	defer cancel()

	workers := min(cfg.concurrency, cfg.requests)
	errs := make(chan error, 1)
	var nextRequest atomic.Int64
	var completed atomic.Int64
	var wg sync.WaitGroup

	reportError := func(err error) {
		select {
		case errs <- err:
			cancel()
		default:
		}
	}

	for workerID := range workers {
		wg.Go(func() {
			err := runWorker(ctx, cfg, client, &nextRequest, &completed)
			if err != nil && !isOverallDeadlineCancellation(ctx, err) {
				reportError(fmt.Errorf("worker %d: %w", workerID, err))
			}
		})
	}
	wg.Wait()

	count := int(completed.Load())
	if count == cfg.requests {
		return count, nil
	}
	select {
	case err := <-errs:
		return count, err
	default:
	}
	if errors.Is(context.Cause(ctx), errLoadDurationExpired) {
		return count, fmt.Errorf(
			"completed %d of %d exchanges before duration %s expired",
			count,
			cfg.requests,
			cfg.duration,
		)
	}
	return count, fmt.Errorf("completed %d of %d exchanges", count, cfg.requests)
}

func runWorker(
	ctx context.Context,
	cfg testConfig,
	client ntsClient,
	nextRequest, completed *atomic.Int64,
) error {
	if cfg.skipNTP || cfg.freshKEPerRequest {
		for {
			requestID, ok := claimRequest(ctx, cfg.requests, nextRequest)
			if !ok {
				return nil
			}
			if err := runFreshExchange(ctx, cfg, client); err != nil {
				return fmt.Errorf("request %d: %w", requestID, err)
			}
			completed.Add(1)
		}
	}

	session, err := initializeSession(ctx, cfg, client)
	if err != nil {
		return fmt.Errorf("initialization: %w", err)
	}
	for {
		requestID, ok := claimRequest(ctx, cfg.requests, nextRequest)
		if !ok {
			return nil
		}
		if err := runSessionExchange(ctx, cfg, client, session); err != nil {
			return fmt.Errorf("request %d: %w", requestID, err)
		}
		completed.Add(1)
	}
}

func claimRequest(ctx context.Context, requests int, nextRequest *atomic.Int64) (int, bool) {
	if ctx.Err() != nil {
		return 0, false
	}
	requestID := int(nextRequest.Add(1) - 1)
	return requestID, requestID < requests
}

func isOverallDeadlineCancellation(ctx context.Context, err error) bool {
	return errors.Is(context.Cause(ctx), errLoadDurationExpired) &&
		(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
}

func runSessionExchange(ctx context.Context, cfg testConfig, client ntsClient, session *workerSession) error {
	if len(session.cookies) == 0 {
		newSession, err := initializeSession(ctx, cfg, client)
		if err != nil {
			return err
		}
		*session = *newSession
	}

	cookie := session.cookies[0]
	session.cookies = session.cookies[1:]
	freshCookies, err := performNTPExchange(ctx, cfg, client, session.ntpAddr, session.handshake, cookie)
	if err != nil {
		return fmt.Errorf("NTPv4 exchange: %w", err)
	}
	session.cookies = append(session.cookies, freshCookies...)
	return nil
}

func runFreshExchange(ctx context.Context, cfg testConfig, client ntsClient) error {
	res, err := performHandshake(ctx, cfg, client)
	if err != nil {
		return fmt.Errorf("KE handshake: %w", err)
	}
	printHandshakeResult(cfg, res)
	if cfg.skipNTP {
		return nil
	}
	if len(res.Cookies) == 0 {
		return errors.New("KE handshake returned no cookies")
	}
	ntpAddr, err := resolveNTPAddr(cfg.addr, cfg.ntpAddr, res)
	if err != nil {
		return err
	}
	freshCookies, err := performNTPExchange(ctx, cfg, client, ntpAddr, res, res.Cookies[0])
	if err != nil {
		return fmt.Errorf("NTPv4 exchange: %w", err)
	}
	if cfg.printResult {
		fmt.Printf("[ntp] PASS: fresh-cookies=%d\n", len(freshCookies))
	}
	return nil
}

func initializeSession(ctx context.Context, cfg testConfig, client ntsClient) (*workerSession, error) {
	res, err := performHandshake(ctx, cfg, client)
	if err != nil {
		return nil, fmt.Errorf("KE handshake: %w", err)
	}
	printHandshakeResult(cfg, res)
	if len(res.Cookies) == 0 {
		return nil, errors.New("KE handshake returned no cookies")
	}
	ntpAddr, err := resolveNTPAddr(cfg.addr, cfg.ntpAddr, res)
	if err != nil {
		return nil, err
	}
	return &workerSession{
		handshake: res,
		ntpAddr:   ntpAddr,
		cookies:   res.Cookies,
	}, nil
}

func performHandshake(ctx context.Context, cfg testConfig, client ntsClient) (*ntske.HandshakeResult, error) {
	operationCtx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()
	return client.handshake(operationCtx, cfg)
}

func performNTPExchange(
	ctx context.Context,
	cfg testConfig,
	client ntsClient,
	ntpAddr string,
	res *ntske.HandshakeResult,
	cookie []byte,
) ([][]byte, error) {
	operationCtx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()
	return client.exchange(operationCtx, cfg, ntpAddr, res, cookie)
}

func (realNTSClient) handshake(ctx context.Context, cfg testConfig) (*ntske.HandshakeResult, error) {
	client := &ntske.Client{RequestCompliantExport: true, Timeout: cfg.timeout}
	return client.Handshake(ctx, cfg.addr, cfg.tlsConfig.Clone())
}

func printHandshakeResult(cfg testConfig, res *ntske.HandshakeResult) {
	if !cfg.printResult {
		return
	}
	fmt.Printf("[ke] PASS: next-proto=%s aead=%d cookies=%d\n",
		ntske.NextProtocolName(res.NextProtocol), res.AEAD, len(res.Cookies))
	if res.CompliantExport {
		fmt.Println("[ke]   compliant-128-GCM-SIV-export negotiated")
	}
}

func resolveNTPAddr(keAddr, ntpAddr string, res *ntske.HandshakeResult) (string, error) {
	if ntpAddr == "" && res.NTPServer != "" {
		port := "123"
		if res.NTPPort != 0 {
			port = strconv.Itoa(int(res.NTPPort))
		}
		ntpAddr = net.JoinHostPort(res.NTPServer, port)
	}
	if ntpAddr == "" {
		host, _, err := net.SplitHostPort(keAddr)
		if err != nil {
			return "", fmt.Errorf("deriving ntp host from %q: %w", keAddr, err)
		}
		ntpAddr = net.JoinHostPort(host, "123")
	}
	return ntpAddr, nil
}

// exchange sends one NTS-protected NTPv4 request using the selected cookie and
// returns fresh cookies only after verifying the response under the S2C key.
func (realNTSClient) exchange(
	ctx context.Context,
	cfg testConfig,
	ntpAddr string,
	res *ntske.HandshakeResult,
	cookie []byte,
) ([][]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("no time left before NTP exchange: %w", err)
	}
	slog.Debug("[ntp] target resolved",
		"ke", cfg.addr, "ntp", ntpAddr, "ke_advertised_server", res.NTPServer, "ke_advertised_port", res.NTPPort)

	uid := make([]byte, nts.MinUniqueIdentifierLen)
	if _, err := rand.Read(uid); err != nil {
		return nil, fmt.Errorf("generating unique identifier: %w", err)
	}
	reqBytes, err := nts.BuildNTSRequest(protocol.Packet{Settings: 0x23}, nts.RequestParams{
		AEAD:     protocol.AEADAlgorithm(res.AEAD),
		C2S:      res.C2S,
		Cookie:   cookie,
		UniqueID: uid,
	})
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	slog.Debug("[ntp] request built", "bytes", len(reqBytes), "aead", res.AEAD, "cookie_len", len(cookie))

	conn, err := net.Dial("udp", ntpAddr)
	if err != nil {
		return nil, fmt.Errorf("dial %q: %w", ntpAddr, err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	slog.Debug("[ntp] dialed", "local", conn.LocalAddr(), "remote", conn.RemoteAddr())

	if _, err := conn.Write(reqBytes); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("send: %w", err)
	}
	slog.Debug("[ntp] request sent", "bytes", len(reqBytes))
	buf := make([]byte, 1500)
	n, err := conn.Read(buf)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("read: %w", err)
	}
	slog.Debug("[ntp] response received", "bytes", n)

	_, freshCookies, err := nts.VerifyNTSResponse(buf[:n], protocol.AEADAlgorithm(res.AEAD), res.S2C, uid)
	if err != nil {
		return nil, err
	}
	return freshCookies, nil
}
