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

package render

import (
	"bytes"
	"context"
	"testing"

	"github.com/olekukonko/tablewriter/tw"
	"github.com/stretchr/testify/require"
)

func TestTable(t *testing.T) {
	var buf bytes.Buffer
	table := Table(&buf, []tw.Align{tw.AlignLeft, tw.AlignRight}, "ADDRESS", "OFFSET")
	require.NoError(t, table.Append([]string{"2401:db00::1", "39"}))
	require.NoError(t, table.Render())

	out := buf.String()
	require.Contains(t, out, "ADDRESS", "headers are not reformatted")
	require.Contains(t, out, "+----", "ASCII rendition")
	require.Contains(t, out, "| 2401:db00::1")
}

func TestAddrKeepsAddress(t *testing.T) {
	require.Equal(t, "2001:db8::1", Addr(t.Context(), "2001:db8::1", true), "resolution disabled")
	require.Equal(t, "2001:db8::1", Addr(t.Context(), "2001:db8::1", false), "no PTR record")
}

// A resolver that never answers must not hold a command open.
func TestAddrRespectsDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.Equal(t, "2001:db8::1", Addr(ctx, "2001:db8::1", false))
}
