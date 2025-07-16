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

// RB is rubidium clock check
type RB struct {
	Remediation Remediation
}

// Name returns the name of the check
func (p *RB) Name() string {
	return "Rubidium clock"
}

// Run executes the check
func (p *RB) Run(target string, insecureTLS bool) error {
	api := api.NewAPI(target, insecureTLS, 10*time.Second)

	rb, err := api.RBStatus()
	if err != nil {
		return err
	}

	if rb.RBState == 5 {
		// RBState 5 is "Disciplining"
		return nil
	}

	if rb.RBState == 7 {
		// RBState 7 is "Hold-over (Weak GNSS)"
		return nil
	}
	return fmt.Errorf("Rubidium clock state is: %s", rb.RBStateName)
}

// Remediate the check
func (p *RB) Remediate(ctx context.Context) (string, error) {
	return p.Remediation.Remediate(ctx)
}

// RBRemediation is an open source remediation for RB check
type RBRemediation struct{}

// Remediate remediates the RB check failure
func (a RBRemediation) Remediate(_ context.Context) (string, error) {
	return "Replace failed unit", nil
}
