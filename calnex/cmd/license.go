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
	RootCmd.AddCommand(licenseCmd)
	licenseCmd.Flags().BoolVar(&insecureTLS, "insecureTLS", false, "Ignore TLS certificate errors")
	licenseCmd.Flags().StringVar(&target, "target", "", "device to configure")
	licenseCmd.Flags().StringVar(&source, "file", "", "license file path")

	if err := licenseCmd.MarkFlagRequired("target"); err != nil {
		log.Fatal(err)
	}
	if err := licenseCmd.MarkFlagRequired("file"); err != nil {
		log.Fatal(err)
	}
}

func licenseFunc() error {
	api := api.NewAPI(target, insecureTLS, time.Minute)
	_, err := api.PushLicense(source)
	return err
}

var licenseCmd = &cobra.Command{
	Use:   "license",
	Short: "install device license",
	Run: func(_ *cobra.Command, _ []string) {
		if err := licenseFunc(); err != nil {
			log.Fatal(err)
		}
	},
}
