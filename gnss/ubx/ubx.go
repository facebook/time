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

// Package ubx provides a thin Go wrapper around the upstream `cfgtool`
// CLI from the open-source ubloxcfg library (Philippe Kehl,
// https://oinkzwurgl.org/hacking/ubloxcfg/, EPEL package: ubloxcfg).
//
// The wrapper invokes `cfgtool dump` against the receiver, captures
// the receiver-detection banner that cfgtool emits within ~1 second
// on connect, and kills the subprocess as soon as the banner is seen.
// The banner contains firmware version, model, and detected baudrate:
//
//	cmd: rx0: Receiver detected at baudrate 115200: TIM 2.01 (ZED-F9T)
//
// cfgtool is configuration-only  it does NOT flash firmware. Flashing
// will be implemented in a sibling package (firmware/) once the
// strategy decision in T269402364 is made.
package ubx

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CfgtoolBinary is the absolute path of the upstream cfgtool binary
// installed by the EPEL `ubloxcfg` RPM.
const CfgtoolBinary = "/usr/bin/cfgtool"

// DefaultStatusTimeout is the wall-clock budget allowed for a single
// banner read against a live receiver.
const DefaultStatusTimeout = 10 * time.Second

// ErrBannerNotFound is returned when cfgtool exits without emitting
// a parseable receiver-detection banner.
var ErrBannerNotFound = errors.New("cfgtool receiver-detection banner not found")

// ErrPortLocked is returned when cfgtool fails to obtain an exclusive
// lock on the GNSS serial port  typically because oscillatord (or
// another time-card daemon) is holding it open. On production time
// servers this is the expected steady state. To read firmware
// in-band, stop oscillatord first or read from the firmware-version
// cache populated at boot before oscillatord starts.
var ErrPortLocked = errors.New("GNSS serial port is locked by another process (likely oscillatord)")

// portLockHints are substrings cfgtool emits on stderr when flock()
// or open(O_EXCL) fails on the serial device.
var portLockHints = []string{
	"Failed locking device",
	"Resource temporarily unavailable",
}

// bannerRegex matches the cfgtool receiver-detection banner, e.g.
//
//	status: rx0: Receiver detected at baudrate 115200: TIM 2.01 (ZED-F9T)
//	dump: rx0: Receiver detected at baudrate 9600: ROM SPG 5.10 (NEO-M9N)
//
// Capture groups: 1=baudrate, 2=firmware string, 3=model.
var bannerRegex = regexp.MustCompile(
	`Receiver detected at baudrate (\d+):\s*(.+?)\s*\((.+?)\)\s*$`,
)

// StatusInfo captures the fields parsed from the cfgtool
// receiver-detection banner.
type StatusInfo struct {
	// Firmware is the version string reported by the receiver, e.g.
	// "TIM 2.01" for the OCP TimeCard's u-blox F9T timing firmware.
	Firmware string
	// Model is the receiver model string, e.g. "ZED-F9T".
	Model string
	// Baudrate is the autobaud-detected serial baudrate, e.g. 115200.
	Baudrate int
}

// FirmwareVersion returns the firmware version string suitable for
// publication via ANR / SeRF. Returns ErrBannerNotFound if the field
// was never populated.
func (s *StatusInfo) FirmwareVersion() (string, error) {
	if s.Firmware == "" {
		return "", ErrBannerNotFound
	}
	return s.Firmware, nil
}

// Status invokes `cfgtool dump` against the given serial port,
// captures the receiver-detection banner, kills the subprocess once
// the banner is seen, and returns the parsed StatusInfo.
//
// The serialPort argument is the absolute /dev path
// (e.g. "/dev/ttyS4"); cfgtool requires a "ser://" prefix which is
// added automatically.
func Status(ctx context.Context, serialPort string) (*StatusInfo, error) {
	return statusWith(ctx, CfgtoolBinary, serialPort)
}

