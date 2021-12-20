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
	"github.com/facebook/time/calnex/firmware"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(firmwareCmd)
	firmwareCmd.Flags().BoolVar(&insecureTLS, "insecureTLS", false, "Ignore TLS certificate errors")
	firmwareCmd.Flags().BoolVar(&apply, "apply", false, "apply the firmware upgrade")
	firmwareCmd.Flags().StringVar(&target, "target", "", "device to configure")
	firmwareCmd.Flags().StringVar(&source, "file", "", "firmware file path")
	if err := firmwareCmd.MarkFlagRequired("target"); err != nil {
		log.Fatal(err)
	}
	if err := firmwareCmd.MarkFlagRequired("file"); err != nil {
		log.Fatal(err)
	}
}

var firmwareCmd = &cobra.Command{
	Use:   "firmware",
	Short: "update the device firmware",
	Run: func(cmd *cobra.Command, args []string) {
		fw := &firmware.OSSFW{
			Filepath: source,
		}
		if err := firmware.Firmware(target, insecureTLS, fw, apply); err != nil {
			log.Fatal(err)
		}
	},
}
