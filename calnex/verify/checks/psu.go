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

package checks

import (
	"fmt"
	"time"

	"github.com/facebook/time/calnex/api"
)

// PSU check
type PSU struct {
	Remediation Remediation
}

// Name returns the name of the check
func (p *PSU) Name() string {
	return "PSU"
}

// Run executes the check
func (p *PSU) Run(target string, insecureTLS bool) error {
	api := api.NewAPI(target, insecureTLS, 10*time.Second)

	pu, err := api.PowerSupplyStatus()
	if err != nil {
		return err
	}

	if !pu.PowerSupplyGood {
		for i, psu := range pu.Supplies {
			if !psu.StatusGood {
				return fmt.Errorf("psu: failed power supply #%d: %s", i, psu.Name)
			}
		}
		return fmt.Errorf("psu: failed power supply")
	}

	return nil
}

// Remediate the check
func (p *PSU) Remediate() (string, error) {
	return p.Remediation.Remediate()
}

// PSURemediation is an open source remediation for PSU check
type PSURemediation struct{}

// Remediate remediates the PSU check failure
func (a PSURemediation) Remediate() (string, error) {
	return "Replace failed PSU", nil
}
