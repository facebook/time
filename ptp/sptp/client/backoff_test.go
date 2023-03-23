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

package client

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBackoffNone(t *testing.T) {
	cfg := BackoffConfig{Mode: backoffNone}
	require.NoError(t, cfg.Validate())
	b := newBackoff(cfg)
	require.False(t, b.active(), "disabled backoff is never active")
	require.Equal(t, 0, b.bump(), "bumping disabled backoff does nothing")
	require.False(t, b.active(), "bumped disabled backoff is still not active")
	require.Equal(t, 0, b.tick(), "ticking disabled backoff does nothing")
	require.False(t, b.active(), "ticked disabled backoff is still not active")
	b.reset()
	require.False(t, b.active(), "disabled backoff is not active after reset")
	require.Equal(t, 0, b.tick(), "ticking disabled backoff does nothing even after reset")
	require.False(t, b.active(), "disabled backoff is not active after reset and tick")
}

func TestBackoffFixed(t *testing.T) {
	cfg := BackoffConfig{Mode: backoffFixed, Step: 3, MaxValue: 10}
	require.NoError(t, cfg.Validate())
	b := newBackoff(cfg)
	require.False(t, b.active(), "fixed backoff is not active when init")
	require.Equal(t, 3, b.bump(), "bumping fixed backoff does something")
	require.True(t, b.active(), "bumped fixed backoff is active")
	require.Equal(t, 2, b.tick(), "ticking fixed backoff reduces the value")
	require.True(t, b.active(), "ticked fixed backoff is active")
	require.Equal(t, 1, b.tick(), "ticking fixed backoff reduces the value")
	require.True(t, b.active(), "ticked fixed backoff is active")
	require.Equal(t, 0, b.tick(), "ticking fixed backoff reduces the value")
	require.False(t, b.active(), "ticked fixed backoff is active once value is 0")

	require.Equal(t, 3, b.bump(), "bumping fixed backoff without resetting updates value in fixed fashion")
	require.Equal(t, 3, b.bump(), "bumping fixed backoff without resetting updates value in fixed fashion")
	require.True(t, b.active(), "maxed out fixed backoff is active")

	b.reset()
	require.False(t, b.active(), "linear backoff is not active after reset")
}

func TestBackoffLinear(t *testing.T) {
	cfg := BackoffConfig{Mode: backoffLinear, Step: 3, MaxValue: 10}
	require.NoError(t, cfg.Validate())
	b := newBackoff(cfg)
	require.False(t, b.active(), "linear backoff is not active when init")
	require.Equal(t, 3, b.bump(), "bumping linear backoff does something")
	require.True(t, b.active(), "bumped linear backoff is active")
	require.Equal(t, 2, b.tick(), "ticking linear backoff reduces the value")
	require.True(t, b.active(), "ticked linear backoff is active")
	require.Equal(t, 1, b.tick(), "ticking linear backoff reduces the value")
	require.True(t, b.active(), "ticked linear backoff is active")
	require.Equal(t, 0, b.tick(), "ticking linear backoff reduces the value")
	require.False(t, b.active(), "ticked linear backoff is active once value is 0")

	require.Equal(t, 6, b.bump(), "bumping linear backoff without resetting updates value in linear fashion")
	require.Equal(t, 9, b.bump(), "bumping linear backoff without resetting updates value in linear fashion")
	require.Equal(t, 10, b.bump(), "bumping linear backoff over max value produces max value")
	require.Equal(t, 10, b.bump(), "bumping linear backoff over max value produces max value")
	require.True(t, b.active(), "maxed out linear backoff is active")

	b.reset()
	require.False(t, b.active(), "linear backoff is not active after reset")
}

func TestBackoffExponential(t *testing.T) {
	cfg := BackoffConfig{Mode: backoffExponential, Step: 3, MaxValue: 30}
	require.NoError(t, cfg.Validate())
	b := newBackoff(cfg)
	require.False(t, b.active(), "exponential backoff is not active when init")
	require.Equal(t, 3, b.bump(), "bumping exponential backoff does something")
	require.True(t, b.active(), "bumped exponential backoff is active")
	require.Equal(t, 2, b.tick(), "ticking exponential backoff reduces the value")
	require.True(t, b.active(), "ticked exponential backoff is active")
	require.Equal(t, 1, b.tick(), "ticking exponential backoff reduces the value")
	require.True(t, b.active(), "ticked exponential backoff is active")
	require.Equal(t, 0, b.tick(), "ticking exponential backoff reduces the value")
	require.False(t, b.active(), "ticked exponential backoff is active once value is 0")

	require.Equal(t, 9, b.bump(), "bumping exponential backoff without resetting updates value in exponential fashion")
	require.Equal(t, 27, b.bump(), "bumping exponential backoff without resetting updates value in exponential fashion")
	require.Equal(t, 30, b.bump(), "bumping exponential backoff over max value produces max value")
	require.Equal(t, 30, b.bump(), "bumping exponential backoff over max value produces max value")
	require.True(t, b.active(), "maxed out exponential backoff is active")

	b.reset()
	require.False(t, b.active(), "exponential backoff is not active after reset")
}
