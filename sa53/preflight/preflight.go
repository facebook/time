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

// Package preflight refuses to start a poll session when another process
// already has the SA53 tty open. /dev/ttyS6 is shared with oscillatord on
// Time Card hosts and Linux ttys are not exclusive: if both peers scribble
// on the wire we get garbage. Surfaces who's holding it (PID + comm) so the
// operator can stop the right thing — including the config-management daemon
// that might restart it.
package preflight

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

// Scanner finds processes that have a given device file open. Defaults to
// scanning /proc; tests substitute a temporary tree.
type Scanner struct {
	ProcRoot string
}

// Default is the production Scanner pointed at the real /proc.
var Default = Scanner{ProcRoot: "/proc"}

// Holders returns the PIDs of processes whose open file descriptors point at
// device. Both the literal path and any symlink target are matched.
// Per-pid read errors are ignored since /proc is racy.
func (s Scanner) Holders(device string) ([]int, error) {
	target, _ := filepath.EvalSymlinks(device)
	if target == "" {
		target = device
	}

	entries, err := os.ReadDir(s.ProcRoot)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", s.ProcRoot, err)
	}

	self := os.Getpid()
	var holders []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		fdDir := filepath.Join(s.ProcRoot, e.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue // process may have exited or we lack permission
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if link == device || link == target {
				holders = append(holders, pid)
				break
			}
		}
	}
	return holders, nil
}

// ProcessName reads /proc/<pid>/comm. Empty string on failure.
func (s Scanner) ProcessName(pid int) string {
	data, err := os.ReadFile(filepath.Join(s.ProcRoot, strconv.Itoa(pid), "comm"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// Preflight refuses to proceed if another process is holding the device. A
// scan failure is fatal: returning the error lets the caller bail rather
// than charging ahead and producing garbage when the wire might still be
// contended.
func (s Scanner) Preflight(device string) error {
	holders, err := s.Holders(device)
	if err != nil {
		return fmt.Errorf("preflight scan failed: proc_root=%s: %w", s.ProcRoot, err)
	}
	if len(holders) == 0 {
		return nil
	}
	parts := make([]string, 0, len(holders))
	oscillatordHolder := false
	for _, pid := range holders {
		name := s.ProcessName(pid)
		if name == "oscillatord" {
			oscillatordHolder = true
		}
		if name == "" {
			parts = append(parts, strconv.Itoa(pid))
			continue
		}
		parts = append(parts, fmt.Sprintf("%d (%s)", pid, name))
	}
	if oscillatordHolder {
		log.Warn("stopping oscillatord will take this host offline as a PTP grandmaster " +
			"for the duration of the capture; ensure quorum/failover before proceeding")
	}
	return fmt.Errorf("device %s is already open by: %s; stop those processes "+
		"(and any service that would restart them, e.g. chef) before retrying",
		device, strings.Join(parts, ", "))
}
