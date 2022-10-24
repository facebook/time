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

	"github.com/stretchr/testify/assert"
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
	assert.InEpsilonSlice(t, want1, got1, 1e+10)

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
	assert.InEpsilonSlice(t, want2, got2, 1e+10)
}

func TestMean(t *testing.T) {
	input := []float64{3, 5, 8, 8}
	want := 6.0
	assert.Equal(t, want, mean(input))
	input = []float64{1, 4, 0, 3, 8}
	want = 3.2
	assert.Equal(t, want, mean(input))
}

func TestVariance(t *testing.T) {
	input := []float64{8, 8, 8, 8}
	want := 0.0
	assert.Equal(t, want, variance(input))
	input = []float64{1, 4, 0, 3, 8}
	want = 9.7
	assert.Equal(t, want, variance(input))
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
	assert.Equal(t, want, got)
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

	_, err = expr.Evaluate(parameters)
	require.Error(t, err)
}
