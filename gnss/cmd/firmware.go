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
	"encoding/json"
	"errors"
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/facebook/time/gnss/ubx"
)

var (
	fwFile  string
	upgrade bool
	force   bool
	check   bool
)

func init() {
	RootCmd.AddCommand(firmwareCmd)
	firmwareCmd.Flags().StringVar(&fwFile, "fw", "", "GNSS firmware file (NOT YET IMPLEMENTED)")
	firmwareCmd.Flags().BoolVar(&upgrade, "upgrade", false, "Apply the firmware upgrade (NOT YET IMPLEMENTED)")
	firmwareCmd.Flags().BoolVar(&force, "force", false, "Force firmware upgrade")
	firmwareCmd.Flags().BoolVar(&check, "check", false, "Check firmware version only (JSON output)")
}

// checkResult is the JSON contract emitted to stdout in --check mode.
// Field names and casing must remain stable: SysInspector / ANR
// timecard.py consume this verbatim.
type checkResult struct {
	Firmware string `json:"firmware"`
	Model    string `json:"model"`
	Baudrate int    `json:"baudrate"`
}

var firmwareCmd = &cobra.Command{
	Use:   "firmware",
	Short: "read or upgrade GNSS receiver firmware",
	RunE: func(_ *cobra.Command, _ []string) error {
		return runFirmware()
	},
}

func runFirmware() error {
	log.Infof("requesting firmware version from %s", serialPort)
	status, err := ubx.Status(context.Background(), serialPort)
	if err != nil {
		if errors.Is(err, ubx.ErrPortLocked) {
			log.Warn("hint: stop oscillatord (`systemctl stop oscillatord`) before running --check,")
			log.Warn("      or read the firmware-version cache populated at boot. See T269402318.")
		}
		return err
	}
	if _, err := status.FirmwareVersion(); err != nil {
		return err
	}
	log.Infof("GNSS receiver: %s (%s) @ %d baud", status.Firmware, status.Model, status.Baudrate)

	// --check mode: print JSON on stdout and return. All progress lines
	// above were emitted via logrus to stderr to keep stdout
	// machine-parseable.
	if check {
		cr := checkResult{
			Firmware: status.Firmware,
			Model:    status.Model,
			Baudrate: status.Baudrate,
		}
		data, err := json.Marshal(cr)
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Upgrade path  not yet implemented. The cfgtool binary is
	// configuration-only and does not flash firmware. The flashing
	// strategy (live OS vs Grasstile, ubxfwupdate vs in-band UBX-UPD-SOS)
	// is being decided in T269402364.
	if upgrade || fwFile != "" {
		_ = force // reserved for future upgrade path
		return fmt.Errorf("firmware upgrade is not yet implemented (see T269402364)")
	}

	log.Warn("no action specified; pass --check for JSON version output")
	return nil
}
