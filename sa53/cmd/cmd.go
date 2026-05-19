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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const ttyMACPath = "/sys/class/timecard/ocp0/tty/ttyMAC"
const ttyMACFallback = "/dev/ttyS6"

// RootCmd is the entry point for the sa53 binary.
var RootCmd = &cobra.Command{
	Use:   "sa53",
	Short: "tools for the Microchip SA53 atomic clock on Celestica time cards",
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
		if verbose {
			log.SetLevel(log.DebugLevel)
		}
		userSetSerial = serialPort != ""
		if userSetSerial {
			return nil // user passed --serial explicitly
		}
		detected, err := detectMACSerial()
		if err != nil {
			log.Warnf("MAC serial auto-detect failed, using fallback %s: %v", ttyMACFallback, err)
			serialPort = ttyMACFallback
			return nil
		}
		serialPort = detected
		return nil
	},
}

// serialPort is the shared serial device path used by all subcommands.
var serialPort string

// userSetSerial is true when --serial was passed on the command line.
// Subcommands key off this to decide whether to run hardware detection
// (skipped when the operator has already pinned the device).
var userSetSerial bool

// verbose enables debug-level logging when set via --verbose / -v.
var verbose bool

func init() {
	RootCmd.CompletionOptions.DisableDefaultCmd = true
	RootCmd.PersistentFlags().StringVar(
		&serialPort,
		"serial",
		"",
		"SA53 serial port. If unset, auto-detected from "+ttyMACPath+", falling back to "+ttyMACFallback+".",
	)
	RootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable debug-level logging")
}

// detectMACSerial reads the MAC serial port device name from sysfs.
// Returns the full device path (e.g. "/dev/ttyS5") or an error if detection fails.
func detectMACSerial() (string, error) {
	data, err := os.ReadFile(ttyMACPath)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", ttyMACPath, err)
	}
	ttyName := strings.TrimSpace(string(data))
	if ttyName == "" {
		return "", fmt.Errorf("empty tty device name in %s", ttyMACPath)
	}
	return filepath.Join("/dev", ttyName), nil
}

// Execute runs the root command.
func Execute() {
	log.SetLevel(log.InfoLevel)
	if err := RootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
