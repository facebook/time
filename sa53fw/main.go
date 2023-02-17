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

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	fw "github.com/facebook/time/sa53fw/firmware"
	"github.com/facebook/time/sa53fw/mac"
	"github.com/facebook/time/sa53fw/xmodem"
	"github.com/fatih/color"
	"golang.org/x/term"
)

var okString = color.GreenString("[OK]")
var infoString = color.GreenString("[INFO]")
var warnString = color.YellowString("[WARN]")
var failString = color.RedString("[FAIL]")

func progressLine(format string, args ...interface{}) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return
	}
	fmt.Printf("\u001b[1000D")
	fmt.Printf(format, args...)
}

func main() {
	var serialPort, fwFile string
	var upgrade, force bool

	flag.StringVar(&serialPort, "serial", "/dev/ttyS6", "SA53 serial port device")
	flag.BoolVar(&upgrade, "upgrade", false, "Should we try to upgrade firmware")
	flag.StringVar(&fwFile, "fw", "", "SA53 new firmware file")
	flag.BoolVar(&force, "force", false, "Force firmware upgrade")
	flag.Parse()

	// init serial port for MAC53
	sa53, err := mac.Init(serialPort)
	if err != nil {
		fmt.Println(failString, err)
		return
	}
	defer sa53.Close()

	fmt.Println(infoString, "Requesting firmware version...")

	err = sa53.ReadFirmware()
	if err != nil && !errors.Is(err, mac.ErrFWFormat) {
		fmt.Println(failString, err)
		return
	}

	if errors.Is(err, mac.ErrFWFormat) {
		fmt.Println(warnString, err)
	} else {
		fmt.Println(okString, "SA53 has firmware version:", sa53.FormatFWVersion())
	}

	if fwFile == "" {
		fmt.Println(failString, "Firmware file name must be provided")
		return
	}

	f, err := fw.Open(fwFile)
	if err != nil {
		fmt.Println(failString, err)
		return
	}
	defer f.Close()

	if err = f.ParseVersion(); err != nil {
		fmt.Println(failString, err)
		if !force {
			return
		}
		fmt.Println(warnString, "Force flag was provided, continue...")
	} else {
		fmt.Println(okString, "Firmware file version:", f.FormatFWVersion())
	}

	if sa53.Version() >= f.Version() && !force {
		fmt.Println(failString, "SA53 has the same or newer firmware, upgrade is not needed")
		return
	}

	if !upgrade {
		fmt.Println(warnString, "Please provide -upgrade flag to upgrade firmware")
		return
	}

	if err = sa53.Reset(); err != nil {
		fmt.Println(failString, err)
		return
	}
	fmt.Println(okString, "Reset command ok, switching to upload mode...")

	if err = sa53.Upgrade(); err != nil {
		fmt.Println(failString, err)
		return
	}
	fmt.Println(okString, "Upload mode, init XModem...")

	if err = sa53.XModemInit(); err != nil {
		fmt.Println(failString, err)
		return
	}

	sent := 0
	block := uint16(1)
	buff := make([]byte, xmodem.XModem1KBlockSsize)
	for n, err := f.Read(buff); err == nil && n > 0; n, err = f.Read(buff) {
		if err = xmodem.SendBlock1K(sa53, uint8(block&0x0ff), buff, uint16(n)); err != nil {
			fmt.Printf("%s Block %d, error %v\n", failString, block, err)
		}
		block++
		sent += n
		progressLine("Send block %d, bytes %d/%d\n", block, sent, f.Size())
	}

	if err = xmodem.SendEOT(sa53); err != nil {
		fmt.Printf("%s Firmware upgrade completed with error, %v", failString, err)
		return
	}

	if err = sa53.XModemDone(); err != nil {
		fmt.Println(failString, err)
		return
	}

	fmt.Println(okString, "Firmware upgrade completed without error")

	fmt.Println(okString, "SA53 is reloading...")
	if err = sa53.Reset(); err != nil {
		fmt.Println(failString, "SA53 failed to reload")
		return
	}

	fmt.Println(okString, "Waiting for SA53 to boot...")
	if err = sa53.WaitBoot(); err != nil {
		fmt.Println(failString, err)
		return
	}
	time.Sleep(time.Second)
	if err = sa53.ReadFirmware(); err != nil {
		fmt.Println(failString, err)
		return
	}

	fmt.Println(okString, "SA53 FW version: ", sa53.FormatFWVersion())
}
