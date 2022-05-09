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

package stats

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCountersToMap(t *testing.T) {
	c := counters{
		utcOffset:        1,
		phcOffset:        2,
		oscillatorOffset: 3,
		clockAccuracy:    42,
		clockClass:       6,
		reload:           7,
		dataError:        8,
	}
	result := c.toMap()

	expectedMap := make(map[string]int64)
	expectedMap["utcoffset"] = 1
	expectedMap["phcoffset"] = 2
	expectedMap["oscillatoroffset"] = 3
	expectedMap["clockaccuracy"] = 42
	expectedMap["clockclass"] = 6
	expectedMap["reload"] = 7
	expectedMap["dataerror"] = 8

	require.Equal(t, expectedMap, result)
}
