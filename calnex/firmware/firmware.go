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

	"github.com/facebook/time/calnex/api"
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

// Firmware checks target Calnex firmware version via protocol and upgrades if apply is specified
func Firmware(target string, insecureTLS bool, fw FW, apply bool) error {
	api := api.NewAPI(target, insecureTLS)
	cv, err := api.FetchVersion()
	if err != nil {
		return err
	}
	calnexVersion, err := version.NewVersion(strings.ToLower(cv.Firmware))
	if err != nil {
		return err
	}

	v, err := fw.Version()
	if err != nil {
		return err
	}
	if calnexVersion.GreaterThanOrEqual(v) {
		log.Infof("no update is required")
		return nil
	}

	log.Infof("%s is running %s, latest is %s. Needs an update", target, calnexVersion, v)

	if !apply {
		log.Infof("dry run. Exiting")
		return nil
	}

	status, err := api.FetchStatus()
	if err != nil {
		return err
	}
	if status.MeasurementActive {
		log.Infof("stopping measurement")
		// stop measurement
		if err = api.StopMeasure(); err != nil {
			return err
		}
	}
	log.Infof("updating firmware")
	p, err := fw.Path()
	if err != nil {
		return err
	}
	_, err = api.PushVersion(p)
	return err
}
