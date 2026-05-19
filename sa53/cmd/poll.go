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
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/facebook/time/sa53/detect"
	"github.com/facebook/time/sa53/preflight"
	"github.com/facebook/time/sa53/protocol"
	"github.com/facebook/time/sa53/rawlog"
)

// defaultPollParams is the conservative V1.1+ set that works on every
// firmware version we ship. Operators on V1.5+ firmware can opt into the
// extra parameters (LaserTempSet, OscTuning, OvenCurrent, DCSignal) by
// passing --params explicitly.
var defaultPollParams = []string{
	"LockProgress",
	"EffectiveTuning",
	"DigitalTuning",
	"DisciplineTuning",
	"Temperature",
	"Alarms",
}

var (
	pollOut       string
	pollRaw       string
	pollParams    string
	pollInterval  time.Duration
	pollDuration  time.Duration
	pollSingleCmd string
)

// pollReadTimeout is what we set on the port before draining and entering
// the polling loop. Matches the bookmark.
const pollReadTimeout = 2 * time.Second

// maxConsecutiveAllErrors aborts the run when every parameter errors for
// this many ticks in a row. Catches chip resets, USB disconnects, or
// anything else stomping on us.
const maxConsecutiveAllErrors = 5

func init() {
	RootCmd.AddCommand(pollCmd)
	pollCmd.Flags().StringVar(&pollOut, "out", "", "CSV output path; empty = stdout (default)")
	pollCmd.Flags().StringVar(&pollRaw, "raw", "", "optional raw TX/RX log path; strongly recommended while validating the protocol")
	pollCmd.Flags().StringVar(&pollParams, "params", strings.Join(defaultPollParams, ","), "comma-separated SA53 parameters to poll")
	pollCmd.Flags().DurationVar(&pollInterval, "interval", time.Second, "polling interval (e.g. 100ms for 10Hz, 1s for 1Hz)")
	pollCmd.Flags().DurationVar(&pollDuration, "duration", 0, "total capture duration; 0 means run until interrupted")
	pollCmd.Flags().StringVar(&pollSingleCmd, "cmd", "",
		"send a single raw command and exit; bypasses polling. "+
			"Quote the value, e.g. --cmd '\\{swrev?}' "+
			"(single quotes, brace must be backslash-escaped or quoted to avoid shell expansion).")
	// --cmd is a one-shot raw probe; the polling-mode flags are silently
	// ignored if combined with it. Make that explicit.
	pollCmd.MarkFlagsMutuallyExclusive("cmd", "interval")
	pollCmd.MarkFlagsMutuallyExclusive("cmd", "duration")
	pollCmd.MarkFlagsMutuallyExclusive("cmd", "params")
	pollCmd.MarkFlagsMutuallyExclusive("cmd", "out")
}

var pollCmd = &cobra.Command{
	Use:   "poll",
	Short: "poll SA53 parameters into CSV, or send a single raw command",
	Long: "Polls a Microchip SA53 atomic clock over the serial port and writes\n" +
		"the requested parameters to a CSV file.\n\n" +
		"The serial device is shared with oscillatord on Time Card hosts and\n" +
		"Linux ttys are not exclusive. If oscillatord is running, both peers\n" +
		"will scribble on the same wire and produce garbage. Stop chef and\n" +
		"oscillatord first (chef is stopped via the standard chef-stop\n" +
		"procedure, oscillatord via `systemctl stop oscillatord`).\n\n" +
		"\tsa53fw poll --out sa53_capture.csv --interval 100ms --duration 5m\n\n" +
		"Pass --raw <path> to capture the exact bytes sent and received.",
	Run: func(_ *cobra.Command, _ []string) {
		if pollSingleCmd != "" {
			if err := runPollSingleCmd(serialPort, pollSingleCmd, pollRaw); err != nil {
				log.Fatal(err)
			}
			return
		}
		params := splitParams(pollParams)
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		if err := runPoll(ctx, serialPort, pollOut, pollRaw, params, pollInterval, pollDuration); err != nil {
			log.Fatal(err)
		}
	},
}

