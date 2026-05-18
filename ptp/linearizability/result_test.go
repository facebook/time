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

package linearizability

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPTPTestResultTarget(t *testing.T) {
	tr := PTPTestResult{Server: "server01"}
	require.Equal(t, "server01", tr.Target())
}

func TestPTPTestConfigTarget(t *testing.T) {
	cfg := &PTPTestConfig{Server: "original"}
	cfg.Target("new-server")
	require.Equal(t, "new-server", cfg.Server)
}

func TestSPTPHTTPTestResultTarget(t *testing.T) {
	tr := SPTPHTTPTestResult{
		Config: SPTPHTTPTestConfig{Server: "gm01"},
	}
	require.Equal(t, "gm01", tr.Target())
}

func TestSPTPHTTPTestConfigTarget(t *testing.T) {
	cfg := &SPTPHTTPTestConfig{Server: "original"}
	cfg.Target("new-gm")
	require.Equal(t, "new-gm", cfg.Server)
}

func TestPTPTestResultDelta(t *testing.T) {
	tr := PTPTestResult{
		TXTimestamp: time.Unix(0, 1000),
		RXTimestamp: time.Unix(0, 1500),
	}
	require.Equal(t, 500*time.Nanosecond, tr.Delta())
}

func TestPTPTestResultDeltaNegative(t *testing.T) {
	tr := PTPTestResult{
		TXTimestamp: time.Unix(0, 2000),
		RXTimestamp: time.Unix(0, 1000),
	}
	require.Equal(t, -1000*time.Nanosecond, tr.Delta())
}

func TestPTPTestResultGoodSuccess(t *testing.T) {
	tr := PTPTestResult{
		TXTimestamp: time.Unix(0, 1000),
		RXTimestamp: time.Unix(0, 2000),
	}
	good, err := tr.Good()
	require.NoError(t, err)
	require.True(t, good)
}

func TestPTPTestResultGoodNegativeDelta(t *testing.T) {
	tr := PTPTestResult{
		TXTimestamp: time.Unix(0, 2000),
		RXTimestamp: time.Unix(0, 1000),
	}
	good, err := tr.Good()
	require.NoError(t, err)
	require.False(t, good)
}

func TestPTPTestResultGoodError(t *testing.T) {
	tr := PTPTestResult{
		Error: fmt.Errorf("test error"),
	}
	good, err := tr.Good()
	require.Error(t, err)
	require.False(t, good)
}

func TestPTPTestResultExplainSuccess(t *testing.T) {
	tr := PTPTestResult{
		Server:      "server01",
		TXTimestamp: time.Unix(0, 1000),
		RXTimestamp: time.Unix(0, 2000),
	}
	require.Contains(t, tr.Explain(), "passed")
}

func TestPTPTestResultExplainError(t *testing.T) {
	tr := PTPTestResult{
		Server: "server01",
		Error:  fmt.Errorf("test error"),
	}
	require.Contains(t, tr.Explain(), "error")
}

func TestPTPTestResultExplainFailed(t *testing.T) {
	tr := PTPTestResult{
		Server:      "server01",
		TXTimestamp: time.Unix(0, 2000),
		RXTimestamp: time.Unix(0, 1000),
	}
	require.Contains(t, tr.Explain(), "failed")
}

func TestPTPTestResultErr(t *testing.T) {
	testErr := fmt.Errorf("test error")
	tr := PTPTestResult{Error: testErr}
	require.Equal(t, testErr, tr.Err())

	tr2 := PTPTestResult{}
	require.NoError(t, tr2.Err())
}
