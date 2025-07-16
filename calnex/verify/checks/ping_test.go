//go:build !race
// +build !race

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

package checks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPingError(t *testing.T) {
	r := PingRemediation{}
	c := Ping{Remediation: r}
	require.Equal(t, "Ping", c.Name())

	err := c.Run("1.2.3.4", false)
	require.Error(t, err)

	ctx := context.Background()
	want, _ := r.Remediate(ctx)
	got, err := c.Remediate(ctx)
	require.NoError(t, err)
	require.Equal(t, want, got)
}
