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

package firmware

import (
	"fmt"
	"strings"
	"sync"
	"time"

	calnexAPI "github.com/facebook/time/calnex/api"
	version "github.com/hashicorp/go-version"
	log "github.com/sirupsen/logrus"
)

// FW is an interface of the Firmware Version
type FW interface {
	// Version returns latest fw version available
	Version() *version.Version
	// Path returns local FW path
	Path() (string, error)
}

// CalnexUpgraderInterface represents an upgradeable firmware
type CalnexUpgraderInterface interface {
	Firmware(target string, insecureTLS bool, fw FW, apply bool, force bool) error
	InProgress(target string, api *calnexAPI.API) (bool, error)
	ShouldUpgrade(target string, api *calnexAPI.API, fw FW, force bool) (bool, error)
}

// CalnexUpgrader represents an upgradeable firmware
type CalnexUpgrader struct{}

// ShouldUpgrade checks if Calnex firmware needs an upgrade
func (up CalnexUpgrader) ShouldUpgrade(target string, api *calnexAPI.API, fw FW, force bool) (bool, error) {
	cv, err := api.FetchVersion()
	if err != nil {
		return false, err
	}
	//remove major version 2.17.0 -> 17.0 as this is hadware revision related
	calnexVersion, err := version.NewVersion(strings.ToLower(strings.SplitN(cv.Firmware, ".", 2)[1]))
	if err != nil {
		return false, err
	}

	if calnexVersion.LessThan(fw.Version()) || force {
		log.Debugf("%s: is running %s, latest is %s. Needs an update", target, calnexVersion, fw.Version())
		return true, nil
	}

	log.Debugf("%s: no firmware update is required", target)
	return false, nil
}

// InProgress checks if Calnex firmware upgrade is in progress
func (up CalnexUpgrader) InProgress(target string, api *calnexAPI.API) (bool, error) {
	// Checking maybe upgrade is in progress
	instrumentStatus, statusErr := api.FetchInstrumentStatus()
	if statusErr != nil {
		return false, statusErr
	}
	if instrumentStatus.Channels[calnexAPI.ChannelONE].Progress != -1 {
		log.Infof("%s: update %d%% complete", target, instrumentStatus.Channels[calnexAPI.ChannelONE].Progress)
		return true, nil
	}

	return false, nil
}

// Firmware checks target Calnex firmware version and upgrades if apply is specified
// Returns err if there is a failure at any point in the process
func (up CalnexUpgrader) Firmware(target string, insecureTLS bool, fw FW, apply bool, force bool) error {
	api := calnexAPI.NewAPI(target, insecureTLS, 4*time.Minute)

	shouldUpgrade, err := up.ShouldUpgrade(target, api, fw, force)
	if err != nil {
		return err
	}

	if !shouldUpgrade {
		_, err := up.InProgress(target, api)
		if err != nil {
			return err
		}
		return nil
	}

	if !apply {
		log.Infof("%s: dry run. Not upgrading firmware", target)
		return nil
	}

	status, err := api.FetchStatus()
	if err != nil {
		return err
	}
	if status.MeasurementActive {
		log.Debugf("%s: stopping measurement", target)
		// stop measurement
		if err = api.StopMeasure(); err != nil {
			return err
		}
	}
	log.Infof("%s: updating firmware", target)
	p, err := fw.Path()
	if err != nil {
		return err
	}
	_, err = api.PushVersion(p)
	if err != nil {
		return err
	}
	return nil
}

// ParallelFirmwareUpgrade upgrades the provided list of devices in parallel
// Returns a slice of errors, which contains an error for each device that failed to upgrade
func ParallelFirmwareUpgrade(devices []string, insecureTLS bool, fw FW, ufw CalnexUpgraderInterface, apply bool, force bool) []error {
	var wg = sync.WaitGroup{}
	errors := make([]error, 0, len(devices))
	errorMutex := sync.Mutex{}
	for i := range len(devices) {
		wg.Add(1)
		device := devices[i]
		go func(device string) {
			defer wg.Done()
			err := ufw.Firmware(device, insecureTLS, fw, apply, force)
			if err != nil {
				errorMutex.Lock()
				errors = append(errors, fmt.Errorf("%s: error during firmware upgrade: %w", device, err))
				errorMutex.Unlock()
			}
		}(device)
	}
	wg.Wait()
	return errors
}
