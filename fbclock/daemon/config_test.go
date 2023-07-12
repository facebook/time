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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEvalAndValidate(t *testing.T) {
	c := &Config{
		LinearizabilityTestInterval:    -1 * time.Second,
		LinearizabilityTestMaxGMOffset: -1 * time.Second,
		Math: Math{
			M:     "1",
			W:     "1",
			Drift: "1",
		},
	}
	require.Equal(t, fmt.Errorf("bad config: 'ptpclientaddress'"), c.EvalAndValidate())

	c.PTPClientAddress = "some address"
	require.Equal(t, fmt.Errorf("bad config: 'ringsize' must be >0"), c.EvalAndValidate())

	c.RingSize = 42
	require.Equal(t, fmt.Errorf("bad config: 'interval' must be between 0 and 1 minute"), c.EvalAndValidate())

	c.Interval = 1 * time.Microsecond
	require.Equal(t, fmt.Errorf("bad config: 'test interval' must be positive"), c.EvalAndValidate())

	c.LinearizabilityTestInterval = 1 * time.Microsecond
	require.Equal(t, fmt.Errorf("bad config: 'offset' must be positive"), c.EvalAndValidate())

	c.LinearizabilityTestMaxGMOffset = 1 * time.Microsecond
	require.Nil(t, c.EvalAndValidate())
}
