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

// HTTP check
type HTTP struct {
	Remediation Remediation
}

// Name returns the name of the check
func (p *HTTP) Name() string {
	return "HTTP"
}

// Run executes the check
func (p *HTTP) Run(target string, insecureTLS bool) error {
	api := api.NewAPI(target, insecureTLS, 10*time.Second)

	_, err := api.FetchStatus()
	if err != nil {
		return fmt.Errorf("https: %w", err)
	}
	return nil
}

// Remediate the check
func (p *HTTP) Remediate(ctx context.Context) (string, error) {
	return p.Remediation.Remediate(ctx)
}

// HTTPRemediation is an open source remediation for HTTP check
type HTTPRemediation struct{}

// Remediate remediates the HTTP check failure
func (a HTTPRemediation) Remediate(_ context.Context) (string, error) {
	return "Restart the device", nil
}
