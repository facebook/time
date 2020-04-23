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
	"syscall"

	"github.com/spf13/cobra"
)

// clockState report system clock state via adjtimex syscall
func clockState() {
	if state, err := syscall.Adjtimex(&syscall.Timex{}); err != nil {
		fmt.Printf("Error calling adjtimex(2): %s", err)
	} else {
		if desc, ok := timexToDesc[state]; ok {
			fmt.Println(desc)
		} else {
			fmt.Printf("Error: %v state is not recognized\n", state)
		}
	}
}

// ntpTime prints data similar to 'ntptime' command output
func ntpTime() {
	var buf syscall.Timex
	if state, err := syscall.Adjtimex(&buf); err != nil {
		fmt.Printf("Error calling adjtimex(2): %s", err)
	} else {
		if desc, ok := timexToDesc[state]; ok {
			fmt.Printf("adjtimex() returns code %d (%s)\n", state, desc)
		} else {
			fmt.Printf("Error: %v state is not recognized\n", state)
		}

		var offset float64
		// 0x2000 is STA_NANO
		if buf.Status&0x2000 != 0 {
			offset = float64(buf.Offset) / 1000.0 // ns -> us
		} else {
			offset = float64(buf.Offset)
		}

		fmt.Printf("  modes 0x%x,\n", buf.Modes)
		fmt.Printf("  offset %.3f us, frequency %.3f ppm, interval %d s\n", offset, float64(buf.Freq)/65536.0, buf.Shift)
		fmt.Printf("  maximum error %d us, estimated error %d us,\n", buf.Maxerror, buf.Esterror)
		fmt.Printf("  status 0x%x,\n", buf.Status)
		fmt.Printf("  time constant %d, precision %d.000 us, tolerance %d ppm,\n", buf.Constant, buf.Precision, buf.Tolerance/65535)
	}
}

func init() {
	// clockstate
	utilsCmd.AddCommand(clockStateCmd)
	// ntptime
	utilsCmd.AddCommand(ntpTimeCmd)
}

var clockStateCmd = &cobra.Command{
	Use:   "clockstate",
	Short: "Print kernel clock state with description.",
	Long: `Print kernel clock state with description.
Useful for checking if kernel noticed leap second. Uses adjtimex(2) to get info.`,
	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()
		clockState()
	},
}

var ntpTimeCmd = &cobra.Command{
	Use:   "ntptime",
	Short: "Print OS kernel output that is similar to ntp_gettime() and ntp_adjtime() output of 'ntptime' utility.",
	Long:  "'ntptime' utility is a part of ntp package. This command produces similar output.",
	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()
		ntpTime()
	},
}
