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
	"time"

	"github.com/facebook/time/calnex/api"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(rebootCmd)
	rebootCmd.Flags().BoolVar(&apply, "apply", false, "apply the config changes")
	rebootCmd.Flags().BoolVar(&insecureTLS, "insecureTLS", false, "Ignore TLS certificate errors")
	rebootCmd.Flags().StringVar(&target, "target", "", "device to configure")
	if err := rebootCmd.MarkFlagRequired("target"); err != nil {
		log.Fatal(err)
	}
}

func reboot() error {
	if !apply {
		log.Info("dry run. Exiting")
		return nil
	}

	api := api.NewAPI(target, insecureTLS, time.Minute)
	if err := api.Reboot(); err != nil {
		return err
	}

	log.Infof("Calnex device will now reboot.")

	return nil
}

var rebootCmd = &cobra.Command{
	Use:   "reboot",
	Short: "reboot the device",
	Run: func(cmd *cobra.Command, args []string) {
		if err := reboot(); err != nil {
			log.Fatal(err)
		}
	},
}
