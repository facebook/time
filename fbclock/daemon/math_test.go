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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConvolve(t *testing.T) {
	input := []float64{
		-1.70422422e+08,
		-1.69753965e+08,
		-1.68707777e+08,
		-1.67177413e+08,
		-1.65204696e+08,
		-1.66994577e+08,
	}
	coeffs1 := []float64{0.2, 0.2, 0.2, 0.2, 0.2}

	got1, err := convolve(input, coeffs1)
	require.Nil(t, err)
	/*
		how to get this output (using Python and numpy):
			import numpy as np
			x = np.array([-1.70422422e+08, -1.69753965e+08, -1.68707777e+08, -1.67177413e+08, -1.65204696e+08, -1.66994577e+08])
			coeffs = np.array([0.2, 0.2, 0.2, 0.2, 0.2])
			c = np.convolve(x, coeffs)
			print(c)
	*/
	want1 := []float64{
		-3.40844844e+07,
		-6.80352774e+07,
		-1.01776833e+08,
		-1.35212315e+08,
		-1.68253255e+08,
		-1.67567686e+08,
	}
	require.Equal(t, len(want1), len(got1))
	require.InEpsilonSlice(t, want1, got1, 1e+10)

	coeffs2 := []float64{1, 3, -3, 1}
	got2, err := convolve(input, coeffs2)
	require.Nil(t, err)
	want2 := []float64{
		-1.70422422e+08,
		-6.81021231e+08,
		-1.66702406e+08,
		-3.34461271e+08,
		-3.30367569e+08,
		-3.29784203e+08,
	}
	require.Equal(t, len(want2), len(got2))
	require.InEpsilonSlice(t, want2, got2, 1e+10)
}

func TestMean(t *testing.T) {
	input := []float64{3, 5, 8, 8}
	want := 6.0
	require.Equal(t, want, mean(input))
	input = []float64{1, 4, 0, 3, 8}
	want = 3.2
	require.Equal(t, want, mean(input))
}

func TestVariance(t *testing.T) {
	input := []float64{8, 8, 8, 8}
	want := 0.0
	require.Equal(t, want, variance(input))
	input = []float64{1, 4, 0, 3, 8}
	want = 9.7
	require.Equal(t, want, variance(input))
}

func TestPrepareExpression(t *testing.T) {
	input := "mean(clockaccuracy, 5) + abs(mean(offset, 5)) + 1.0 * stddev(offset, 4) + 1.0 * stddev(delay, 4) + 1.0 * stddev(freq, 5)"
	expr, err := prepareExpression(input)
	require.Nil(t, err)

	parameters := map[string]interface{}{
		"offset":        []float64{1, 2, 3, 4, 5},
		"delay":         []float64{2, 2, 2, 2, 2},
		"freq":          []float64{123, 424, 444, 222, 424},
		"clockaccuracy": []float64{100, 150, 50, 100, 100},
	}

	want := 250.1909601786838
	got, err := expr.Evaluate(parameters)
	require.Nil(t, err)
	require.Equal(t, want, got)
}

func TestPrepareExpressionWrongVari(t *testing.T) {
	input := "abs(mean(offset, 5)) + 1.0 * stddev(missing, 4)"
	_, err := prepareExpression(input)
	require.Error(t, err)
}

func TestPrepareExpressionNotEnoughValues(t *testing.T) {
	input := "mean(offset, 50)"
	expr, err := prepareExpression(input)
	require.Nil(t, err)

	parameters := map[string]interface{}{
		"offset": []float64{1, 2, 3, 4, 5},
	}

	r, err := expr.Evaluate(parameters)
	require.NoError(t, err)
	require.Equal(t, float64(3), r)
}

// TestPrepareExpressionMalformedInput reproduces OSS-Fuzz issue 471488972.
func TestPrepareExpressionMalformedInput(t *testing.T) {
	inputs := []string{
		"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00",
		"[[[[[[[[[[[",
		string([]byte{0xff, 0xfe, 0xfd}),
		"",
	}
	for _, input := range inputs {
		require.NotPanics(t, func() {
			_, _ = prepareExpression(input)
		})
	}
}

func FuzzPrepareExpression(f *testing.F) {
	f.Add("mean(clockaccuracy, 5) + abs(mean(offset, 5)) + 1.0 * stddev(offset, 4) + 1.0 * stddev(delay, 4) + 1.0 * stddev(freq, 5)")
	f.Add("abs(mean(offset, 5)) + 1.0 * stddev(missing, 4)")
	f.Add("mean(offset, 50)")

	f.Fuzz(func(t *testing.T, input string) {
		_, _ = prepareExpression(input)
	})
}

// TestGradualWindowByteIdentityAtFullRing locks the invariant that the new k(n) W expression reduces
// bit-for-bit to the legacy "mean(m, N) + 4.0 * stddev(m, N)" once the ring is full (n == N).
func TestGradualWindowByteIdentityAtFullRing(t *testing.T) {
	cases := []struct {
		n      int
		legacy string
	}{
		{30, "mean(m, 30) + 4.0 * stddev(m, 30)"},
		{100, "mean(m, 100) + 4.0 * stddev(m, 100)"},
	}
	for _, tc := range cases {
		ms := make([]float64, tc.n)
		for i := range ms {
			ms[i] = 100 + float64(i%7) // varied so stddev(m) > 0
		}
		newExpr, err := prepareExpression(MathDefaultW)
		require.NoError(t, err)
		legacyExpr, err := prepareExpression(tc.legacy)
		require.NoError(t, err)
		// At the full ring the precomputed factor is exactly 4.0 (coverageZP), so the new scalar-k
		// expression must reduce bit-for-bit to the legacy "mean(m,N) + 4.0*stddev(m,N)".
		gotNew, err := newExpr.Evaluate(map[string]interface{}{
			"m": ms,
			"n": float64(tc.n),
			"k": coverageZP,
		})
		require.NoError(t, err)
		gotLegacy, err := legacyExpr.Evaluate(map[string]interface{}{"m": ms})
		require.NoError(t, err)
		require.Equal(t, gotLegacy, gotNew)
	}
}
