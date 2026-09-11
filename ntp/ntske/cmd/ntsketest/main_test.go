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

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/facebook/time/ntp/ntske"
	"github.com/stretchr/testify/require"
)

type fakeNTSClient struct {
	handshakeCookies [][]byte
	freshCookies     map[string][][]byte
	exchangeErr      error
	blockAfter       int64
	blockErr         error
	cancelAfter      int64
	cancel           context.CancelFunc
	handshakeCalls   atomic.Int64
	exchangeCalls    atomic.Int64
	sessionCalls     atomic.Int64
	mu               sync.Mutex
	consumedCookies  [][]byte
	sessions         []*fakeExchangeSession
	events           []string
}

type fakeExchangeSession struct {
	client        *fakeNTSClient
	exchangeCalls atomic.Int64
	closeCalls    atomic.Int64
	mu            sync.Mutex
	deadlines     []time.Time
}

func (f *fakeNTSClient) handshake(context.Context, testConfig) (*ntske.HandshakeResult, error) {
	f.handshakeCalls.Add(1)
	f.recordEvent("handshake")
	return &ntske.HandshakeResult{Cookies: f.handshakeCookies}, nil
}

func (f *fakeNTSClient) newExchangeSession(context.Context, testConfig, string) (exchangeSession, error) {
	f.sessionCalls.Add(1)
	session := &fakeExchangeSession{client: f}
	f.mu.Lock()
	f.sessions = append(f.sessions, session)
	f.events = append(f.events, "open")
	f.mu.Unlock()
	return session, nil
}

func (s *fakeExchangeSession) exchange(
	ctx context.Context,
	deadline time.Time,
	_ *ntske.HandshakeResult,
	cookie []byte,
) ([][]byte, error) {
	s.mu.Lock()
	s.deadlines = append(s.deadlines, deadline)
	s.mu.Unlock()
	s.exchangeCalls.Add(1)

	f := s.client
	call := f.exchangeCalls.Add(1)
	if f.cancelAfter > 0 && call == f.cancelAfter {
		f.cancel()
	}
	if f.blockAfter > 0 && call > f.blockAfter {
		<-ctx.Done()
		if f.blockErr != nil {
			return nil, f.blockErr
		}
		return nil, ctx.Err()
	}
	if f.exchangeErr != nil {
		return nil, f.exchangeErr
	}
	f.mu.Lock()
	f.consumedCookies = append(f.consumedCookies, cookie)
	f.mu.Unlock()
	return f.freshCookies[string(cookie)], nil
}

func (s *fakeExchangeSession) close() {
	s.closeCalls.Add(1)
	s.client.recordEvent("close")
}

func (f *fakeNTSClient) recordEvent(event string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, event)
}

func (f *fakeNTSClient) sessionSnapshot() []*fakeExchangeSession {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*fakeExchangeSession(nil), f.sessions...)
}

func requireAllSessionsClosed(t *testing.T, client *fakeNTSClient) {
	t.Helper()
	sessions := client.sessionSnapshot()
	require.Len(t, sessions, int(client.sessionCalls.Load()))
	for _, session := range sessions {
		require.Equal(t, int64(1), session.closeCalls.Load())
	}
}

func TestRunLoadRejectsInvalidArguments(t *testing.T) {
	client := &fakeNTSClient{}

	completed, err := runLoad(t.Context(), testConfig{requests: 0, concurrency: 1}, client)
	require.EqualError(t, err, "requests must be positive: 0")
	require.Zero(t, completed)

	completed, err = runLoad(t.Context(), testConfig{requests: 1, concurrency: 0}, client)
	require.EqualError(t, err, "concurrency must be positive: 0")
	require.Zero(t, completed)
}

func TestRunLoadCancelsOnFirstError(t *testing.T) {
	sentinel := errors.New("exchange failed")
	client := &fakeNTSClient{
		handshakeCookies: [][]byte{[]byte("cookie")},
		exchangeErr:      sentinel,
	}
	cfg := operationConfig(10000, 8)

	completed, err := runLoad(t.Context(), cfg, client)
	require.ErrorIs(t, err, sentinel)
	require.Zero(t, completed)
	require.LessOrEqual(t, client.exchangeCalls.Load(), int64(8))
	requireAllSessionsClosed(t, client)
}

func TestRunLoadReportsShortCount(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	client := &fakeNTSClient{
		handshakeCookies: [][]byte{[]byte("cookie")},
		freshCookies:     map[string][][]byte{"cookie": {[]byte("cookie")}},
		cancelAfter:      1,
		cancel:           cancel,
	}

	completed, err := runLoad(ctx, operationConfig(10, 1), client)
	require.EqualError(t, err, "completed 1 of 10 exchanges")
	require.Equal(t, 1, completed)
	requireAllSessionsClosed(t, client)
}

