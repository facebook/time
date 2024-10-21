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
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/facebook/time/phc/unix" // a temporary shim for "golang.org/x/sys/unix" until v0.27.0 is cut
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
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

func printIfaceData(ifname string, tsinfo *unix.EthtoolTsInfo, reverse bool) {
	if tsinfo.Phc_index < 0 {
		fmt.Printf("No PHC support for %s\n", ifname)
		return
	}
	if reverse {
		fmt.Printf("/dev/ptp%d -> %s\n", tsinfo.Phc_index, ifname)
	} else {
		fmt.Printf("%s -> /dev/ptp%d\n", ifname, tsinfo.Phc_index)
	}
}

func getDevice(ifname string) error {
	tsinfo, err := unix.IoctlGetEthtoolTsInfo(mapIofd, ifname)
	if err != nil {
		return fmt.Errorf("%v: IoctlGetEthtoolTsInfo: %w", ifname, err)
	}
	printIfaceData(ifname, tsinfo, false)
	return nil
}

func getIface(ptpDevice int) error {
	ifaces, err := net.Interfaces()
	if err != nil {
		return err
	}
	n := 0
	for _, iface := range ifaces {
		tsinfo, err := unix.IoctlGetEthtoolTsInfo(mapIofd, iface.Name)
		if err != nil {
			log.Errorf("%v: IoctlGetEthtoolTsInfo: %v", iface.Name, err)
			continue
		}
		if int(tsinfo.Phc_index) == ptpDevice || ptpDevice < 0 {
			printIfaceData(iface.Name, tsinfo, true)
			n++
		}
	}
	if ptpDevice >= 0 && n == 0 {
		return fmt.Errorf("no nic found for /dev/ptp%d", ptpDevice)
	}
	return nil
}

var (
	mapIofd      int
	mapIfaceFlag bool
)

func init() {
	RootCmd.AddCommand(mapCmd)
	mapCmd.Flags().BoolVarP(&mapIfaceFlag, "iface", "i", false, "Treat args as network interfaces")
}

var mapCmd = &cobra.Command{
	Use:   "map [ptp device/network interface]...",
	Short: "Find network interfaces for ptp devices and vice versa",
	Run: func(_ *cobra.Command, args []string) {
		var err error
		ConfigureVerbosity()

		mapIofd, err = unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
		if err != nil {
			log.Fatal(err)
		}
		defer unix.Close(mapIofd)

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
