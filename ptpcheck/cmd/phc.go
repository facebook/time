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

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/facebookincubator/ptp/phc"
)

// flag
var device string
var method string

func init() {
	RootCmd.AddCommand(phcCmd)
	phcCmd.Flags().StringVarP(&device, "device", "d", "/dev/ptp0", "PTP device to get time from")
	phcCmd.Flags().StringVarP(
		&method,
		"method",
		"m",
		string(phc.MethodIoctlSysOffsetExtended),
		fmt.Sprintf("Method to get PHC time: %v", phc.SupportedMethods),
	)
}

func printPHC(device string, method phc.TimeMethod) error {
	timeAndOffset, err := phc.TimeAndOffsetFromDevice(device, method)
	if err != nil {
		if method == phc.MethodSyscallClockGettime {
			return err
		}
		log.Warningf("Falling back to clock_gettime method: %v", err)
		timeAndOffset, err = phc.TimeAndOffsetFromDevice(device, phc.MethodSyscallClockGettime)
		if err != nil {
			return err
		}
	}
	fmt.Printf("PHC clock: %s\n", timeAndOffset.PHCTime)
	fmt.Printf("SYS clock: %s\n", timeAndOffset.SysTime)
	fmt.Printf("Offset: %s\n", timeAndOffset.Offset)
	fmt.Printf("Delay: %s\n", timeAndOffset.Delay)
	return nil
}

var phcCmd = &cobra.Command{
	Use:   "phc",
	Short: "Print PHC clock information. Use `phc_ctl` cli for richer functionality",
	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()

		if err := printPHC(device, phc.TimeMethod(method)); err != nil {
			log.Fatal(err)
		}
	},
}
