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
	"github.com/facebook/time/calnex/export"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(exportCmd)
	exportCmd.Flags().BoolVar(&allData, "allData", true, "Export entire data from device every run. Set false for unread only")
	exportCmd.Flags().BoolVar(&insecureTLS, "insecureTLS", false, "Ignore TLS certificate errors")
	exportCmd.Flags().Var(&channels, "channel", "Channel name. Ex: 1, 2, C ,D, VP1. Repeat for multiple. Skip for auto-detection")
	exportCmd.Flags().StringVar(&source, "device", "localhost", "Source of the data. Ex: calnex01.example.com")
	if err := exportCmd.MarkFlagRequired("device"); err != nil {
		log.Fatal(err)
	}
}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "export calnex measurement data",
	Run: func(_ *cobra.Command, _ []string) {
		var chs []api.Channel
		for _, channel := range channels {
			chs = append(chs, channel)
		}
		l := export.JSONLogger{Out: os.Stdout}
		if err := export.Export(source, insecureTLS, allData, chs, l); err != nil {
			log.Fatal(err)
		}
	},
}
