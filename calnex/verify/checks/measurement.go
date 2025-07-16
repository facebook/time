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

const (
	measuring           = "Measuring"
	ready               = "Ready"
	readyForMeasurement = "ReadyForMeasurement"
)

// Module check
type Module struct {
	Remediation Remediation
}

// Name returns the name of the check
func (m *Module) Name() string {
	return "Module"
}

// Run executes the check
func (m *Module) Run(target string, insecureTLS bool) error {
	api := api.NewAPI(target, insecureTLS, 10*time.Second)

	ms, err := api.FetchInstrumentStatus()
	if err != nil {
		return err
	}

	for channel, module := range ms.Modules {
		if module.State != measuring && module.State != ready && module.State != readyForMeasurement {
			return fmt.Errorf("module: failed module %s: state: %s", channel, module.State)
		}
	}

	return nil
}

// Remediate the check
func (m *Module) Remediate(ctx context.Context) (string, error) {
	return m.Remediation.Remediate(ctx)
}

// ModuleRemediation is an open source remediation for Module check
type ModuleRemediation struct{}

// Remediate remediates the Module check failure
func (a ModuleRemediation) Remediate(_ context.Context) (string, error) {
	return "Restart the device", nil
}
