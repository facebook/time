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

import "context"

// Check abstracts the checks to be executed
type Check interface {
	Name() string
	Run(name string, insecureTLS bool) error
	Remediate(ctx context.Context) (string, error)
}

// Remediation interface with check remediation
type Remediation interface {
	Remediate(ctx context.Context) (string, error)
}
