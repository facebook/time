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
