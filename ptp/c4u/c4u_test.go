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

package c4u

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/facebook/time/ptp/c4u/clock"
	"github.com/facebook/time/ptp/c4u/utcoffset"
	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/ptp4u/server"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	// We don't really care about UTCOffset here - just to be the same result as in c4u.Run()
	utcoffset, _ := utcoffset.Run()

	expected := &server.DynamicConfig{
		ClockClass:     clock.ClockClassUncalibrated,
		ClockAccuracy:  ptp.ClockAccuracyUnknown,
		DrainInterval:  30 * time.Second,
		MaxSubDuration: 1 * time.Hour,
		MetricInterval: 1 * time.Minute,
		MinSubInterval: 1 * time.Second,
		UTCOffset:      utcoffset,
	}

	cfg, err := ioutil.TempFile("", "c4u")
	require.NoError(t, err)
	defer os.Remove(cfg.Name())

	c := &Config{
		Path:   cfg.Name(),
		Sample: 3,
		Apply:  true,
	}

	Run(c, clock.NewRingBuffer(1))

	dc, err := server.ReadDynamicConfig(c.Path)
	require.NoError(t, err)
	require.Equal(t, expected, dc)
}
