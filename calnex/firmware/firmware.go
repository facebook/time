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
	"strings"
	"time"

	calnexAPI "github.com/facebook/time/calnex/api"
	version "github.com/hashicorp/go-version"
	log "github.com/sirupsen/logrus"
)

// FW is an interface of the Firmware Version
type FW interface {
	// Version returns latest fw version available
	Version() (*version.Version, error)
	// Path returns local FW path
	Path() (string, error)
}

// ShouldUpgrade checks if Calnex firmware needs an upgrade
func ShouldUpgrade(target string, api *calnexAPI.API, fw FW, force bool) (bool, error) {
	cv, err := api.FetchVersion()
	if err != nil {
		return false, err
	}
	calnexVersion, err := version.NewVersion(strings.ToLower(cv.Firmware))
	if err != nil {
		return false, err
	}

	v, err := fw.Version()
	if err != nil {
		return false, err
	}

	if calnexVersion.LessThan(v) || force {
		log.Infof("%s: is running %s, latest is %s. Needs an update", target, calnexVersion, v)
		return true, nil
	}

	log.Infof("%s: no firmware update is required", target)
	return false, nil
}

// InProgress checks if Calnex firmware upgrade is in progress
func InProgress(target string, api *calnexAPI.API) (bool, error) {
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
func Firmware(target string, insecureTLS bool, fw FW, apply bool, force bool) error {
	api := calnexAPI.NewAPI(target, insecureTLS, 4*time.Minute)

	shouldUpgrade, err := ShouldUpgrade(target, api, fw, force)
	if err != nil {
		return err
	}

	if !shouldUpgrade {
		if _, err = InProgress(target, api); err != nil {
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
		log.Infof("%s: stopping measurement", target)
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
	return err
}
