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
	"io/ioutil"
	"os"

	"github.com/facebook/time/calnex/api"
	"github.com/facebook/time/calnex/config"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(configCmd)
	configCmd.Flags().BoolVar(&apply, "apply", false, "apply the config changes")
	configCmd.Flags().Var(&aproto, "proto", "API protocol to communicate with Calnex device")
	configCmd.Flags().StringVar(&target, "target", "", "device to configure")
	configCmd.Flags().StringVar(&source, "file", "", "configuration file")
	if err := configCmd.MarkFlagRequired("target"); err != nil {
		log.Fatal(err)
	}
	if err := configCmd.MarkFlagRequired("file"); err != nil {
		log.Fatal(err)
	}
}

type measureConfig struct {
	Target string
	Probe  string
}

type deviceConfig struct {
	Network *config.NetworkConfig
	Measure map[string]measureConfig
}

type devices map[string]deviceConfig

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "configure a calnex appliance",
	Run: func(cmd *cobra.Command, args []string) {
		configFile, err := os.Open(source)
		if err != nil {
			log.Fatal(err)
		}
		defer configFile.Close()
		b, err := ioutil.ReadAll(configFile)
		if err != nil {
			log.Fatal(err)
		}

		var d devices
		err = json.Unmarshal(b, &d)
		if err != nil {
			log.Fatal(err)
		}

		dc, ok := d[target]
		if !ok {
			log.Fatalf("Failed to find config for %s in %s", target, source)
		}

		cc, err := calnexConfig(dc.Measure)
		if err != nil {
			log.Fatal(err)
		}

		if err := config.Config(aproto, target, dc.Network, cc, apply); err != nil {
			log.Fatal(err)
		}
	},
}

func calnexConfig(mc map[string]measureConfig) (config.CalnexConfig, error) {
	c := config.CalnexConfig{}
	for ch, m := range mc {
		channel, err := api.ChannelFromString(ch)
		if err != nil {
			return nil, err
		}

		probe, err := api.ProbeFromString(m.Probe)
		if err != nil {
			return nil, err
		}
		c[*channel] = config.MeasureConfig{
			Target: m.Target,
			Probe:  *probe,
		}
	}
	return c, nil
}
