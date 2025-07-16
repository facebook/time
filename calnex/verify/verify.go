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

package verify

import (
	"context"

	"github.com/facebook/time/calnex/verify/checks"
	log "github.com/sirupsen/logrus"
)

// VF is an open source implementation of the VF interface
type VF struct {
	Checks []checks.Check
}

// Verify runs health checks and report diagnosis
func Verify(ctx context.Context, target string, insecureTLS bool, verify *VF, apply bool) error {
	for _, c := range verify.Checks {
		if err := c.Run(target, insecureTLS); err != nil {
			log.Warningf("%s: %s check fail: %v", target, c.Name(), err)
			if apply {
				result, err := c.Remediate(ctx)
				if result != "" {
					log.Warningf("%s: %s", target, result)
				}
				if err != nil {
					return err
				}
				break
			}
		} else {
			log.Debugf("%s: %s check pass", target, c.Name())
		}
	}
	return nil
}
