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

package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/facebook/time/fbclock"
	"github.com/facebook/time/phc"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// ManagedPTPDevicePath is the path we will set up a copy of iface's PHC device,
// so that fbclock clients can access it without explicit configuration.
const ManagedPTPDevicePath = string(fbclock.PTPPath)

// SetupDeviceDir creates a PHC device path from the interface name
func SetupDeviceDir(iface string) error {
	// explicitly convert to string to prevent GOPLS from panicking here
	target := ManagedPTPDevicePath
	dir := filepath.Dir(target)
	wantMode := os.ModeCharDevice | os.ModeDevice | 0644

	device, err := phc.IfaceToPHCDevice(iface)
	if err != nil {
		return fmt.Errorf("getting PHC device from iface %q: %w", iface, err)
	}

	devInfo, err := os.Stat(device)
	if err != nil {
		return fmt.Errorf("getting PTP device %q info: %w", device, err)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("preparing dir %s: %w", dir, err)
	}
	// check if it's already there
	if targetInfo, err := os.Stat(target); err == nil {
		if os.SameFile(devInfo, targetInfo) && targetInfo.Mode() == wantMode {
			log.Debugf("Device %s already exists, nothing to link", target)
			return nil
		}
		// remove the wrong file
		if err := os.RemoveAll(target); err != nil {
			log.Debugf("Removing %s", target)
			return fmt.Errorf("removing target file %q: %w", target, err)
		}
	}
	log.Debugf("Linking %s to %s", device, target)
	if err := os.Link(device, target); err != nil {
		return fmt.Errorf("linking device %s to %s: %w", device, target, err)
	}
	return os.Chmod(target, wantMode)
}

// TimeMonotonicRaw returns the current time from CLOCK_MONOTONIC_RAW
func TimeMonotonicRaw() (time.Time, error) {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC_RAW, &ts); err != nil {
		return time.Time{}, fmt.Errorf("failed clock_gettime: %w", err)
	}
	return time.Unix(ts.Unix()), nil
}
