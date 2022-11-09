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
	"encoding/json"
	"fmt"
	"time"

	"github.com/facebook/time/phc"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type phcStats struct {
	PHCOffset time.Duration `json:"ptp.phc.offset_ns"`
	PHC1Delay time.Duration `json:"ptp.phc.1.delay_ns"`
	PHC2Delay time.Duration `json:"ptp.phc.2.delay_ns"`
}

var (
	phcDiffDeviceA string
	phcDiffDeviceB string
	phcDiffIsJSON  bool
)

func init() {
	RootCmd.AddCommand(phcdiffCmd)
	phcdiffCmd.Flags().StringVarP(&phcDiffDeviceA, "deviceA", "a", "/dev/ptp0", "First PHC device")
	phcdiffCmd.Flags().StringVarP(&phcDiffDeviceB, "deviceB", "b", "/dev/ptp2", "Second PHC device")
	phcdiffCmd.Flags().BoolVarP(&phcDiffIsJSON, "json", "j", false, "produce json output")
}

func calcDiff(timeAndOffsetA, timeAndOffsetB phc.SysoffResult) (PHCDiff time.Duration, PHC1Delay time.Duration, PHC2Delay time.Duration) {
	sysOffset := timeAndOffsetB.SysTime.Sub(timeAndOffsetA.SysTime)
	phcOffset := timeAndOffsetB.PHCTime.Sub(timeAndOffsetA.PHCTime)
	phcOffset -= sysOffset

	return phcOffset, timeAndOffsetA.Delay, timeAndOffsetB.Delay
}

func phcdiffRun(deviceA, deviceB string, isJSON bool) error {
	timeAndOffsetA, err := phc.TimeAndOffsetFromDevice(deviceA, phc.MethodIoctlSysOffsetExtended)
	if err != nil {
		return err
	}

	timeAndOffsetB, err := phc.TimeAndOffsetFromDevice(deviceB, phc.MethodIoctlSysOffsetExtended)
	if err != nil {
		return err
	}

	phcOffset, delay1, delay2 := calcDiff(timeAndOffsetA, timeAndOffsetB)

	if isJSON {
		stats := phcStats{PHCOffset: phcOffset, PHC1Delay: delay1, PHC2Delay: delay2}
		str, err := json.Marshal(stats)
		if err != nil {
			return fmt.Errorf("marshaling json: %w", err)
		}
		fmt.Println(string(str))
	} else {
		fmt.Printf("PHC offset: %s\n", phcOffset)
		fmt.Printf("Delay for PHC1: %s\n", delay1)
		fmt.Printf("Delay for PHC2: %s\n", delay2)
	}

	return nil
}

var phcdiffCmd = &cobra.Command{
	Use:   "phcdiff",
	Short: "Print diff in ns between 2 PHCs",
	Run: func(c *cobra.Command, args []string) {
		ConfigureVerbosity()
		if err := phcdiffRun(phcDiffDeviceA, phcDiffDeviceB, phcDiffIsJSON); err != nil {
			log.Fatal(err)
		}
	},
}
