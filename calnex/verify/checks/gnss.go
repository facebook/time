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
	"context"
	"fmt"
	"time"

	"github.com/facebook/time/calnex/api"
)

// GNSS check
type GNSS struct {
	Remediation Remediation
}

// Name returns the name of the check
func (p *GNSS) Name() string {
	return "GNSS"
}

// Run executes the check
func (p *GNSS) Run(target string, insecureTLS bool) error {
	api := api.NewAPI(target, insecureTLS, 10*time.Second)

	g, err := api.GnssStatus()
	if err != nil {
		return err
	}

	if g.LockedSatellites < 4 {
		return fmt.Errorf("gnss: not enough satellites")
	}

	return nil
}

// Remediate the check
func (p *GNSS) Remediate(ctx context.Context) (string, error) {
	return p.Remediation.Remediate(ctx)
}

// GNSSRemediation is an open source remediation for GNSS check
type GNSSRemediation struct{}

// Remediate remediates the GNSS check failure
func (a GNSSRemediation) Remediate(_ context.Context) (string, error) {
	return "Check antenna cabling", nil
}
