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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrepareExpression(t *testing.T) {
	_, err := prepareExpression("lkjdfkj")
	require.Error(t, err)
	_, err = prepareExpression("2 + 2")
	require.NoError(t, err)

	_, err = prepareExpression("mean(phcoffset)")
	require.NoError(t, err)

	_, err = prepareExpression("mean(missing)")
	require.Error(t, err)
}

func TestMath(t *testing.T) {
	e, err := prepareExpression("2 + 2")
	require.NoError(t, err)
	result, err := e.Evaluate(nil)
	require.NoError(t, err)
	require.Equal(t, 4.0, result.(float64))

	e, err = prepareExpression("max(2, 3)")
	require.NoError(t, err)
	result, err = e.Evaluate(nil)
	require.NoError(t, err)
	require.Equal(t, 3.0, result.(float64))

	e, err = prepareExpression("abs(-3)")
	require.NoError(t, err)
	result, err = e.Evaluate(nil)
	require.NoError(t, err)
	require.Equal(t, 3.0, result.(float64))

	e, err = prepareExpression("mean(phcoffset)")
	require.NoError(t, err)
	result, err = e.Evaluate(map[string]interface{}{"phcoffset": []float64{1, 2, 3, 4, 5}})
	require.NoError(t, err)
	require.Equal(t, 3.0, result.(float64))

	e, err = prepareExpression("stddev(phcoffset)")
	require.NoError(t, err)
	result, err = e.Evaluate(map[string]interface{}{"phcoffset": []float64{1, 2, 3, 4, 5}})
	require.NoError(t, err)
	require.InDelta(t, 1.5811, result.(float64), 0.001)

	e, err = prepareExpression("variance(phcoffset)")
	require.NoError(t, err)
	result, err = e.Evaluate(map[string]interface{}{"phcoffset": []float64{1, 2, 3, 4, 5}})
	require.NoError(t, err)
	require.Equal(t, 2.5, result.(float64))

	e, err = prepareExpression("p99(phcoffset)")
	require.NoError(t, err)
	result, err = e.Evaluate(map[string]interface{}{"phcoffset": []float64{1, 2, 3, 4, 5}})
	require.NoError(t, err)
	require.Equal(t, 5.0, result.(float64))
}

func FuzzPrepareExpression(f *testing.F) {

	f.Add("lkjdfkj")
	f.Add("2 + 2")
	f.Add("mean(phcoffset)")
	f.Add("max(2, 3)")
	f.Add("abs(-3)")
	f.Add("stddev(phcoffset)")
	f.Add("variance(phcoffset)")

	f.Fuzz(func(t *testing.T, input string) {
		prepareExpression(input)
	})
}
