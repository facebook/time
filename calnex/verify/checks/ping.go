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
	"errors"
	"time"

	"github.com/go-ping/ping"
)

// Ping check
type Ping struct {
	Remediation Remediation
}

// Name returns the name of the check
func (p *Ping) Name() string {
	return "Ping"
}

// Run executes the check
func (p *Ping) Run(target string, _ bool) error {
	pinger, err := ping.NewPinger(target)
	if err != nil {
		return err
	}

	pinger.Count = 3
	pinger.Timeout = time.Second
	err = pinger.Run()
	if err != nil {
		return err
	}

	stats := pinger.Statistics()
	if stats.PacketLoss != 100 {
		return nil
	}

	return errors.New("ping: unreachable")
}

// Remediate the check
func (p *Ping) Remediate() (string, error) {
	return p.Remediation.Remediate()
}

// PingRemediation is an open source action for Ping check
type PingRemediation struct{}

// Remediate remediates the Ping check failure
func (a PingRemediation) Remediate() (string, error) {
	return "Restart the device", nil
}
