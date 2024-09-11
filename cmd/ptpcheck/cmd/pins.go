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

	"github.com/facebook/time/phc"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// flags
var devPath string
var pinName string
var pinFunc phc.PinFunc
var setMode bool

func init() {
	RootCmd.AddCommand(pinsCmd)
	pinsCmd.Flags().StringVarP(&devPath, "device", "d", "/dev/ptp0", "the PTP device")
	pinsCmd.Flags().StringVarP(&pinName, "name", "n", "", "name of the pin")
	pinsCmd.Flags().VarP(&pinFunc, "mode", "m", "the PTP function")
	pinsCmd.Flags().BoolVarP(&setMode, "set", "s", false, "set PTP function")
}

var pinsCmd = &cobra.Command{
	Use:   "pins",
	Short: "Print PHC pins and their functions",
	Run: func(_ *cobra.Command, _ []string) {
		ConfigureVerbosity()
		if setMode && pinName == "" {
			log.Fatal("-set needs -name")
		}
		if err := doListPins(devPath); err != nil {
			log.Fatal(err)
		}
	},
}

func doListPins(device string) error {
	// we may need RW permissions to issue PTP_SETFUNC ioctl on the device
	f, err := os.OpenFile(device, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("opening device %q: %w", device, err)
	}
	defer f.Close()
	dev := phc.FromFile(f)

	pins, err := dev.ReadPins()
	if err != nil {
		return err
	}
	for _, p := range pins {
		if setMode && pinName == p.Name {
			if err := p.SetFunc(pinFunc); err != nil {
				log.Fatal(err)
			}
		}
		if pinName == "" || pinName == p.Name {
			fmt.Printf("%s: pin %d function %-7[3]s (%[3]d) chan %d\n", p.Name, p.Index, p.Func, p.Chan)
		}
	}
	return nil
}
