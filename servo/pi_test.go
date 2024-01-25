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
	"fmt"
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

	spike := pi.IsSpike(919000)
	require.Equal(t, true, spike)
	freq = pi.MeanFreq()
	fmt.Println(freq)
	require.InEpsilon(t, -111441.130482, freq, 0.00001)
	require.InEpsilon(t, f.freqMean, freq, 0.00001)
	require.Equal(t, 1, f.skippedCount)

	require.True(t, pi.IsSpike(919000))
	freq = pi.MeanFreq()
	fmt.Println(freq)
	require.InEpsilon(t, -111441.130482, freq, 0.00001)
	require.InEpsilon(t, f.freqMean, freq, 0.00001)
	require.Equal(t, 2, f.skippedCount)

	require.True(t, pi.IsSpike(921000))
	freq = pi.MeanFreq()
	fmt.Println(freq)
	require.InEpsilon(t, -111441.130482, freq, 0.00001)
	require.Equal(t, 0, pi.count)
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

	require.False(t, pi.IsSpike(1191))

	freq, state := pi.Sample(1191, 1674148530671467104)
	require.InEpsilon(t, -111288.406372, freq, 0.00001)
	require.Equal(t, StateInit, state)

	require.False(t, pi.IsSpike(225))
	freq, state = pi.Sample(225, 1674148531671518924)
	require.InEpsilon(t, -112254.463816, freq, 0.00001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(1170))
	freq, state = pi.Sample(1170, 1674148532671555647)
	require.InEpsilon(t, -111084.463816, freq, 0.00001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(919))
	freq, state = pi.Sample(919, 1674148533671484215)
	require.InEpsilon(t, -110984.463816, freq, 0.00001)
	require.Equal(t, StateLocked, state)
	require.Equal(t, 0, pi.filter.skippedCount)

	require.False(t, pi.IsSpike(919000))
	_, state = pi.Sample(919000, 1674148534671684215)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(909000))
	_, state = pi.Sample(9090000, 1674148535671684215)
	require.Equal(t, StateLocked, state)
	require.Equal(t, 0, f.skippedCount)
}

func TestPiServoSetFreq(t *testing.T) {
	pi := NewPiServo(DefaultServoConfig(), DefaultPiServoCfg(), -111288.406372)
	pi.InitLastFreq(11111.0025)

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
	require.InEpsilon(t, -111288.406372, pi.filter.freqMean, 0.00001)

	freq, state := pi.Sample(1191, 1674148530671467104)
	require.InEpsilon(t, -111288.406372, freq, 0.00001)
	require.Equal(t, StateInit, state)
	fmt.Printf("freq: %f, meanFreq %f\n", freq, pi.MeanFreq())

	freq, state = pi.Sample(225, 1674148531671518924)
	require.InEpsilon(t, -112254.463816, freq, 0.00001)
	require.Equal(t, StateLocked, state)
	fmt.Printf("freq: %f, meanFreq %f\n", freq, pi.MeanFreq())

	freq, state = pi.Sample(-170, 1674148532671555647)
	require.InEpsilon(t, -112424.463816, freq, 0.00001)
	require.Equal(t, StateLocked, state)
	fmt.Printf("freq: %f, meanFreq %f\n", freq, pi.MeanFreq())

	freq, state = pi.Sample(68, 1674148533671484215)
	require.InEpsilon(t, -112237.463816, freq, 0.00001)
	require.Equal(t, StateLocked, state)
	require.Equal(t, 0, pi.filter.skippedCount)
	fmt.Printf("freq: %f, meanFreq %f\n", freq, pi.MeanFreq())

	require.True(t, pi.IsSpike(919000))
	freq = pi.MeanFreq()
	//freq, state = pi.Sample(919000, 1674148534671684215)
	require.InEpsilon(t, -112305.463816, freq, 0.00001)
	require.Equal(t, 1, f.skippedCount)
	fmt.Printf("freq: %f, meanFreq %f\n", freq, pi.MeanFreq())

	require.True(t, pi.IsSpike(-1921000))
	freq = pi.MeanFreq()
	require.InEpsilon(t, -112305.463816, freq, 0.00001)
	require.Equal(t, 2, f.skippedCount)
	fmt.Printf("freq: %f, meanFreq %f\n", freq, pi.MeanFreq())

	require.True(t, pi.IsSpike(1921000))
	freq = pi.MeanFreq()
	require.InEpsilon(t, -112305.463816, freq, 0.00001)
	require.Equal(t, f.freqMean, freq)
	fmt.Printf("freq: %f, meanFreq %f\n", freq, pi.MeanFreq())
}

