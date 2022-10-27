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

	"github.com/facebook/time/fbclock"

	log "github.com/sirupsen/logrus"
)

func SetupDeviceDir(device string) error {
	// explicitly conver to string to prevent GOPLS from panicing here
	target := string(fbclock.PTPPath)
	dir := filepath.Dir(target)
	wantMode := os.ModeCharDevice | os.ModeDevice | 0644
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
