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

package preflight

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// makeFakeProc builds a temporary directory shaped like /proc, with the given
// processes each holding the supplied list of file descriptors. fds are
// expressed as symlink targets so we can simulate any path the real kernel
// might expose.
func makeFakeProc(t *testing.T, byPid map[int]map[string]string, names map[int]string) string {
	t.Helper()
	root := t.TempDir()
	for pid, fds := range byPid {
		fdDir := filepath.Join(root, strconv.Itoa(pid), "fd")
		require.NoError(t, os.MkdirAll(fdDir, 0o755))
		for fdName, target := range fds {
			require.NoError(t, os.Symlink(target, filepath.Join(fdDir, fdName)))
		}
		if name, ok := names[pid]; ok {
			require.NoError(t, os.WriteFile(
				filepath.Join(root, strconv.Itoa(pid), "comm"),
				[]byte(name+"\n"), 0o644))
		}
	}
	return root
}

func TestHoldersDetectsLiteralPath(t *testing.T) {
	root := makeFakeProc(t, map[int]map[string]string{
		111: {"3": "/dev/ttyS6"},
		222: {"3": "/dev/null"},
	}, nil)
	s := Scanner{ProcRoot: root}
	got, err := s.Holders("/dev/ttyS6")
	require.NoError(t, err)
	require.ElementsMatch(t, []int{111}, got)
}

func TestHoldersIgnoresSelf(t *testing.T) {
	self := os.Getpid()
	root := makeFakeProc(t, map[int]map[string]string{
		self: {"3": "/dev/ttyS6"},
		333:  {"3": "/dev/ttyS6"},
	}, nil)
	s := Scanner{ProcRoot: root}
	got, err := s.Holders("/dev/ttyS6")
	require.NoError(t, err)
	require.ElementsMatch(t, []int{333}, got)
}

func TestHoldersSkipsPidsWithoutFdDir(t *testing.T) {
	// A pid directory without an "fd" subdirectory shouldn't crash the scan.
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "444"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "555", "fd"), 0o755))
	require.NoError(t, os.Symlink("/dev/ttyS6", filepath.Join(root, "555", "fd", "1")))
	s := Scanner{ProcRoot: root}
	got, err := s.Holders("/dev/ttyS6")
	require.NoError(t, err)
	require.ElementsMatch(t, []int{555}, got)
}

func TestPreflightSucceedsWhenFree(t *testing.T) {
	root := makeFakeProc(t, map[int]map[string]string{
		111: {"3": "/dev/null"},
	}, nil)
	s := Scanner{ProcRoot: root}
	require.NoError(t, s.Preflight("/dev/ttyS6"))
}

func TestPreflightErrorsWhenHeldAndNamesProcess(t *testing.T) {
	root := makeFakeProc(t,
		map[int]map[string]string{777: {"3": "/dev/ttyS6"}},
		map[int]string{777: "oscillatord"},
	)
	s := Scanner{ProcRoot: root}
	err := s.Preflight("/dev/ttyS6")
	require.Error(t, err)
	require.Contains(t, err.Error(), "777")
	require.Contains(t, err.Error(), "oscillatord")
	require.Contains(t, err.Error(), "chef") // hint about config-management restart
}