/*
1705509028.124002 161053 sptp.go:395] offset         -1 s2 freq  -23186 path delay       4493
1705509029.124866 161053 sptp.go:395] offset        -13 s2 freq  -23198 path delay       4493
1705509030.124943 161053 sptp.go:395] offset          2 s2 freq  -23187 path delay       4493
1705509031.126138 161053 sptp.go:395] offset        -28 s2 freq  -23216 path delay       4493
1705509032.126981 161053 sptp.go:395] offset         -7 s2 freq  -23204 path delay       4493
1705509033.128078 161053 sptp.go:395] offset         14 s2 freq  -23185 path delay       4493
1705509034.128960 161053 sptp.go:395] offset          5 s2 freq  -23190 path delay       4493
1705509035.129991 161053 sptp.go:395] offset        -14 s2 freq  -23207 path delay       4494
1705509036.130273 161053 sptp.go:395] offset         -1 s2 freq  -23198 path delay       4494
1705509037.131229 161053 sptp.go:395] offset         23 s2 freq  -23175 path delay       4495
1705509038.132353 161053 sptp.go:395] offset        -17 s2 freq  -23208 path delay       4495
1705509039.133252 161053 sptp.go:395] offset          1 s2 freq  -23195 path delay       4495
1705509040.134036 161053 sptp.go:395] offset        -24 s2 freq  -23220 path delay       4495
1705509041.134984 161053 sptp.go:395] offset          3 s2 freq  -23200 path delay       4495
1705509042.136087 161053 sptp.go:395] offset         34 s2 freq  -23168 path delay       4495
1705509043.137061 161053 sptp.go:395] offset          1 s2 freq  -23191 path delay       4495
1705509044.137951 161053 sptp.go:395] offset         16 s2 freq  -23175 path delay       4495
1705509045.138549 161053 sptp.go:395] offset         -9 s2 freq  -23196 path delay       4495
1705509046.138969 161053 sptp.go:395] offset        -10 s2 freq  -23199 path delay       4495
1705509047.140065 161053 sptp.go:395] offset         15 s2 freq  -23177 path delay       4495
1705509048.141196 161053 sptp.go:395] offset          1 s2 freq  -23187 path delay       4495
1705509049.141153 161053 sptp.go:395] offset        -24 s2 freq  -23212 path delay       4496
1705509050.142218 161053 sptp.go:395] offset         -6 s2 freq  -23201 path delay       4496
1705509051.143105 161053 sptp.go:395] offset         -3 s2 freq  -23200 path delay       4496
1705509052.144188 161053 sptp.go:395] offset         21 s2 freq  -23176 path delay       4496
1705509053.145134 161053 sptp.go:395] offset         11 s2 freq  -23180 path delay       4496
1705509054.145250 161053 sptp.go:395] offset        -15 s2 freq  -23203 path delay       4496
1705509055.146215 161053 sptp.go:395] offset         -6 s2 freq  -23198 path delay       4496
1705509056.147176 161053 sptp.go:395] offset         -3 s2 freq  -23197 path delay       4496
1705509057.147938 161053 sptp.go:395] offset         18 s2 freq  -23177 path delay       4496
1705509058.148857 161053 sptp.go:395] offset         14 s2 freq  -23176 path delay       4496
1705509059.149155 161053 sptp.go:395] offset         -3 s2 freq  -23188 path delay       4496
1705509060.150073 161053 sptp.go:395] offset        -27 s2 freq  -23213 path delay       4496
1705509061.151095 161053 sptp.go:395] offset        -11 s2 freq  -23205 path delay       4496
1705509062.152144 161053 sptp.go:395] offset        -13 s2 freq  -23211 path delay       4496
1705509063.152956 161053 sptp.go:395] offset         37 s2 freq  -23165 path delay       4496
1705509064.153914 161053 sptp.go:395] offset         25 s2 freq  -23166 path delay       4496
1705509065.155252 161053 sptp.go:395] offset        -10 s2 freq  -23193 path delay       4496
1705509066.156531 161053 sptp.go:395] offset        -18 s2 freq  -23204 path delay       4496
1705509067.157337 161053 sptp.go:395] offset        -22 s2 freq  -23213 path delay       4496
1705509068.157934 161053 sptp.go:395] offset         17 s2 freq  -23181 path delay       4496
1705509069.158955 161053 sptp.go:395] offset          3 s2 freq  -23190 path delay       4496
1705509070.160033 161053 sptp.go:395] offset        -26 s2 freq  -23218 path delay       4496
1705509071.161056 161053 sptp.go:395] offset         11 s2 freq  -23189 path delay       4496
1705509072.161972 161053 sptp.go:395] offset        -20 s2 freq  -23217 path delay       4496
1705509073.163181 161053 sptp.go:395] offset         22 s2 freq  -23181 path delay       4496
1705509074.163961 161053 sptp.go:395] offset         -8 s2 freq  -23204 path delay       4496
1705509075.165209 161053 sptp.go:395] offset          0 s2 freq  -23198 path delay       4496
1705509076.166091 161053 sptp.go:395] offset        -27 s2 freq  -23225 path delay       4495
1705509077.167153 161053 sptp.go:395] offset         -3 s2 freq  -23209 path delay       4495
1705509078.168353 161053 sptp.go:395] offset         23 s2 freq  -23184 path delay       4495
1705509079.169090 161053 sptp.go:395] offset         -5 s2 freq  -23205 path delay       4495
1705509080.170178 161053 sptp.go:395] offset         -5 s2 freq  -23207 path delay       4495
1705509081.170962 161053 sptp.go:395] offset        -23 s2 freq  -23226 path delay       4494
1705509082.172252 161053 sptp.go:395] offset     175101 s3 freq  -23197 path delay       4495
*/
func TestPiServoFilterSample2(t *testing.T) {
	pi := NewPiServo(DefaultServoConfig(), DefaultPiServoCfg(), -23186)
	pi.SyncInterval(1)
	piFilterCfg := DefaultPiServoFilterCfg()
	piFilterCfg.ringSize = 30
	piFilterCfg.maxSkipCount = 15
	_ = NewPiServoFilter(pi, piFilterCfg)

	require.False(t, pi.IsSpike(-1))

	freq, state := pi.Sample(-1, 1705509028124002000)
	require.InEpsilon(t, -23186.0, freq, 0.00001)
	require.Equal(t, StateInit, state)

	require.False(t, pi.IsSpike(-13))
	freq, state = pi.Sample(-13, 1705509029124866000)
	require.InEpsilon(t, -23198.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(2))
	freq, state = pi.Sample(2, 1705509030124943000)
	require.InEpsilon(t, -23187.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-28))
	freq, state = pi.Sample(-28, 1705509031126138000)
	require.InEpsilon(t, -23216.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-7))
	freq, state = pi.Sample(-7, 1705509032126981000)
	require.InEpsilon(t, -23204.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(14))
	freq, state = pi.Sample(14, 1705509033128078000)
	require.InEpsilon(t, -23185.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(5))
	freq, state = pi.Sample(5, 1705509034128960000)
	require.InEpsilon(t, -23190.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-14))
	freq, state = pi.Sample(-14, 1705509035129991000)
	require.InEpsilon(t, -23207.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-1))
	freq, state = pi.Sample(-1, 1705509036130273000)
	require.InEpsilon(t, -23198.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(23))
	freq, state = pi.Sample(23, 1705509037131229000)
	require.InEpsilon(t, -23175.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-17))
	freq, state = pi.Sample(-17, 1705509038132353000)
	require.InEpsilon(t, -23208.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(1))
	freq, state = pi.Sample(1, 1705509039133252000)
	require.InEpsilon(t, -23195.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-24))
	freq, state = pi.Sample(-24, 1705509040134036000)
	require.InEpsilon(t, -23220.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(3))
	freq, state = pi.Sample(3, 1705509041134984000)
	require.InEpsilon(t, -23200.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(34))
	freq, state = pi.Sample(34, 1705509042136087000)
	require.InEpsilon(t, -23168.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(1))
	freq, state = pi.Sample(1, 1705509043137061000)
	require.InEpsilon(t, -23191.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(16))
	freq, state = pi.Sample(16, 1705509044137951000)
	require.InEpsilon(t, -23175.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-9))
	freq, state = pi.Sample(-9, 1705509045138549000)
	require.InEpsilon(t, -23196.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-10))
	freq, state = pi.Sample(-10, 1705509046138969000)
	require.InEpsilon(t, -23199.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(15))
	freq, state = pi.Sample(15, 1705509047140065000)
	require.InEpsilon(t, -23177.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(1))
	freq, state = pi.Sample(1, 1705509048141196000)
	require.InEpsilon(t, -23187.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-24))
	freq, state = pi.Sample(-24, 1705509049141153000)
	require.InEpsilon(t, -23212.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-6))
	freq, state = pi.Sample(-6, 1705509050142218000)
	require.InEpsilon(t, -23201.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-3))
	freq, state = pi.Sample(-3, 1705509051143105000)
	require.InEpsilon(t, -23200.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(21))
	freq, state = pi.Sample(21, 1705509052144188000)
	require.InEpsilon(t, -23176.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(11))
	freq, state = pi.Sample(11, 1705509053145134000)
	require.InEpsilon(t, -23180.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-15))
	freq, state = pi.Sample(-15, 1705509054145250000)
	require.InEpsilon(t, -23203.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-6))
	freq, state = pi.Sample(-6, 1705509055146215000)
	require.InEpsilon(t, -23198.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-3))
	freq, state = pi.Sample(-3, 1705509056147176000)
	require.InEpsilon(t, -23197.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(18))
	freq, state = pi.Sample(18, 1705509057147938000)
	require.InEpsilon(t, -23177.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(14))
	freq, state = pi.Sample(14, 1705509058148857000)
	require.InEpsilon(t, -23176.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-3))
	freq, state = pi.Sample(-3, 1705509059149155000)
	require.InEpsilon(t, -23188.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-27))
	freq, state = pi.Sample(-27, 1705509060150073000)
	require.InEpsilon(t, -23213.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-11))
	freq, state = pi.Sample(-11, 1705509061151095000)
	require.InEpsilon(t, -23205.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-13))
	freq, state = pi.Sample(-13, 1705509062152144000)
	require.InEpsilon(t, -23211.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(37))
	freq, state = pi.Sample(37, 1705509063152956000)
	require.InEpsilon(t, -23165.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(25))
	freq, state = pi.Sample(25, 1705509064153914000)
	require.InEpsilon(t, -23166.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-10))
	freq, state = pi.Sample(-10, 1705509065155252000)
	require.InEpsilon(t, -23193.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-18))
	freq, state = pi.Sample(-18, 1705509066156531000)
	require.InEpsilon(t, -23204.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-22))
	freq, state = pi.Sample(-22, 1705509067157337000)
	require.InEpsilon(t, -23213.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(17))
	freq, state = pi.Sample(17, 1705509068157934000)
	require.InEpsilon(t, -23181.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(3))
	freq, state = pi.Sample(3, 1705509069158955000)
	require.InEpsilon(t, -23190.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-26))
	freq, state = pi.Sample(-26, 1705509070160033000)
	require.InEpsilon(t, -23218.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(11))
	freq, state = pi.Sample(11, 1705509071161056000)
	require.InEpsilon(t, -23189.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-20))
	freq, state = pi.Sample(-20, 1705509072161972000)
	require.InEpsilon(t, -23217.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(22))
	freq, state = pi.Sample(22, 1705509073163181000)
	require.InEpsilon(t, -23181.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-8))
	freq, state = pi.Sample(-8, 1705509074163961000)
	require.InEpsilon(t, -23204.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(0))
	freq, state = pi.Sample(0, 1705509075165209000)
	require.InEpsilon(t, -23198.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-27))
	freq, state = pi.Sample(-27, 1705509076166091000)
	require.InEpsilon(t, -23225.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-3))
	freq, state = pi.Sample(-3, 1705509077167153000)
	require.InEpsilon(t, -23209.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(23))
	freq, state = pi.Sample(23, 1705509078168353000)
	require.InEpsilon(t, -23184.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-5))
	freq, state = pi.Sample(-5, 1705509079169090000)
	require.InEpsilon(t, -23205.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-5))
	freq, state = pi.Sample(-5, 1705509080170178000)
	require.InEpsilon(t, -23207.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.False(t, pi.IsSpike(-23))
	freq, state = pi.Sample(-23, 1705509081170962000)
	require.InEpsilon(t, -23226.000, freq, 0.001)
	require.Equal(t, StateLocked, state)

	require.True(t, pi.IsSpike(175101))
	/* pi.Sample should not be called after IsSpike is true
	   freq, state = pi.Sample(175101, 1705509082172252000)
	*/
	freq = pi.MeanFreq()
	require.InEpsilon(t, -23197.000, freq, 0.001)

	/*
		I0117 08:31:23.173349 161053 sptp.go:395] offset      96571 s2 freq  +73361 path delay       4495
	*/
	require.True(t, pi.IsSpike(96571))
	/* pi.Sample should not be called after IsSpike is true
	   freq, state = pi.Sample(96571, 1705509083173349000)
	*/
	freq = pi.MeanFreq()
	require.InEpsilon(t, -23197.000, freq, 0.001)
}
