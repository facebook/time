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
	"os"

	"github.com/facebook/time/calnex/api"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(certCmd)
	certCmd.Flags().BoolVar(&apply, "apply", false, "apply the config changes")
	certCmd.Flags().BoolVar(&insecureTLS, "insecureTLS", false, "Ignore TLS certificate errors")
	certCmd.Flags().StringVar(&target, "target", "", "device to configure")
	certCmd.Flags().StringVar(&source, "file", "", "certificate file path")

	if err := certCmd.MarkFlagRequired("target"); err != nil {
		log.Fatal(err)
	}
	if err := certCmd.MarkFlagRequired("file"); err != nil {
		log.Fatal(err)
	}
}

func cert() error {
	api := api.NewAPI(target, insecureTLS)
	cert, err := os.ReadFile(source)
	if err != nil {
		return err
	}

	if !apply {
		log.Infof("dry run. Exiting")
		return nil
	}

	r, err := api.PushCert(cert)
	log.Infof(r.Message)
	return err
}

var certCmd = &cobra.Command{
	Use:   "cert",
	Short: "install device certificate",
	Run: func(cmd *cobra.Command, args []string) {
		if err := cert(); err != nil {
			log.Fatal(err)
		}
	},
}
