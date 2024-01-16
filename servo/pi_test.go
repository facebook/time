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

package servo

import (
	"testing"

	"github.com/stretchr/testify/require"
)

/*
pi servo: sample 0, offset 1191, local_ts 1674148530671467104, last_freq -111288.406372
pi servo: sample 1, offset 225, local_ts 1674148531671518924, last_freq -111288.406372
pi servo: sample 2, offset 1170, local_ts 1674148532671555647, last_freq -112254.463816
pi servo: sample 2, offset 919, local_ts 1674148533671484215, last_freq -111084.463816
pi servo: sample 2, offset 654, local_ts 1674148534671526263, last_freq -110984.463816
pi servo: sample 2, offset 303, local_ts 1674148535671478938, last_freq -110973.763816

*/

func TestPiServoSample(t *testing.T) {
	pi := NewPiServo(DefaultServoConfig(), DefaultPiServoCfg(), -111288.406372)
	pi.SyncInterval(1)
	require.InEpsilon(t, -111288.406372, pi.lastFreq, 0.00001)
	require.InEpsilon(t, -111288.406372, pi.drift, 0.00001)

	freq, state := pi.Sample(1191, 1674148530671467104)
	require.InEpsilon(t, -111288.406372, freq, 0.00001)
	require.Equal(t, StateInit, state)

	freq, state = pi.Sample(225, 1674148531671518924)
	require.InEpsilon(t, -112254.463816, freq, 0.00001)
	require.Equal(t, StateLocked, state)

	freq, state = pi.Sample(1170, 1674148532671555647)
	require.InEpsilon(t, -111084.463816, freq, 0.00001)
	require.Equal(t, StateLocked, state)

	freq, state = pi.Sample(919, 1674148533671484215)
	require.InEpsilon(t, -110984.463816, freq, 0.00001)
	require.Equal(t, StateLocked, state)

	freq = pi.MeanFreq()
	require.InEpsilon(t, -110984.463816, freq, 0.00001)
}

func TestPiServoStepSample(t *testing.T) {
	cfg := DefaultServoConfig()
	cfg.FirstStepThreshold = 200000
	cfg.FirstUpdate = true
	pi := NewPiServo(cfg, DefaultPiServoCfg(), -111288.406372)
	pi.SyncInterval(1)
	require.InEpsilon(t, -111288.406372, pi.lastFreq, 0.00001)
	require.InEpsilon(t, -111288.406372, pi.drift, 0.00001)

	freq, state := pi.Sample(235000, 1674148528671467104)
	require.InEpsilon(t, -111288.406372, freq, 0.00001)
	require.Equal(t, StateInit, state)

	freq, state = pi.Sample(225000, 1674148529671518924)
	require.InEpsilon(t, -121289.001025, freq, 0.00001)
	require.Equal(t, StateJump, state)

	freq, state = pi.Sample(1191, 1674148530671467104)
	require.InEpsilon(t, -120098.001025, freq, 0.00001)
	require.Equal(t, StateLocked, state)

	freq, state = pi.Sample(225, 1674148531671518924)
	require.InEpsilon(t, -120706.701025, freq, 0.00001)
	require.Equal(t, StateLocked, state)
}

func TestPiServoFilterSample(t *testing.T) {
	pi := NewPiServo(DefaultServoConfig(), DefaultPiServoCfg(), -111288.406372)
	pi.SyncInterval(1)
	piFilterCfg := DefaultPiServoFilterCfg()
	piFilterCfg.ringSize = 3
	piFilterCfg.maxSkipCount = 2
	piFilterCfg.offsetRange = 100000
	f := NewPiServoFilter(pi, piFilterCfg)

	require.InEpsilon(t, -111288.406372, pi.lastFreq, 0.00001)
	require.InEpsilon(t, -111288.406372, pi.drift, 0.00001)

	freq, state := pi.Sample(1191, 1674148530671467104)
	require.InEpsilon(t, -111288.406372, freq, 0.00001)
	require.Equal(t, StateInit, state)

	freq, state = pi.Sample(225, 1674148531671518924)
	require.InEpsilon(t, -112254.463816, freq, 0.00001)
	require.Equal(t, StateLocked, state)

	freq, state = pi.Sample(1170, 1674148532671555647)
	require.InEpsilon(t, -111084.463816, freq, 0.00001)
	require.Equal(t, StateLocked, state)

	freq, state = pi.Sample(919, 1674148533671484215)
	require.InEpsilon(t, -110984.463816, freq, 0.00001)
	require.Equal(t, StateLocked, state)
	require.Equal(t, 0, pi.filter.skippedCount)

	freq, state = pi.Sample(919000, 1674148534671684215)
	require.InEpsilon(t, -111441.130482, freq, 0.00001)
	require.InEpsilon(t, f.freqMean, freq, 0.00001)
	require.Equal(t, StateFilter, state)
	require.Equal(t, 1, f.skippedCount)

	freq, state = pi.Sample(9190000, 1674148535671684215)
	require.InEpsilon(t, -111441.130482, freq, 0.00001)
	require.InEpsilon(t, f.freqMean, freq, 0.00001)
	require.Equal(t, StateFilter, state)
	require.Equal(t, 2, f.skippedCount)

	freq = pi.MeanFreq()
	require.InEpsilon(t, -111441.130482, freq, 0.00001)

	freq, state = pi.Sample(921000, 1674148535771674067)
	require.InEpsilon(t, -111441.130482, freq, 0.00001)
	require.Equal(t, f.freqMean, 0.0)
	require.Equal(t, StateInit, state)
}

