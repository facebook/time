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
	"github.com/facebook/time/calnex/api"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(clearCmd)
	clearCmd.Flags().BoolVar(&apply, "apply", false, "apply the config changes")
	clearCmd.Flags().BoolVar(&insecureTLS, "insecureTLS", false, "Ignore TLS certificate errors")
	clearCmd.Flags().StringVar(&target, "target", "", "device to configure")
	if err := clearCmd.MarkFlagRequired("target"); err != nil {
		log.Fatal(err)
	}
}

func clear() error {
	if !apply {
		log.Info("dry run. Exiting")
		return nil
	}

	api := api.NewAPI(target, insecureTLS)
	if err := api.ClearDevice(); err != nil {
		return err
	}

	log.Infof("Device data cleared. The device will now reboot.")

	return nil
}

var clearCmd = &cobra.Command{
	Use:   "clear",
	Short: "clear device data",
	Run: func(cmd *cobra.Command, args []string) {
		if err := clear(); err != nil {
			log.Fatal(err)
		}
	},
}
