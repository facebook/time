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
	"math"

	"github.com/Knetic/govaluate"
	"github.com/eclesh/welford"
)

// MathHelp is a help message used by flags in main
const MathHelp = `When composing the -m and -w formulas, here is what you can do:
supported operations:
  evaluation is done with govaluate, please check https://github.com/Knetic/govaluate/blob/master/MANUAL.md
supported variables:
  offset (list of last offsets from GM, in ns)
  delay (list of last path delays, in ns)
  freq (list of last frequency adjustments, in PPM)
  clockaccuracy (list of clock accuracy values received from GM)
  freqchange (list of last changes in frequency)
  freqchangeabs (list of last changes in frequency, abs values)
supported functions:
  abs(value) - absolute value of single float64, for example abs(-1) = 1
  mean(values, number) - mean of list of 'number' values, for example mean(offset, 10) will take 10 elements from array 'offset' and return mean for those values
  variance(values, number) - variance of list of 'number' values, for example variance(offset, 10) will take 10 elements from array 'offset' and return variance for those values
  stddev(values, number) - standard deviation of list of 'number' values, for example stddev(offset, 10) will take 10 elements from array 'offset' and return standard deviation for those values`

const (
	// MathDefaultHistory is a default number of samples to keep
	MathDefaultHistory = 100
	// MathDefaultM is a default formula to calculate M
	MathDefaultM = "mean(clockaccuracy, 100) + abs(mean(offset, 100)) + 1.0 * stddev(offset, 100)"
	// MathDefaultW is a default formula to calculate W
	MathDefaultW = "mean(m, 100) + 4.0 * stddev(m, 100)"
	// MathDefaultDrift is a default formula to calculate default drift
	MathDefaultDrift = "1.5 * mean(freqchangeabs, 99)"
)

// Math stores our math expressions for M ans W values in two forms: string and parsed
type Math struct {
	M         string // Measurement, our value for clock quality
	mExpr     *govaluate.EvaluableExpression
	W         string // window of uncertainty, without adjustment for potential holdover
	wExpr     *govaluate.EvaluableExpression
	Drift     string // drift in PPB, for holdover multiplier calculations
	driftExpr *govaluate.EvaluableExpression
}

// Prepare will prepare all math expressions
func (m *Math) Prepare() error {
	var err error
	m.mExpr, err = prepareExpression(m.M)
	if err != nil {
		return fmt.Errorf("evaluating M: %w", err)
	}
	m.wExpr, err = prepareExpression(m.W)
	if err != nil {
		return fmt.Errorf("evaluating W: %w", err)
	}
	m.driftExpr, err = prepareExpression(m.Drift)
	if err != nil {
		return fmt.Errorf("evaluating Drift: %w", err)
	}
	return nil
}

func mean(input []float64) float64 {
	s := welford.New()
	for _, v := range input {
		s.Add(v)
	}
	return s.Mean()
}

func variance(input []float64) float64 {
	s := welford.New()
	for _, v := range input {
		s.Add(v)
	}
	return s.Variance()
}

func stddev(input []float64) float64 {
	s := welford.New()
	for _, v := range input {
		s.Add(v)
	}
	return s.Stddev()
}

// convolve mixes two signals together
func convolve(input, coeffs []float64) ([]float64, error) {
	if len(input) < len(coeffs) {
		return nil, fmt.Errorf("not enough values")
	}

	output := make([]float64, len(input))
	for i := range len(coeffs) {
		var sum float64

		for j := range i + 1 {
			sum += (input[j] * coeffs[len(coeffs)-(1+i-j)])
		}
		output[i] = sum
	}

	for i := len(coeffs); i < len(input); i++ {
		var sum float64
		for j := range len(coeffs) {
			sum += (input[i-j] * coeffs[j])
		}
		output[i] = sum
	}

	return output, nil
}

var supportedVariables = []string{
	"offset",
	"delay",
	"freq",
	"m",
	"clockaccuracy",
	"freqchange",
	"freqchangeabs",
}

func isSupportedVar(varName string) bool {
	for _, v := range supportedVariables {
		if v == varName {
			return true
		}
	}
	return false
}

// all the functions we support in expressions
var functions = map[string]govaluate.ExpressionFunction{
	"abs": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("abs: wrong number of arguments: want 1, got %d", len(args))
		}
		val := args[0].(float64)
		return math.Abs(val), nil
	},
	"mean": func(args ...interface{}) (interface{}, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("mean: wrong number of arguments: want 2, got %d", len(args))
		}
		vals := args[0].([]float64)
		nSamples := int(args[1].(float64))
		if len(vals) < nSamples {
			return mean(vals), nil
		}
		return mean(vals[:nSamples]), nil
	},
	"variance": func(args ...interface{}) (interface{}, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("variance: wrong number of arguments: want 2, got %d", len(args))
		}
		vals := args[0].([]float64)
		nSamples := int(args[1].(float64))
		if len(vals) < nSamples {
			return variance(vals), nil
		}
		return variance(vals[:nSamples]), nil
	},
	"stddev": func(args ...interface{}) (interface{}, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("stddev: wrong number of arguments: want 2, got %d", len(args))
		}
		vals := args[0].([]float64)
		nSamples := int(args[1].(float64))
		if len(vals) < nSamples {
			return stddev(vals), nil
		}
		return stddev(vals[:nSamples]), nil
	},
}

func prepareExpression(exprStr string) (*govaluate.EvaluableExpression, error) {
	expr, err := govaluate.NewEvaluableExpressionWithFunctions(exprStr, functions)
	if err != nil {
		return nil, err
	}
	for _, v := range expr.Vars() {
		if !isSupportedVar(v) {
			return nil, fmt.Errorf("unsupported variable %q", v)
		}
	}
	return expr, nil
}

func prepareMathParameters(lastN []*DataPoint) map[string][]float64 {
	size := len(lastN)
	offsets := make([]float64, size)
	delays := make([]float64, size)
	freqs := make([]float64, size)
	clockAccuracies := make([]float64, size)
	freqChanges := make([]float64, size-1)
	freqChangesAbs := make([]float64, size-1)
	prev := lastN[0]
	for i := range size {
		offsets[i] = lastN[i].MasterOffsetNS
		delays[i] = lastN[i].PathDelayNS
		freqs[i] = lastN[i].FreqAdjustmentPPB
		clockAccuracies[i] = lastN[i].ClockAccuracyNS
		if i != 0 {
			freqChanges[i-1] = lastN[i].FreqAdjustmentPPB - prev.FreqAdjustmentPPB
			freqChangesAbs[i-1] = math.Abs(lastN[i].FreqAdjustmentPPB - prev.FreqAdjustmentPPB)
		}
		prev = lastN[i]
	}
	return map[string][]float64{
		"offset":        offsets,
		"delay":         delays,
		"freq":          freqs,
		"clockaccuracy": clockAccuracies,
		"freqchange":    freqChanges,
		"freqchangeabs": freqChangesAbs,
	}
}

func mapOfInterface(m map[string][]float64) map[string]interface{} {
	mm := make(map[string]interface{}, len(m))
	for k, v := range m {
		mm[k] = v
	}
	return mm
}
