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
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExactFactorPublishedTable validates the exact one-sided noncentral-t tolerance factor against an
// EXTERNAL published tolerance table (p=0.90, gamma=0.95). These are standard one-sided normal
// tolerance factors (NIST e-Handbook 7.2.6.3 / Krishnamoorthy & Mathew); matching them confirms the
// whole betainc -> nctCDF -> nctPPF chain is correct, independent of the daemon's z_p=4 anchoring.
func TestExactFactorPublishedTable(t *testing.T) {
	const zp90 = 1.2815515594908223 // Phi^-1(0.90)
	want := map[float64]float64{
		3:  6.158,
		4:  4.163,
		5:  3.407,
		7:  2.755,
		10: 2.355,
		15: 2.068,
	}
	for n, w := range want {
		require.InDeltaf(t, w, exactFactor(n, zp90, 0.95), 0.01, "exactFactor(n=%v, p=0.90, gamma=0.95)", n)
	}
}

// TestExactFactorMethodsAgree cross-checks the two exact methods in their overlap, at the shipped
// warmupConfidence: the AS243 noncentral-t quantile and the chi-density integral. They must agree where
// both are valid (delta<36), which is what validates the integral branch used at the RingSize=100 anchor
// (delta=40) where AS243 underflows. Run at the high operating gamma -- that is the deep-tail path.
func TestExactFactorMethodsAgree(t *testing.T) {
	for _, n := range []float64{40, 60, 80} {
		delta := math.Sqrt(n) * coverageZP
		as243 := nctPPF(warmupConfidence, n-1, delta) / math.Sqrt(n)
		integral := exactFactorIntegral(n, coverageZP, warmupConfidence)
		require.InEpsilonf(t, as243, integral, 1e-3, "n=%v: AS243=%v integral=%v", n, as243, integral)
	}
}

// TestNormCDF validates the Erfc-based standard-normal CDF against textbook Phi(z) values -- the one
// special function every other routine here (noncentral-t, the tolerance integral) is built on.
func TestNormCDF(t *testing.T) {
	cases := []struct{ z, want float64 }{
		{0.0, 0.5},
		{1.0, 0.8413447460685429},
		{-1.0, 0.15865525393145707},
		{2.0, 0.9772498680518208},
		{4.0, 0.9999683287581669},
	}
	for _, c := range cases {
		require.InDeltaf(t, c.want, normCDF(c.z), 1e-9, "normCDF(%v)", c.z)
	}
}

// TestExactFactorZP4 spot-checks the factor at the operating point (z_p=4, gamma=0.9999) against
// independently computed reference values: F(100)=5.437 (integral branch, delta=40), raw k(4)=97.41
// (AS243 branch), anchored K(4)=4*k(4)/F(100)=71.67. This is the z_p=4 path the daemon ships, which the
// published p=0.90 table never reaches (its delta stays < 36).
func TestExactFactorZP4(t *testing.T) {
	require.InDelta(t, 5.437, exactFactor(100, coverageZP, warmupConfidence), 0.05) // F(100), integral branch
	require.InDelta(t, 97.41, exactFactor(4, coverageZP, warmupConfidence), 0.5)    // raw k(4), AS243 branch
	k := precomputeGradualWindowFactors(100, warmupConfidence)
	require.InDelta(t, 71.67, k[4], 0.5) // anchored K(4) the daemon publishes at n=4
}

// TestGradualWindowProductionSafetyInvariant guards the precomputed k(n) factor table at the shipped
// gamma=0.9999: every factor is finite, >= 4, strictly decreasing, and exactly 4 at the full ring, so a
// warm-up factor is never tighter than steady. Runs RingSize=100 (anchor fN via the integral routine) and
// a smaller ring (anchor fN via AS243). Tests the table, not runtime W.
func TestGradualWindowProductionSafetyInvariant(t *testing.T) {
	// anchor fN (delta=40, the integral routine); it scales every k(n) at RingSize=100.
	require.InDelta(t, 5.437, exactFactorIntegral(100, coverageZP, warmupConfidence), 0.05)

	// RingSize=100 anchors fN via the integral routine; a smaller ring anchors it via AS243. Check both.
	for _, ringSize := range []int{30, 100} {
		k := precomputeGradualWindowFactors(ringSize, warmupConfidence)
		require.Lenf(t, k, ringSize+1, "ringSize=%d table length", ringSize)
		require.Equalf(t, coverageZP, k[ringSize], "ringSize=%d anchor k(RingSize) must equal coverageZP", ringSize)
		for n := 2; n <= ringSize; n++ {
			require.Falsef(t, math.IsNaN(k[n]) || math.IsInf(k[n], 0), "ringSize=%d k(%d) must be finite", ringSize, n)
			require.GreaterOrEqualf(t, k[n], coverageZP, "ringSize=%d k(%d) must stay >= coverageZP", ringSize, n)
			if n > 2 {
				require.Lessf(t, k[n], k[n-1], "ringSize=%d k(%d) must be < k(%d)", ringSize, n, n-1)
			}
		}
		require.Greaterf(t, k[2], 10.0, "ringSize=%d k(2) should be large", ringSize)
	}
}
