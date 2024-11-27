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

package cmd

import (
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResetTimestamps(t *testing.T) {
	ts := timestamps{}
	ts.t1 = time.Now()
	ts.t2 = time.Now()
	ts.t3 = time.Now()
	ts.t4 = time.Now()

	ts.reset()
	require.True(t, ts.t1.IsZero())
	require.True(t, ts.t2.IsZero())
	require.True(t, ts.t3.IsZero())
	require.True(t, ts.t4.IsZero())
}

func TestCollectionTimeout(t *testing.T) {
	timeout := time.Millisecond
	p := &ptping{}
	start := time.Now()
	err := p.timestamps(timeout)
	require.Greater(t, time.Since(start), timeout)
	require.Equal(t, fmt.Errorf("timeout waiting"), err)
}

func TestT1IsZero(t *testing.T) {
	ts := timestamps{}
	ts.t1 = time.Date(0001, time.January, 1, 0, 0, 0, 0, time.UTC)
	ts.t2 = time.Date(2024, time.November, 1, 9, 0, 0, 2, time.UTC)
	ts.t3 = time.Date(2024, time.November, 1, 9, 0, 0, 0, time.UTC)
	ts.t4 = time.Date(2024, time.November, 1, 9, 0, 0, 1, time.UTC)
	server := "mars"
	count := 1
	totalRTT := time.Duration(500_000)

	readStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	ptpingOutput(count, server, totalRTT, ts)

	w.Close()
	out, _ := io.ReadAll(r)
	os.Stdout = readStdout

	ptpingOutput(count, server, totalRTT, ts)
	expectedOutput := "mars: seq=1 net=NaN (->1ns + <-NaN)\trtt=500µs\n"
	if string(out) != expectedOutput {
		t.Errorf("Expected %s, got %s", expectedOutput, out)
	}
}

func TestT2IsZero(t *testing.T) {
	ts := timestamps{}
	ts.t1 = time.Date(2024, time.November, 1, 9, 0, 0, 101, time.UTC)
	ts.t2 = time.Date(0001, time.January, 1, 0, 0, 0, 0, time.UTC)
	ts.t3 = time.Date(2024, time.November, 1, 9, 0, 0, 0, time.UTC)
	ts.t4 = time.Date(2024, time.November, 1, 9, 0, 0, 1, time.UTC)
	server := "jupiter"
	count := 2
	totalRTT := time.Duration(250_000)

	readStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	ptpingOutput(count, server, totalRTT, ts)

	w.Close()
	out, _ := io.ReadAll(r)
	os.Stdout = readStdout

	ptpingOutput(count, server, totalRTT, ts)
	expectedOutput := "jupiter: seq=2 net=NaN (->1ns + <-NaN)\trtt=250µs\n"
	if string(out) != expectedOutput {
		t.Errorf("Expected %s, got %s", expectedOutput, out)
	}
}
