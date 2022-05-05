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

package clock

import (
	"fmt"
	"math"
	"sort"

	"github.com/Knetic/govaluate"
	"github.com/eclesh/welford"
)

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

func p99(input []float64) float64 {
	sort.Float64s(input)
	p1 := len(input) / 100 * 1
	return input[len(input)-1-p1]
}

// once oscillatord supports reporting offset we can add it here
var supportedVariables = []string{
	"phcoffset",
	"oscillatoroffset",
	"oscillatorclass",
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
	"max": func(args ...interface{}) (interface{}, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("max: wrong number of arguments: want 2, got %d", len(args))
		}
		val1 := args[0].(float64)
		val2 := args[1].(float64)
		return math.Max(val1, val2), nil
	},
	"mean": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("mean: wrong number of arguments: want 1, got %d", len(args))
		}
		vals := args[0].([]float64)
		return mean(vals), nil
	},
	"variance": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("variance: wrong number of arguments: want 1, got %d", len(args))
		}
		vals := args[0].([]float64)
		return variance(vals), nil
	},
	"stddev": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("stddev: wrong number of arguments: want 1, got %d", len(args))
		}
		vals := args[0].([]float64)
		return stddev(vals), nil
	},
	"p99": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("stddev: wrong number of arguments: want 1, got %d", len(args))
		}
		vals := args[0].([]float64)
		return p99(vals), nil
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