func TestReusableLoadInitializesOneSessionPerWorker(t *testing.T) {
	client := &fakeNTSClient{
		handshakeCookies: [][]byte{[]byte("cookie")},
		freshCookies:     map[string][][]byte{"cookie": {[]byte("cookie")}},
	}

	completed, err := runLoad(t.Context(), operationConfig(9, 3), client)
	require.NoError(t, err)
	require.Equal(t, 9, completed)
	require.Equal(t, int64(3), client.handshakeCalls.Load())
	require.Equal(t, int64(3), client.sessionCalls.Load())
	requireAllSessionsClosed(t, client)
}

func TestReusableLoadReusesExchangeSession(t *testing.T) {
	client := &fakeNTSClient{
		handshakeCookies: [][]byte{[]byte("cookie")},
		freshCookies:     map[string][][]byte{"cookie": {[]byte("cookie")}},
	}

	completed, err := runLoad(t.Context(), operationConfig(3, 1), client)
	require.NoError(t, err)
	require.Equal(t, 3, completed)
	require.Equal(t, int64(1), client.sessionCalls.Load())
	sessions := client.sessionSnapshot()
	require.Equal(t, int64(3), sessions[0].exchangeCalls.Load())
	requireAllSessionsClosed(t, client)
}

func TestReusableLoadConsumesFreshCookiesWithoutReusingSpentCookies(t *testing.T) {
	client := &fakeNTSClient{
		handshakeCookies: [][]byte{[]byte("first"), []byte("second")},
		freshCookies: map[string][][]byte{
			"first": {[]byte("fresh-first")},
		},
	}

	completed, err := runLoad(t.Context(), operationConfig(3, 1), client)
	require.NoError(t, err)
	require.Equal(t, 3, completed)
	require.Equal(t, [][]byte{[]byte("first"), []byte("second"), []byte("fresh-first")}, client.consumedCookies)
}

func TestReusableLoadRenegotiatesOnlyWhenCookiePoolIsEmpty(t *testing.T) {
	client := &fakeNTSClient{
		handshakeCookies: [][]byte{[]byte("first"), []byte("second")},
	}

	completed, err := runLoad(t.Context(), operationConfig(3, 1), client)
	require.NoError(t, err)
	require.Equal(t, 3, completed)
	require.Equal(t, int64(2), client.handshakeCalls.Load())
	require.Equal(t, int64(2), client.sessionCalls.Load())
	requireAllSessionsClosed(t, client)
	client.mu.Lock()
	events := append([]string(nil), client.events...)
	client.mu.Unlock()
	require.Equal(t, []string{"handshake", "open", "close", "handshake", "open", "close"}, events)
}

func TestFreshKEPerRequestOpensAndClosesSessionForEveryRequest(t *testing.T) {
	client := &fakeNTSClient{handshakeCookies: [][]byte{[]byte("cookie")}}
	cfg := operationConfig(5, 2)
	cfg.freshKEPerRequest = true

	completed, err := runLoad(t.Context(), cfg, client)
	require.NoError(t, err)
	require.Equal(t, 5, completed)
	require.Equal(t, int64(5), client.handshakeCalls.Load())
	require.Equal(t, int64(5), client.sessionCalls.Load())
	requireAllSessionsClosed(t, client)
}

func TestSkipNTPPerformsFreshHandshakeWithoutOpeningSessions(t *testing.T) {
	client := &fakeNTSClient{}
	cfg := operationConfig(5, 2)
	cfg.skipNTP = true

	completed, err := runLoad(t.Context(), cfg, client)
	require.NoError(t, err)
	require.Equal(t, 5, completed)
	require.Equal(t, int64(5), client.handshakeCalls.Load())
	require.Zero(t, client.sessionCalls.Load())
	require.Zero(t, client.exchangeCalls.Load())
}

func TestRunLoadDurationReportsExactVerifiedCount(t *testing.T) {
	client := &fakeNTSClient{
		handshakeCookies: [][]byte{[]byte("cookie")},
		freshCookies:     map[string][][]byte{"cookie": {[]byte("cookie")}},
		blockAfter:       2,
	}
	cfg := operationConfig(10, 1)
	cfg.duration = 20 * time.Millisecond

	completed, err := runLoad(t.Context(), cfg, client)
	require.EqualError(t, err, "completed 2 of 10 exchanges before duration 20ms expired")
	require.Equal(t, 2, completed)
	requireAllSessionsClosed(t, client)
}

