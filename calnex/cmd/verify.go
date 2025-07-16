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

package cmd

import (
	"github.com/facebook/time/calnex/verify"
	"github.com/facebook/time/calnex/verify/checks"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(verifyCmd)
	verifyCmd.Flags().BoolVar(&apply, "apply", false, "execute remediation if available")
	verifyCmd.Flags().BoolVar(&insecureTLS, "insecureTLS", false, "Ignore TLS certificate errors")
	verifyCmd.Flags().StringVar(&target, "device", "", "device to verify")
	if err := verifyCmd.MarkFlagRequired("device"); err != nil {
		log.Fatal(err)
	}
}

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "verify if the appliance needs to be sent to repair",
	Run: func(cmd *cobra.Command, _ []string) {
		v := &verify.VF{Checks: []checks.Check{
			&checks.Ping{Remediation: checks.PingRemediation{}},
			&checks.HTTP{Remediation: checks.HTTPRemediation{}},
			&checks.GNSS{Remediation: checks.GNSSRemediation{}},
			&checks.PSU{Remediation: checks.PSURemediation{}},
			&checks.Module{Remediation: checks.ModuleRemediation{}},
		}}
		if err := verify.Verify(cmd.Context(), target, insecureTLS, v, apply); err != nil {
			log.Fatal(err)
		}
	},
}
