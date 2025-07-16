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

package verify

import (
	"context"
	"testing"

	"github.com/facebook/time/calnex/verify/checks"
	"github.com/stretchr/testify/require"
)

func TestVerify(t *testing.T) {
	v := &VF{Checks: []checks.Check{
		&checks.Ping{Remediation: checks.PingRemediation{}},
	}}

	ctx := context.Background()
	err := Verify(ctx, "localhost", false, v, true)
	require.NoError(t, err)

	err = Verify(ctx, "1.2.3.4", false, v, true)
	require.NoError(t, err)
}
