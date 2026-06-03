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

// Package cmd hosts the gnssfw cobra root command and its subcommands.
package cmd

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/facebook/time/gnss/detect"
)

// RootCmd is the entry point for the gnssfw binary.
var RootCmd = &cobra.Command{
	Use:   "gnssfw",
	Short: "tools for the u-blox GNSS receiver on OCP TimeCards",
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
		if verbose {
			log.SetLevel(log.DebugLevel)
		}

		userSetSerial = serialPort != ""
		userSetPCI = pciAddr != ""
		if userSetSerial && userSetPCI {
			return fmt.Errorf("--serial and --pci are mutually exclusive")
		}

		switch {
		case userSetSerial:
			return nil // operator pinned the device explicitly
		case userSetPCI:
			var err error
			serialPort, err = detect.GNSSSerialFromPCI(pciAddr)
			if err != nil {
				return fmt.Errorf("resolving GNSS tty for pci=%s: %w", pciAddr, err)
			}
			return nil
		}

		var err error
		serialPort, err = detect.GNSSSerial()
		if err != nil {
			log.Warnf("GNSS serial auto-detect failed, using fallback %s: %v", detect.DefaultGNSSSerial, err)
			serialPort = detect.DefaultGNSSSerial
		}
		return nil
	},
}

// serialPort is the resolved serial device path used by all subcommands.
var serialPort string

// pciAddr is the PCI BDF (e.g. "0000:11:00.0") of the TimeCard whose GNSS
// receiver we want to address. Used to resolve the GNSS tty under
// /sys/bus/pci/devices/<bdf>/timecard/ocp*/tty/ttyGNSS  required for
// hosts with multiple TimeCards.
var pciAddr string

// userSetSerial is true when --serial was passed on the command line.
var userSetSerial bool

// userSetPCI is true when --pci was passed on the command line.
var userSetPCI bool

// verbose enables debug-level logging when set via --verbose / -v.
var verbose bool

func init() {
	RootCmd.CompletionOptions.DisableDefaultCmd = true
	RootCmd.PersistentFlags().StringVar(
		&serialPort,
		"serial",
		"",
		"GNSS serial port (e.g. /dev/ttyS4). If unset, auto-detected from "+detect.TTYGNSSPath+", falling back to "+detect.DefaultGNSSSerial+".",
	)
	RootCmd.PersistentFlags().StringVar(
		&pciAddr,
		"pci",
		"",
		"TimeCard PCI BDF (e.g. 0000:11:00.0). Resolves the GNSS tty via /sys/bus/pci/devices/<bdf>/timecard/ocp*/tty/ttyGNSS. Mutually exclusive with --serial.",
	)
	RootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable debug-level logging")
}

// Execute runs the root command.
func Execute() {
	log.SetLevel(log.InfoLevel)
	if err := RootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
