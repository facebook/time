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
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/facebook/time/ptp/phc"
)

func ptpDeviceNum(ptpPath string) (int, error) {
	basePath := filepath.Base(ptpPath)
	ptpPath = filepath.Join("/dev", basePath)
	realPath, err := filepath.EvalSymlinks(ptpPath)
	if err != nil {
		return 0, err
	}
	realBasePath := filepath.Base(realPath)
	if realBasePath != basePath {
		log.Infof("%s is %s", ptpPath, realPath)
	}
	return strconv.Atoi(strings.TrimLeftFunc(realBasePath, func(r rune) bool {
		return !unicode.IsNumber(r)
	}))
}

func printIfaceData(d phc.IfaceData, reverse bool) {
	if d.TSInfo.PHCIndex < 0 {
		fmt.Printf("No PHC support for %s\n", d.Iface.Name)
		return
	}
	if reverse {
		fmt.Printf("/dev/ptp%d -> %s\n", d.TSInfo.PHCIndex, d.Iface.Name)
		return
	}
	fmt.Printf("%s -> /dev/ptp%d\n", d.Iface.Name, d.TSInfo.PHCIndex)
}

func getDevice(iface string) error {
	ifaces, err := phc.IfacesInfo()
	if err != nil {
		return err
	}
	if len(ifaces) == 0 {
		return fmt.Errorf("no network devices found")
	}
	found := []phc.IfaceData{}
	for _, d := range ifaces {
		if d.Iface.Name == iface {
			found = append(found, d)
		}
	}
	if len(found) == 0 {
		return fmt.Errorf("no nic information for %s", iface)
	}
	for _, d := range found {
		printIfaceData(d, false)
	}
	return nil

}

func getIface(ptpDevice int) error {
	ifaces, err := phc.IfacesInfo()
	if err != nil {
		return err
	}
	if len(ifaces) == 0 {
		return fmt.Errorf("no network devices found")
	}
	found := []phc.IfaceData{}
	if ptpDevice < 0 {
		found = ifaces
	} else {
		for _, d := range ifaces {
			if int(d.TSInfo.PHCIndex) == ptpDevice {
				found = append(found, d)
			}
		}
	}
	if len(found) == 0 {
		return fmt.Errorf("no nic found for /dev/ptp%d", ptpDevice)
	}
	for _, d := range found {
		printIfaceData(d, true)
	}
	return nil
}

var mapIfaceFlag bool

func init() {
	RootCmd.AddCommand(mapCmd)
	mapCmd.Flags().BoolVarP(&mapIfaceFlag, "iface", "i", false, "Treat args as network interfaces")
}

var mapCmd = &cobra.Command{
	Use:   "map [ptp device/network interface]...",
	Short: "Find network interfaces for ptp devices and vice versa",
	Run: func(cmd *cobra.Command, args []string) {
		ConfigureVerbosity()
		// no args - just print map of all ptp devices to all interfaces
		if len(args) == 0 {
			if err := getIface(-1); err != nil {
				log.Fatal(err)
			}
		}
		// treat args as network interfaces
		if mapIfaceFlag {
			for _, arg := range args {
				if err := getDevice(arg); err != nil {
					log.Fatal(err)
				}

			}
			return
		}
		// map from ptp device to network interface
		for _, arg := range args {
			i, err := ptpDeviceNum(arg)
			if err != nil {
				log.Fatal(err)
			}
			if err := getIface(i); err != nil {
				log.Fatal(err)
			}
		}
	},
}
