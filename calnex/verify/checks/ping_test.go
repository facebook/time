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
	"errors"
	"net"
	"os"
	"runtime/debug"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/icmp"
)

const leakCheckRuns = 20

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

// TestPingRunReleasesSocket pins the socket lifetime of Run. The calnex service
// pings every device on every run and never exits, so a socket left for the
// garbage collector to finalize accumulates between collections.
func TestPingRunReleasesSocket(t *testing.T) {
	probe, err := icmp.ListenPacket("udp6", "::")
	if err != nil {
		t.Skipf("ICMPv6 sockets are not available here: %v", err)
	}
	require.NoError(t, probe.Close())

	// net puts a finalizer on every socket, so a collection during the count
	// would close the descriptors this test exists to notice.
	defer debug.SetGCPercent(debug.SetGCPercent(-1))

	// An IPv4 literal on an IPv6-only ICMP socket fails at the write, which is
	// past the point where the socket exists. Assert that, so this stops
	// silently measuring nothing if Run ever rejects the target sooner.
	c := Ping{}
	opErr, ok := errors.AsType[*net.OpError](c.Run("1.2.3.4", false))
	require.True(t, ok)
	require.Equal(t, "write", opErr.Op)

	before := openDescriptors(t)
	for range leakCheckRuns {
		require.Error(t, c.Run("1.2.3.4", false))
	}

	// Comparing the set rather than the count: a descriptor opened before this
	// test and closed during the loop would otherwise offset a leak of the
	// same size and let a reverted fix pass.
	var leaked []string
	for fd := range openDescriptors(t) {
		if !before[fd] {
			leaked = append(leaked, fd)
		}
	}
	slices.Sort(leaked)
	require.Empty(t, leaked, "%d calls to Run leaked descriptors", leakCheckRuns)
}

// openDescriptors reports the process's open descriptors by number. Absent
// procfs it skips rather than fails: this package is mirrored to
// github.com/facebook/time, where the tests also run off Linux.
func openDescriptors(t *testing.T) map[string]bool {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skipf("/proc/self/fd is unavailable here: %v", err)
	}
	open := make(map[string]bool, len(entries))
	for _, entry := range entries {
		open[entry.Name()] = true
	}
	return open
}