func TestPiServoNoFilterSample(t *testing.T) {
	pi := NewPiServo(DefaultServoConfig(), DefaultPiServoCfg(), -111288.406372)
	pi.SyncInterval(1)
	piFilterCfg := DefaultPiServoFilterCfg()
	piFilterCfg.ringSize = 8
	piFilterCfg.maxSkipCount = 2
	f := NewPiServoFilter(pi, piFilterCfg)

	require.InEpsilon(t, -111288.406372, pi.lastFreq, 0.00001)
	require.InEpsilon(t, -111288.406372, pi.drift, 0.00001)

	freq, state := pi.Sample(1191, 1674148530671467104)
	require.InEpsilon(t, -111288.406372, freq, 0.00001)
	require.Equal(t, StateInit, state)

	freq, state = pi.Sample(225, 1674148531671518924)
	require.InEpsilon(t, -112254.463816, freq, 0.00001)
	require.Equal(t, StateLocked, state)

	freq, state = pi.Sample(1170, 1674148532671555647)
	require.InEpsilon(t, -111084.463816, freq, 0.00001)
	require.Equal(t, StateLocked, state)

	freq, state = pi.Sample(919, 1674148533671484215)
	require.InEpsilon(t, -110984.463816, freq, 0.00001)
	require.Equal(t, StateLocked, state)
	require.Equal(t, 0, pi.filter.skippedCount)

	_, state = pi.Sample(919000, 1674148534671684215)
	require.Equal(t, StateLocked, state)

	_, state = pi.Sample(9090000, 1674148535671684215)
	require.Equal(t, StateLocked, state)
	require.Equal(t, 0, f.skippedCount)
}

func TestPiServoSetFreq(t *testing.T) {
	pi := NewPiServo(DefaultServoConfig(), DefaultPiServoCfg(), -111288.406372)
	pi.SetLastFreq(11111.0025)

	require.InEpsilon(t, 11111.0025, pi.lastFreq, 0.00001)
	require.InEpsilon(t, 11111.0025, pi.drift, 0.00001)
}

func TestPiServoFilterMeanFreq(t *testing.T) {
	pi := NewPiServo(DefaultServoConfig(), DefaultPiServoCfg(), -111288.406372)
	pi.SyncInterval(1)
	piFilterCfg := DefaultPiServoFilterCfg()
	piFilterCfg.ringSize = 3
	piFilterCfg.maxSkipCount = 2
	piFilterCfg.offsetRange = 1000
	f := NewPiServoFilter(pi, piFilterCfg)

	require.InEpsilon(t, -111288.406372, pi.lastFreq, 0.00001)
	require.InEpsilon(t, -111288.406372, pi.drift, 0.00001)

	freq, state := pi.Sample(1191, 1674148530671467104)
	require.InEpsilon(t, -111288.406372, freq, 0.00001)
	require.Equal(t, StateInit, state)

	freq, state = pi.Sample(225, 1674148531671518924)
	require.InEpsilon(t, -112254.463816, freq, 0.00001)
	require.Equal(t, StateLocked, state)

	freq, state = pi.Sample(-170, 1674148532671555647)
	require.InEpsilon(t, -112424.463816, freq, 0.00001)
	require.Equal(t, StateLocked, state)

	freq, state = pi.Sample(68, 1674148533671484215)
	require.InEpsilon(t, -112237.463816, freq, 0.00001)
	require.Equal(t, StateLocked, state)
	require.Equal(t, 0, pi.filter.skippedCount)

	freq, state = pi.Sample(919000, 1674148534671684215)
	require.InEpsilon(t, -112305.463816, freq, 0.00001)
	require.InEpsilon(t, f.freqMean, freq, 0.00001)
	require.Equal(t, StateFilter, state)
	require.Equal(t, 1, f.skippedCount)

	freq = pi.MeanFreq()
	require.InEpsilon(t, -112305.463816, freq, 0.00001)

	freq, state = pi.Sample(1921000, 1674148535771674067)
	require.InEpsilon(t, -112305.463816, freq, 0.00001)
	require.Equal(t, StateFilter, state)
	require.Equal(t, 2, f.skippedCount)

	freq, state = pi.Sample(1921000, 1674148535771674067)
	require.InEpsilon(t, -112305.463816, freq, 0.00001)
	require.Equal(t, f.freqMean, 0.0)
	require.Equal(t, StateInit, state)
}
