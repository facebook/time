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
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// RootCmd is a main entry point. It's exported so ptpcheck could be easily extended without touching core functionality.
var RootCmd = &cobra.Command{
	Use:   "ptpcheck",
	Short: "Swiss Army Knife for PTP",
}

// flags
var rootVerboseFlag bool
var rootClientFlag string

var rootClientFlagDesc = "Address of PTP client to connect to. Can be either Unix socket for ptp4l or http endpoint for sptp. Empty means detect automatically."

func init() {
	RootCmd.PersistentFlags().BoolVarP(&rootVerboseFlag, "verbose", "v", false, "verbose output")
}

// ConfigureVerbosity configures log verbosity based on parsed flags. Needs to be called by any subcommand.
func ConfigureVerbosity() {
	log.SetLevel(log.InfoLevel)
	if rootVerboseFlag {
		log.SetLevel(log.DebugLevel)
	}
}

// Execute is the main entry point for CLI interface
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
