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
	"encoding/json"
	"fmt"

	"github.com/facebook/time/calnex/api"
	"github.com/facebook/time/calnex/export"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(exportCmd)
	exportCmd.Flags().StringArrayVar(&channels, "channel", []string{}, "Channel name. Ex: 1, 2, c ,d, VP1. Repeat for multiple. Skip for auto-detection")
	exportCmd.Flags().BoolVar(&insecureTLS, "insecureTLS", false, "Ignore TLS certificate errors")
	exportCmd.Flags().StringVar(&source, "source", "localhost", "Source of the data. Ex: calnex01.example.com")
	if err := exportCmd.MarkFlagRequired("source"); err != nil {
		log.Fatal(err)
	}
}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "export calnex measurement data",
	Run: func(cmd *cobra.Command, args []string) {
		var chs []api.Channel
		for _, channel := range channels {
			c, err := api.ChannelFromString(channel)
			if err != nil {
				log.Fatal(err)
			}
			chs = append(chs, *c)
		}
		handler := func(e *export.Entry) {
			entryj, _ := json.Marshal(e)
			fmt.Println(string(entryj))
		}
		if err := export.Export(source, insecureTLS, chs, handler); err != nil {
			log.Fatal(err)
		}
	},
}
