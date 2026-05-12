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
	"errors"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/facebook/time/sa53/detect"
	"github.com/facebook/time/sa53/firmware"
	"github.com/facebook/time/sa53/protocol"
	"github.com/facebook/time/sa53/xmodem"
)

var (
	fwFile  string
	upgrade bool
	force   bool
	check   bool
)

func init() {
	RootCmd.AddCommand(firmwareCmd)
	firmwareCmd.Flags().StringVar(&fwFile, "fw", "", "SA53 new firmware file")
	firmwareCmd.Flags().BoolVar(&upgrade, "upgrade", false, "Apply the firmware upgrade")
	firmwareCmd.Flags().BoolVar(&force, "force", false, "Force firmware upgrade")
	firmwareCmd.Flags().BoolVar(&check, "check", false, "Check firmware version only (JSON output)")
}

type checkResult struct {
	Vendor   string `json:"vendor"`
	BoardID  string `json:"board_id"`
	Firmware string `json:"firmware"`
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
	cards, err := detect.Timecards()
	if err != nil {
		return fmt.Errorf("cannot detect time cards: %w", err)
	}

	var sa5xCards []*detect.Result
	for _, c := range cards {
		if c.IsSA5x() {
			sa5xCards = append(sa5xCards, c)
		}
	}

	if len(sa5xCards) == 0 {
		log.Info("no Celestica/SA5x time cards found, skipping")
		return nil
	}

	for _, card := range sa5xCards {
		log.Infof("detected Celestica time card board_id=%s pci=%s", card.BoardID, card.PCIAddr)
	}

	sa53, err := protocol.Init(serialPort)
	if err != nil {
		return err
	}
	defer sa53.Close()

	log.Info("requesting firmware version")
	err = sa53.ReadFirmware()
	if err != nil && !errors.Is(err, protocol.ErrFWFormat) {
		return err
	}
	if errors.Is(err, protocol.ErrFWFormat) {
		log.Warnf("firmware version unreadable: %v", err)
	} else {
		log.Infof("SA53 firmware version=%s", sa53.FormatFWVersion())
	}

	// --check mode: print version as JSON and exit
	if check {
		cr := checkResult{
			Vendor:   sa5xCards[0].Vendor,
			BoardID:  sa5xCards[0].BoardID,
			Firmware: sa53.FormatFWVersion(),
		}
		data, err := json.Marshal(cr)
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	if fwFile == "" {
		return fmt.Errorf("firmware file name must be provided via --fw")
	}

	f, err := firmware.Open(fwFile)
	if err != nil {
		return err
	}
	defer f.Close()

	if err = f.ParseVersion(); err != nil {
		if !force {
			return err
		}
		log.Warnf("firmware footer parse failed, continuing due to --force: %v", err)
	} else {
		log.Infof("firmware file version=%s", f.FormatFWVersion())
	}

	if sa53.Version() >= f.Version() && !force {
		return fmt.Errorf("SA53 already has the same or newer firmware, upgrade not needed")
	}

	if !upgrade {
		log.Warn("dry run; pass --upgrade to apply")
		return nil
	}

	if err = sa53.Reset(); err != nil {
		return err
	}
	log.Info("reset ok, switching to upload mode")

	if err = sa53.Upgrade(); err != nil {
		return err
	}
	log.Info("upload mode active, init XModem")

	if err = sa53.XModemInit(); err != nil {
		return err
	}

	sent := 0
	block := uint16(1)
	buff := make([]byte, xmodem.XModem1KBlockSsize)
	for n, err := f.Read(buff); err == nil && n > 0; n, err = f.Read(buff) {
		// n is bounded by len(buff) = xmodem.XModem1KBlockSsize (1024), fits in uint16.
		//nolint:gosec // G115 false positive: n bounded by xmodem block size
		if err = xmodem.SendBlock1K(sa53, uint8(block&0x0ff), buff, uint16(n)); err != nil {
			log.Errorf("xmodem block %d failed: %v", block, err)
		}
		block++
		sent += n
		log.Debugf("xmodem progress block=%d sent=%d size=%d", block, sent, f.Size())
	}

	if err = xmodem.SendEOT(sa53); err != nil {
		return fmt.Errorf("firmware upgrade completed with error: %w", err)
	}

	if err = sa53.XModemDone(); err != nil {
		return err
	}
	log.Info("firmware upgrade completed")

	log.Info("SA53 reloading")
	if err = sa53.Reset(); err != nil {
		return fmt.Errorf("SA53 failed to reload: %w", err)
	}

	log.Info("waiting for SA53 to boot")
	if err = sa53.WaitBoot(); err != nil {
		return err
	}
	time.Sleep(time.Second)
	if err = sa53.ReadFirmware(); err != nil {
		return err
	}
	log.Infof("SA53 firmware after upgrade version=%s", sa53.FormatFWVersion())
	return nil
}
