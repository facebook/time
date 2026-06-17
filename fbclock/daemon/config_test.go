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

package daemon

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEvalAndValidate(t *testing.T) {
	c := &Config{
		LinearizabilityTestInterval:    -1 * time.Second,
		LinearizabilityTestMaxGMOffset: -1 * time.Second,
		Math: Math{
			M:     "1",
			W:     "1",
			Drift: "1",
		},
	}
	require.Equal(t, fmt.Errorf("bad config: 'ptpclientaddress'"), c.EvalAndValidate())

	c.PTPClientAddress = "some address"
	require.Equal(t, fmt.Errorf("bad config: 'ringsize' must be >0"), c.EvalAndValidate())

	c.RingSize = 42
	require.Equal(t, fmt.Errorf("bad config: 'interval' must be between 0 and 1 minute"), c.EvalAndValidate())

	c.Interval = 1 * time.Microsecond
	require.Equal(t, fmt.Errorf("bad config: 'test interval' must be positive"), c.EvalAndValidate())

	c.LinearizabilityTestInterval = 1 * time.Microsecond
	require.Equal(t, fmt.Errorf("bad config: 'offset' must be positive"), c.EvalAndValidate())

	c.LinearizabilityTestMaxGMOffset = 1 * time.Microsecond
	require.Nil(t, c.EvalAndValidate())
}

func TestPostponeStart(t *testing.T) {
	uptime, err := uptime()
	require.NoError(t, err)
	require.True(t, uptime > 0)

	delay := 500 * time.Millisecond
	c := Config{BootDelay: uptime + delay}

	start := time.Now()
	err = c.PostponeStart()
	require.True(t, time.Since(start) >= delay)
	require.NoError(t, err)
}

// TestConfigBuildsKFactors checks EvalAndValidate precomputes the warm-up tolerance-factor table
// (k[2..RingSize]) at parse time, so the per-tick hot path is just a lookup. gamma is no longer a config
// field -- the warm-up confidence is hardcoded (warmupConfidence), so there is nothing to validate.
func TestConfigBuildsKFactors(t *testing.T) {
	cfg := &Config{
		PTPClientAddress: "/tmp/fbclock-test",
		RingSize:         30,
		Interval:         time.Second,
		Math: Math{
			M:     MathDefaultM,
			W:     MathDefaultW,
			Drift: MathDefaultDrift,
		},
	}
	require.NoError(t, cfg.EvalAndValidate())
	require.Len(t, cfg.kFactors, cfg.RingSize+1)
}

// TestConfigGradualWindowRequiresK locks the fail-closed guard: with GradualWindow on, a W formula that
// never references k is rejected at parse (it would apply a flat 4.0 to a 2-sample stddev); the default W
// (which uses k) is accepted; and with the flag off the same k-less formula is fine.
func TestConfigGradualWindowRequiresK(t *testing.T) {
	const wNoK = "mean(m, 100) + 4.0 * stddev(m, 100)"
	makeCfg := func(gradual bool, w string) *Config {
		return &Config{
			PTPClientAddress: "/tmp/fbclock-test",
			RingSize:         30,
			Interval:         time.Second,
			GradualWindow:    gradual,
			Math: Math{
				M:     MathDefaultM,
				W:     w,
				Drift: MathDefaultDrift,
			},
		}
	}

	require.Error(t, makeCfg(true, wNoK).EvalAndValidate(), "gradualwindow + W without k must fail closed")
	require.NoError(t, makeCfg(true, MathDefaultW).EvalAndValidate(), "gradualwindow + default W (has k) must pass")
	require.NoError(t, makeCfg(false, wNoK).EvalAndValidate(), "flag off + W without k must pass")
}
