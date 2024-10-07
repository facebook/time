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
	"math"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/facebook/time/phc"
)

// flags
var (
	device      string
	method      string
	freq        float64
	step        time.Duration
	setAndPrint bool
)

var phcCmd = &cobra.Command{
	Use:   "phc",
	Short: "Print PHC clock information. Use `phc_ctl` cli for richer functionality",
	Run:   runPhcCmd,
}

func init() {
	RootCmd.AddCommand(phcCmd)
	flags := phcCmd.Flags()
	flags.StringVarP(&device, "device", "d", "/dev/ptp0", "PTP device to get time from")
	flags.StringVarP(&method, "method", "m", string(phc.MethodIoctlSysOffsetExtended),
		fmt.Sprintf("Method to get PHC time: %v", phc.SupportedMethods),
	)
	flags.Float64VarP(&freq, "freq", "f", math.NaN(), "set the frequency (PPB)")
	flags.DurationVarP(&step, "step", "t", 0, "step the clock")
	flags.BoolVarP(&setAndPrint, "print", "p", false, "print clock status after changes")
}

func runPhcCmd(_ *cobra.Command, _ []string) {
	var doPrint = true

	ConfigureVerbosity()
	if step != 0 {
		if err := stepPHC(device, step); err != nil {
			log.Fatal(err)
		}
		doPrint = setAndPrint
	}
	if !math.IsNaN(freq) {
		if err := tunePHC(device, freq); err != nil {
			log.Fatal(err)
		}
		doPrint = setAndPrint
	}
	if doPrint {
		if err := printPHC(device, phc.TimeMethod(method)); err != nil {
			log.Fatal(err)
		}
	}
}

func stepPHC(device string, step time.Duration) error {
	f, err := os.OpenFile(device, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("opening device %q: %w", device, err)
	}
	defer f.Close()
	dev := phc.FromFile(f)

	fmt.Printf("Stepping the clock by %v\n", step)
	return dev.Step(step)
}

func tunePHC(device string, freq float64) error {
	f, err := os.OpenFile(device, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("opening device %q: %w", device, err)
	}
	defer f.Close()
	dev := phc.FromFile(f)

	maxFreq, err := dev.MaxFreqAdjPPB()
	if err != nil {
		return err
	}
	if freq < -maxFreq || freq > maxFreq {
		return fmt.Errorf("frequncy %f is out supported range", freq)
	}
	fmt.Printf("Setting new frequency value %f\n", freq)
	return dev.AdjFreq(freq)
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

	f, err := os.OpenFile(device, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("opening device %q to read frequency: %w", device, err)
	}
	defer f.Close()
	dev := phc.FromFile(f)

	curFreq, err := dev.FreqPPB()
	if err != nil {
		return err
	}
	maxFreq, err := dev.MaxFreqAdjPPB()
	if err != nil {
		return err
	}
	fmt.Printf("Current frequency: %f\n", curFreq)
	fmt.Printf("Frequency range: [%.2f, %.2f]\n", -maxFreq, maxFreq)
	return nil
}