func TestIsOverallDeadlineCancellation(t *testing.T) {
	socketTimeout := socketDeadlineError()
	closed := time.Now().Add(-time.Second)
	open := time.Now().Add(time.Hour)

	require.True(t, isOverallDeadlineCancellation(closed, socketTimeout))
	require.True(t, isOverallDeadlineCancellation(closed, context.DeadlineExceeded))
	require.True(t, isOverallDeadlineCancellation(closed, context.Canceled))
	require.False(t, isOverallDeadlineCancellation(open, socketTimeout))
	require.False(t, isOverallDeadlineCancellation(time.Time{}, socketTimeout))
	require.False(t, isOverallDeadlineCancellation(closed, errors.New("authenticator verification failed")))
}

func TestRunLoadDurationReportsExpiryWhenSocketDeadlineWinsRace(t *testing.T) {
	client := &fakeNTSClient{
		handshakeCookies: [][]byte{[]byte("cookie")},
		freshCookies:     map[string][][]byte{"cookie": {[]byte("cookie")}},
		blockAfter:       2,
		blockErr:         socketDeadlineError(),
	}
	cfg := operationConfig(10, 1)
	cfg.duration = 20 * time.Millisecond

	completed, err := runLoad(t.Context(), cfg, client)
	require.EqualError(t, err, "completed 2 of 10 exchanges before duration 20ms expired")
	require.Equal(t, 2, completed)
}

func TestRunLoadParentDeadlineIsNotReportedAsLoadExpiry(t *testing.T) {
	parent, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	client := &fakeNTSClient{
		handshakeCookies: [][]byte{[]byte("cookie")},
		freshCookies:     map[string][][]byte{"cookie": {[]byte("cookie")}},
		blockAfter:       2,
	}
	cfg := operationConfig(10, 1)
	cfg.duration = time.Hour

	completed, err := runLoad(parent, cfg, client)
	require.EqualError(t, err, "worker 0: request 2: NTPv4 exchange: context deadline exceeded")
	require.Equal(t, 2, completed)
}

func TestExchangeDeadlineUsesEarlierDeadline(t *testing.T) {
	now := time.Unix(100, 0)
	parentDeadline := now.Add(500 * time.Millisecond)
	ctx, cancel := context.WithDeadline(t.Context(), parentDeadline)
	defer cancel()

	require.Equal(t, parentDeadline, exchangeDeadline(ctx, time.Second, now))
	require.Equal(t, now.Add(100*time.Millisecond), exchangeDeadline(ctx, 100*time.Millisecond, now))
	require.Equal(t, now.Add(time.Second), exchangeDeadline(t.Context(), time.Second, now))
}

func TestReusableLoadRefreshesDeadlineForEveryExchange(t *testing.T) {
	client := &fakeNTSClient{
		handshakeCookies: [][]byte{[]byte("cookie")},
		freshCookies:     map[string][][]byte{"cookie": {[]byte("cookie")}},
	}

	completed, err := runLoad(t.Context(), operationConfig(3, 1), client)
	require.NoError(t, err)
	require.Equal(t, 3, completed)
	sessions := client.sessionSnapshot()
	sessions[0].mu.Lock()
	deadlines := append([]time.Time(nil), sessions[0].deadlines...)
	sessions[0].mu.Unlock()
	require.Len(t, deadlines, 3)
	require.True(t, deadlines[1].After(deadlines[0]))
	require.True(t, deadlines[2].After(deadlines[1]))
}

func socketDeadlineError() error {
	return &net.OpError{
		Op:   "read",
		Net:  "udp",
		Addr: &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 11123},
		Err:  fmt.Errorf("socket deadline: %w", os.ErrDeadlineExceeded),
	}
}

func TestReusableLoadPreservesAuthenticationErrorAndRetiresSession(t *testing.T) {
	sentinel := errors.New("authenticator verification failed")
	client := &fakeNTSClient{
		handshakeCookies: [][]byte{[]byte("cookie")},
		exchangeErr:      sentinel,
	}

	completed, err := runLoad(t.Context(), operationConfig(10, 1), client)
	require.ErrorIs(t, err, sentinel)
	require.Zero(t, completed)
	requireAllSessionsClosed(t, client)
}

func operationConfig(requests, concurrency int) testConfig {
	return testConfig{
		addr:        "127.0.0.1:4460",
		ntpAddr:     "127.0.0.1:123",
		timeout:     time.Second,
		requests:    requests,
		concurrency: concurrency,
	}
}
