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

// RootCmd is a main entry point. It's exported so ntpcheck could be easily extended without touching core functionality.
var RootCmd = &cobra.Command{
	Use:   "calnex",
	Short: "collection of calnex utilities",
}

var (
	allData     bool
	apply       bool
	channels    api.Channels
	dir         string
	force       bool
	insecureTLS bool
	saveConfig  string
	source      string
	target      string
)

// Execute is the main entry point for CLI interface
func Execute() {
	log.SetLevel(log.DebugLevel)
	if err := RootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
