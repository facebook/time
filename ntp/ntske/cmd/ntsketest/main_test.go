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
	cancelAfter      int64
	cancel           context.CancelFunc
	handshakeCalls   atomic.Int64
	exchangeCalls    atomic.Int64
	mu               sync.Mutex
	consumedCookies  [][]byte
}

func (f *fakeNTSClient) handshake(context.Context, testConfig) (*ntske.HandshakeResult, error) {
	f.handshakeCalls.Add(1)
	return &ntske.HandshakeResult{Cookies: f.handshakeCookies}, nil
}

func (f *fakeNTSClient) exchange(
	ctx context.Context,
	_ testConfig,
	_ string,
	_ *ntske.HandshakeResult,
	cookie []byte,
) ([][]byte, error) {
	call := f.exchangeCalls.Add(1)
	if f.cancelAfter > 0 && call == f.cancelAfter {
		f.cancel()
	}
	if f.blockAfter > 0 && call > f.blockAfter {
		<-ctx.Done()
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
}

func TestFreshKEPerRequestPerformsHandshakeForEveryRequest(t *testing.T) {
	client := &fakeNTSClient{handshakeCookies: [][]byte{[]byte("cookie")}}
	cfg := operationConfig(5, 2)
	cfg.freshKEPerRequest = true

	completed, err := runLoad(t.Context(), cfg, client)
	require.NoError(t, err)
	require.Equal(t, 5, completed)
	require.Equal(t, int64(5), client.handshakeCalls.Load())
}

func TestSkipNTPPerformsFreshHandshakeForEveryRequest(t *testing.T) {
	client := &fakeNTSClient{}
	cfg := operationConfig(5, 2)
	cfg.skipNTP = true

	completed, err := runLoad(t.Context(), cfg, client)
	require.NoError(t, err)
	require.Equal(t, 5, completed)
	require.Equal(t, int64(5), client.handshakeCalls.Load())
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
}

func TestReusableLoadPreservesAuthenticationError(t *testing.T) {
	sentinel := errors.New("authenticator verification failed")
	client := &fakeNTSClient{
		handshakeCookies: [][]byte{[]byte("cookie")},
		exchangeErr:      sentinel,
	}

	completed, err := runLoad(t.Context(), operationConfig(10, 1), client)
	require.ErrorIs(t, err, sentinel)
	require.Zero(t, completed)
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
