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
	"io"
	"os"

	"github.com/facebook/time/calnex/config"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(configCmd)
	configCmd.Flags().BoolVar(&apply, "apply", false, "apply the config changes")
	configCmd.Flags().BoolVar(&insecureTLS, "insecureTLS", false, "Ignore TLS certificate errors")
	configCmd.Flags().StringVar(&target, "target", "", "device to configure")
	configCmd.Flags().StringVar(&source, "file", "", "configuration file")
	configCmd.Flags().StringVar(&saveConfig, "save", "", "save configuration to the specified path")
	if err := configCmd.MarkFlagRequired("target"); err != nil {
		log.Fatal(err)
	}
	if err := configCmd.MarkFlagRequired("file"); err != nil {
		log.Fatal(err)
	}
	configCmd.MarkFlagsMutuallyExclusive("apply", "save")
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "configure a calnex appliance",
	Run: func(cmd *cobra.Command, args []string) {
		configFile, err := os.Open(source)
		if err != nil {
			log.Fatal(err)
		}
		defer configFile.Close()
		b, err := io.ReadAll(configFile)
		if err != nil {
			log.Fatal(err)
		}

		var cs config.Calnexes
		err = json.Unmarshal(b, &cs)
		if err != nil {
			log.Fatal(err)
		}

		dc, ok := cs[target]
		if !ok {
			log.Fatalf("Failed to find config for %s in %s", target, source)
		}

		if saveConfig != "" {
			if err := config.Save(target, insecureTLS, dc, saveConfig); err != nil {
				log.Fatal(err)
			}
		} else {
			if err := config.Config(target, insecureTLS, dc, apply); err != nil {
				log.Fatal(err)
			}
		}
	},
}