// splitParams trims and drops empty entries so ", ,Temperature" doesn't end
// up sending {get,} on the wire.
func splitParams(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// mark is a nil-safe wrapper around (*rawlog.Logger).Mark for callers that
// only optionally enable raw logging.
func mark(rl *rawlog.Logger, msg string) {
	if rl != nil {
		rl.Mark(msg)
	}
}

// detectSA5xCard runs hardware detection and returns true if at least one
// Celestica/SA5x time card is present. Returns false (with no error) when
// no SA5x card is found, so callers can exit cleanly. Skipped when the
// operator has pinned --serial.
func detectSA5xCard() (bool, error) {
	if userSetSerial {
		return true, nil
	}
	cards, err := detect.Timecards()
	if err != nil {
		return false, fmt.Errorf("cannot detect time cards: %w", err)
	}
	for _, c := range cards {
		if c.IsSA5x() {
			log.Infof("detected Celestica time card board_id=%s pci=%s", c.BoardID, c.PCIAddr)
			return true, nil
		}
	}
	log.Info("no Celestica/SA5x time cards found, skipping")
	return false, nil
}

func runPoll(ctx context.Context, device, outPath, rawPath string, params []string, interval, duration time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("interval must be > 0, got %s", interval)
	}
	if len(params) == 0 {
		return errors.New("no params specified")
	}
	//nolint:contextcheck // detect.Timecards uses its own bounded timeout; signature change is out of scope here
	ok, err := detectSA5xCard()
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if err := preflight.Default.Preflight(device); err != nil {
		return err
	}

	sa53, err := protocol.Init(device)
	if err != nil {
		return err
	}
	defer sa53.Close()

	var rl *rawlog.Logger
	if rawPath != "" {
		f, err := os.Create(rawPath)
		if err != nil {
			return fmt.Errorf("create raw log %s: %w", rawPath, err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				log.Errorf("close raw log %s: %v", rawPath, err)
			}
		}()
		rl = rawlog.NewLogger(f)
		defer rl.Close()
		sa53.WrapPort(func(p protocol.SerialReadWriter) protocol.SerialReadWriter {
			return rawlog.NewLoggingPort(p, rl)
		})
	}

	if err := sa53.SetReadTimeout(pollReadTimeout); err != nil {
		return fmt.Errorf("set read timeout: %w", err)
	}

	mark(rl, "drain pre-start")
	sa53.Drain()

	var out io.Writer = os.Stdout
	if outPath != "" {
		f, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("create %s: %w", outPath, err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				log.Errorf("close csv %s: %v", outPath, err)
			}
		}()
		out = f
	}

	w := csv.NewWriter(out)
	defer w.Flush()

	header := append([]string{"ts_iso", "ts_ns"}, params...)
	_ = w.Write(header)
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	if duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, duration)
		defer cancel()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	count := 0
	consecutiveAllErrors := 0
	start := time.Now()

	writeRow := func() error {
		mark(rl, fmt.Sprintf("sample %d", count+1))
		row, allErrored := pollOnce(sa53, params, rl)
		_ = w.Write(row)
		w.Flush()
		if err := w.Error(); err != nil {
			return fmt.Errorf("write row: %w", err)
		}
		count++
		if allErrored {
			consecutiveAllErrors++
			if consecutiveAllErrors >= maxConsecutiveAllErrors {
				return fmt.Errorf("aborting: %d consecutive ticks where every parameter errored", consecutiveAllErrors)
			}
		} else {
			consecutiveAllErrors = 0
		}
		if count%10 == 0 {
			log.Infof("wrote samples count=%d elapsed_s=%.3f", count, time.Since(start).Seconds())
		}
		return nil
	}

	if err := writeRow(); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			dest := outPath
			if dest == "" {
				dest = "stdout"
			}
			log.Infof("capture complete samples=%d out=%s", count, dest)
			return nil
		case <-ticker.C:
			if err := writeRow(); err != nil {
				return err
			}
		}
	}
}

// pollOnce gathers all params for a single tick. Returns the CSV row
// (ISO timestamp, ns timestamp, then each value or "ERR:<msg>") and
// allErrored=true if every parameter failed.
func pollOnce(sa53 *protocol.Mac, params []string, rl *rawlog.Logger) (row []string, allErrored bool) {
	row = make([]string, 0, len(params)+2)
	now := time.Now()
	row = append(row, now.UTC().Format(time.RFC3339Nano))
	row = append(row, strconv.FormatInt(now.UnixNano(), 10))
	errCount := 0
	for _, p := range params {
		mark(rl, "get "+p)
		v, err := sa53.Get(p)
		if err != nil {
			row = append(row, "ERR:"+err.Error())
			errCount++
			// Drain only when bytes might still be in flight. A clean
			// device error ([!N]) means the response is complete.
			if !errors.Is(err, protocol.ErrDeviceError) {
				sa53.Drain()
			}
			continue
		}
		row = append(row, v)
	}
	return row, errCount == len(params)
}

// runPollSingleCmd opens the port, sends a single raw command, prints the
// response, and returns. Useful for one-off probes like \{swrev?} for
// firmware version. If rawPath is non-empty, raw TX/RX bytes are tee'd to
// that file just like the polling path.
func runPollSingleCmd(device, raw, rawPath string) error {
	ok, err := detectSA5xCard()
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if err := preflight.Default.Preflight(device); err != nil {
		return err
	}
	sa53, err := protocol.Init(device)
	if err != nil {
		return err
	}
	defer sa53.Close()

	var rl *rawlog.Logger
	if rawPath != "" {
		f, err := os.Create(rawPath)
		if err != nil {
			return fmt.Errorf("create raw log %s: %w", rawPath, err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				log.Errorf("close raw log %s: %v", rawPath, err)
			}
		}()
		rl = rawlog.NewLogger(f)
		defer rl.Close()
		sa53.WrapPort(func(p protocol.SerialReadWriter) protocol.SerialReadWriter {
			return rawlog.NewLoggingPort(p, rl)
		})
	}

	if err := sa53.SetReadTimeout(pollReadTimeout); err != nil {
		return fmt.Errorf("set read timeout: %w", err)
	}
	mark(rl, "cmd "+raw)
	resp, err := sa53.Cmd(raw)
	if err != nil {
		return err
	}
	val, perr := protocol.ParseValue(resp)
	if perr == nil {
		fmt.Println(val)
		return nil
	}
	if errors.Is(perr, protocol.ErrDeviceError) {
		log.Fatalf("device returned error: %v", perr)
	}
	// Non-device parse failure: surface the raw response so the operator
	// can decode it themselves.
	fmt.Println(resp)
	return nil
}
