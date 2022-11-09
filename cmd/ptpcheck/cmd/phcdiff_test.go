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

package cmd

import (
	"testing"
	"time"

	"github.com/facebook/time/phc"
	"github.com/stretchr/testify/require"
)

func TestCalcDiffPhcs(t *testing.T) {
	rawA := phc.SysoffResult{
		Offset:  time.Duration(37),
		Delay:   time.Duration(250),
		SysTime: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
		PHCTime: time.Date(2021, 1, 1, 0, 0, 37, 10, time.UTC),
	}
	rawB := phc.SysoffResult{
		Offset:  time.Duration(37),
		Delay:   time.Duration(1250),
		SysTime: time.Date(2021, 1, 1, 0, 0, 0, 380, time.UTC),
		PHCTime: time.Date(2021, 1, 1, 0, 0, 37, 395, time.UTC),
	}
	phcOffset, delay1, delay2 := calcDiff(rawA, rawB)

	require.Equal(t, phcOffset, time.Duration(5))
	require.Equal(t, delay1, time.Duration(250))
	require.Equal(t, delay2, time.Duration(1250))
}
