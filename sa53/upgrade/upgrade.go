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

// Package upgrade applies SA53 firmware to the local Celestica time card over
// serial/XModem. The firmware itself comes from a pluggable Source, so the same
// apply path serves both a local file and a remote (e.g. netcode) download.
package upgrade

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	version "github.com/hashicorp/go-version"
	log "github.com/sirupsen/logrus"

	"github.com/facebook/time/sa53/detect"
	"github.com/facebook/time/sa53/firmware"
	"github.com/facebook/time/sa53/protocol"
	"github.com/facebook/time/sa53/xmodem"
)

// ErrUpToDate is returned by Apply when the chip already runs the same or newer
// firmware than the candidate and force is not set. Callers should treat it as a
// successful no-op.
var ErrUpToDate = errors.New("SA53 already has the same or newer firmware, upgrade not needed")

// ErrNoCard is returned when no Celestica/SA5x time card is present.
var ErrNoCard = errors.New("no Celestica/SA5x time cards found")

// Source provides an SA53 firmware image. Version reports the candidate version
// without downloading (so a dry run can compare against the chip); Path returns a
// local path, downloading the image first for remote sources.
type Source interface {
	Version() (*version.Version, error)
	Path() (string, error)
}

// LocalSource is a Source backed by an existing local firmware file.
type LocalSource struct {
	FilePath string
}

// Path returns the local firmware file path.
func (s LocalSource) Path() (string, error) {
	if s.FilePath == "" {
		return "", fmt.Errorf("no firmware file path provided")
	}
	return s.FilePath, nil
}

// Version reads the firmware version from the local file's footer.
func (s LocalSource) Version() (*version.Version, error) {
	if s.FilePath == "" {
		return nil, fmt.Errorf("no firmware file path provided")
	}
	f, err := firmware.Open(s.FilePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := f.ParseVersion(); err != nil {
		return nil, err
	}
	return version.NewVersion(strings.TrimPrefix(f.FormatFWVersion(), "V"))
}

// sa5xCards returns the detected Celestica/SA5x time cards.
func sa5xCards() ([]*detect.Result, error) {
	cards, err := detect.Timecards()
	if err != nil {
		return nil, fmt.Errorf("cannot detect time cards: %w", err)
	}
	var sa5x []*detect.Result
	for _, c := range cards {
		if c.IsSA5x() {
			sa5x = append(sa5x, c)
		}
	}
	if len(sa5x) == 0 {
		return nil, ErrNoCard
	}
	for _, card := range sa5x {
		log.Debugf("detected Celestica time card board_id=%s pci=%s", card.BoardID, card.PCIAddr)
	}
	return sa5x, nil
}

// readVersion opens the SA53 over serial and reads its current firmware version.
// A malformed/unreadable version (ErrFWFormat) is logged, not fatal, so a broken
// image can still be reflashed.
func readVersion(sa53 *protocol.Mac) error {
	log.Debug("requesting firmware version")
	err := sa53.ReadFirmware()
	if err != nil && !errors.Is(err, protocol.ErrFWFormat) {
		return err
	}
	if errors.Is(err, protocol.ErrFWFormat) {
		log.Warnf("firmware version unreadable: %v", err)
	} else {
		log.Debugf("SA53 firmware version=%s", sa53.FormatFWVersion())
	}
	return nil
}

// Apply compares the chip's firmware against src and, when apply is set and the
// chip is out of date (or force is set), downloads and flashes it. A dry run
// (apply false) compares without downloading. Returns ErrNoCard when no card is
// present and ErrUpToDate when the chip is already current.
func Apply(serialPort string, src Source, apply, force bool) error {
	// Presence check only; sa5xCards logs the detected board(s).
	if _, err := sa5xCards(); err != nil {
		return err
	}

	// Candidate version comes from metadata, so a dry run never downloads.
	candidate, err := src.Version()
	if err != nil {
		if !force {
			return err
		}
		log.Warnf("cannot determine candidate firmware version, continuing due to --force: %v", err)
	}

	sa53, err := protocol.Init(serialPort)
	if err != nil {
		return err
	}
	defer sa53.Close()

	if err := readVersion(sa53); err != nil {
		return err
	}

	current := sa53.FormatFWVersion()
	if candidate != nil {
		log.Infof("SA53 is running %s, latest is %s", current, candidate)
	} else {
		log.Infof("SA53 is running %s", current)
	}

	if candidate != nil && !force {
		cur, err := version.NewVersion(strings.TrimPrefix(current, "V"))
		if err != nil {
			return err
		}
		if cur.GreaterThanOrEqual(candidate) {
			return ErrUpToDate
		}
	}

	if !apply {
		log.Warn("dry run; pass --apply to upgrade")
		return nil
	}

	// Out of date and applying: download (for remote sources) and flash.
	fwPath, err := src.Path()
	if err != nil {
		return err
	}
	f, err := firmware.Open(fwPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return flash(sa53, f)
}

// flash performs the XModem firmware upload and verifies the chip rebooted into
// the new image. The hardware sequence here has been validated against real
// SA53 cards; keep it in lockstep with the protocol package.
func flash(sa53 *protocol.Mac, f *firmware.Firmware) error {
	if err := sa53.Reset(); err != nil {
		return err
	}
	log.Info("reset ok, switching to upload mode")

	if err := sa53.Upgrade(); err != nil {
		return err
	}
	log.Info("upload mode active, init XModem")

	if err := sa53.XModemInit(); err != nil {
		return err
	}

	sent := 0
	block := uint16(1)
	buff := make([]byte, xmodem.XModem1KBlockSsize)
	for {
		n, err := f.Read(buff)
		if n > 0 {
			// n is bounded by len(buff) = xmodem.XModem1KBlockSsize (1024), fits in uint16.
			//nolint:gosec // G115 false positive: n bounded by xmodem block size
			if err := xmodem.SendBlock1K(sa53, uint8(block&0x0ff), buff, uint16(n)); err != nil {
				return fmt.Errorf("xmodem block %d failed: %w", block, err)
			}
			block++
			sent += n
			log.Debugf("xmodem progress block=%d sent=%d size=%d", block, sent, f.Size())
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("reading firmware: %w", err)
		}
	}

	if err := xmodem.SendEOT(sa53); err != nil {
		return fmt.Errorf("xmodem EOT failed: %w", err)
	}

	if err := sa53.XModemDone(); err != nil {
		return err
	}
	log.Info("firmware upgrade completed")

	log.Info("SA53 reloading")
	if err := sa53.Reset(); err != nil {
		return fmt.Errorf("SA53 failed to reload: %w", err)
	}

	log.Info("waiting for SA53 to boot")
	if err := sa53.WaitBoot(); err != nil {
		return err
	}
	time.Sleep(time.Second)
	if err := sa53.ReadFirmware(); err != nil {
		return err
	}
	log.Infof("SA53 firmware after upgrade version=%s", sa53.FormatFWVersion())
	return nil
}