// statusWith is the testable variant of Status that allows injection
// of an alternate cfgtool binary path (e.g. a fake script in tests).
func statusWith(ctx context.Context, cfgtool, serialPort string) (*StatusInfo, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultStatusTimeout)
		defer cancel()
	}

	port := "ser://" + serialPort
	cmd := exec.CommandContext(ctx, cfgtool, "dump", "-p", port)

	// cfgtool prints the banner on stderr in practice; capture both
	// streams to be robust to upstream changes.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting cfgtool: %w", err)
	}

	info, scanErr := scanForBanner(stdout, stderr, cmd.Process.Kill)

	// Always wait on the process to reap it. Ignore the wait error:
	// we expect SIGKILL once the banner is seen, which produces a
	// non-zero exit. Only surface a wait error if we never found the
	// banner (the process likely exited on its own with an error).
	waitErr := cmd.Wait()
	if scanErr != nil {
		if waitErr != nil {
			return nil, fmt.Errorf("cfgtool dump -p %s: %w (exit: %w)", port, scanErr, waitErr)
		}
		return nil, fmt.Errorf("cfgtool dump -p %s: %w", port, scanErr)
	}
	return info, nil
}

// scanForBanner reads stdout and stderr concurrently line-by-line,
// returning as soon as either stream emits a line that matches
// bannerRegex. kill is invoked once a match is found so the cfgtool
// subprocess exits promptly. If both streams close without a match,
// the captured stderr-style diagnostic lines are inspected to
// identify common failure modes (notably port-lock conflicts) and
// surfaced via a wrapped error.
func scanForBanner(stdout, stderr io.Reader, kill func() error) (*StatusInfo, error) {
	results := make(chan *StatusInfo, 2)
	var wg sync.WaitGroup

	// Diagnostic buffers: collect non-banner lines from each stream
	// (capped) so we can surface useful context if no banner appears.
	const maxDiagLines = 32
	var diagMu sync.Mutex
	var diag []string

	appendDiag := func(line string) {
		diagMu.Lock()
		defer diagMu.Unlock()
		if len(diag) >= maxDiagLines {
			return
		}
		diag = append(diag, line)
	}

	scan := func(r io.Reader) {
		s := bufio.NewScanner(r)
		for s.Scan() {
			line := s.Text()
			if info, ok := parseBannerLine(line); ok {
				select {
				case results <- info:
				default:
				}
				return
			}
			appendDiag(line)
		}
	}
	wg.Go(func() { scan(stdout) })
	wg.Go(func() { scan(stderr) })

	// Drain the channel + wait for both scanners to finish in a
	// goroutine so the caller can return as soon as the first match
	// arrives.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case info := <-results:
		_ = kill() // best-effort: kill the subprocess
		<-done     // drain remaining scanners after kill
		return info, nil
	case <-done:
		return nil, classifyFailure(diag)
	}
}

// classifyFailure inspects the captured cfgtool diagnostic lines and
// returns the most actionable error available. Recognises:
//   - port-lock contention (likely oscillatord)  ErrPortLocked
//   - everything else  ErrBannerNotFound, with the captured
//     diagnostic lines included for debuggability.
func classifyFailure(diag []string) error {
	joined := strings.Join(diag, "\n")
	for _, hint := range portLockHints {
		if strings.Contains(joined, hint) {
			if joined == "" {
				return ErrPortLocked
			}
			return fmt.Errorf("%w: %s", ErrPortLocked, joined)
		}
	}
	if joined == "" {
		return ErrBannerNotFound
	}
	return fmt.Errorf("%w; cfgtool output:\n%s", ErrBannerNotFound, joined)
}

// parseBannerLine returns a populated StatusInfo if line matches the
// cfgtool receiver-detection banner, or (nil, false) otherwise.
func parseBannerLine(line string) (*StatusInfo, bool) {
	m := bannerRegex.FindStringSubmatch(line)
	if m == nil {
		return nil, false
	}
	baud, err := strconv.Atoi(m[1])
	if err != nil {
		return nil, false
	}
	return &StatusInfo{
		Firmware: strings.TrimSpace(m[2]),
		Model:    strings.TrimSpace(m[3]),
		Baudrate: baud,
	}, true
}
