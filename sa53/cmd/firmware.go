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
	"errors"
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/facebook/time/sa53/upgrade"
)

var (
	fwFile string
	apply  bool
	force  bool
)

func init() {
	RootCmd.AddCommand(firmwareCmd)
	firmwareCmd.Flags().StringVar(&fwFile, "fw", "", "SA53 new firmware file")
	firmwareCmd.Flags().BoolVar(&apply, "apply", false, "apply the firmware upgrade")
	firmwareCmd.Flags().BoolVar(&force, "force", false, "Force firmware upgrade")
}

var firmwareCmd = &cobra.Command{
	Use:   "firmware",
	Short: "read or upgrade SA53 firmware",
	Run: func(_ *cobra.Command, _ []string) {
		if err := runFirmware(); err != nil {
			log.Fatal(err)
		}
	},
}

func runFirmware() error {
	if fwFile == "" {
		return fmt.Errorf("firmware file name must be provided via --fw")
	}

	err := upgrade.Apply(serialPort, upgrade.LocalSource{FilePath: fwFile}, apply, force)
	switch {
	case errors.Is(err, upgrade.ErrNoCard):
		log.Info("no Celestica/SA5x time cards found, skipping")
		return nil
	case errors.Is(err, upgrade.ErrUpToDate):
		log.Info(err.Error())
		return nil
	default:
		return err
	}
}
