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
	"text/template"

	"github.com/facebook/time/phc/unix" // a temporary shim for "golang.org/x/sys/unix" until v0.27.0 is cut
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "nic [eth0]",
		Short: "List timestamping attributes for network interfaces",
		RunE:  runNicCmd,
	}
	RootCmd.AddCommand(cmd)
}

func runNicCmd(_ *cobra.Command, args []string) error {
	ConfigureVerbosity()

	var ifname string
	switch len(args) {
	case 0:
		ifname = "eth0"
	case 1:
		ifname = args[0]
	default:
		return fmt.Errorf("specify only one interface")
	}

	var tmpl = template.Must(template.New("").Parse(`
{{- .Name}} ({{.Phc}})
Supported:
	tx-types: {{.Txcaps}}
	rx-filters: {{.Rxcaps}}
{{if .HasEnabled -}}
Enabled:
	tx-type: {{.Txtype}} ({{printf "%d" .Txtype}})
	rx-filter: {{.Rxfilter}} ({{printf "%d" .Rxfilter}})
{{end}}`))

	var ifstat = struct {
		Name       string
		Phc        string
		Txcaps     TxTypeCaps
		Rxcaps     RxFilterCaps
		HasEnabled bool
		Txtype     TxType
		Rxfilter   RxFilter
	}{Name: ifname, Phc: "-"}

	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
	if err != nil {
		log.Fatal(err)
	}
	defer unix.Close(fd)

	tsinfo, err := unix.IoctlGetEthtoolTsInfo(fd, ifname)
	if err != nil {
		log.Fatalf("%v: IoctlGetEthtoolTsInfo: %v", ifname, err)
	}
	ifstat.Txcaps = TxTypeCaps(tsinfo.Tx_types)
	ifstat.Rxcaps = RxFilterCaps(tsinfo.Rx_filters)
	if tsinfo.Phc_index >= 0 {
		ifstat.Phc = fmt.Sprintf("/dev/ptp%d", tsinfo.Phc_index)
	}

	if tscfg, err := unix.IoctlGetHwTstamp(fd, ifname); err == nil {
		ifstat.Txtype = TxType(tscfg.Tx_type)
		ifstat.Rxfilter = RxFilter(tscfg.Rx_filter)
		ifstat.HasEnabled = true
	}

	return tmpl.Execute(os.Stdout, &ifstat)
}

// TxType represents a value of unix.HwTstampConfig.Tx_type
type TxType int32

var txTypeNames = []string{
	"off",      // unix.HWTSTAMP_TX_OFF
	"on",       // unix.HWTSTAMP_TX_ON
	"stepsync", // unix.HWTSTAMP_TX_ONESTEP_SYNC
}

// String implements fmt.Stringer interface
func (x TxType) String() string {
	if x >= 0 && int(x) < len(txTypeNames) {
		return txTypeNames[x]
	}
	return "?"
}

// TxTypeCaps represents a value of unix.EthtoolTsInfo.Tx_types
type TxTypeCaps uint32

// String implements fmt.Stringer interface
func (c TxTypeCaps) String() string {
	s := ""
	for i, name := range txTypeNames {
		if c&(1<<i) != 0 {
			if s != "" {
				s += ", "
			}
			s += fmt.Sprintf("%s (%d)", name, i)
		}
	}
	if s == "" {
		s = "-"
	}
	return s
}

// RxFilter represents a value of unix.HwTstampConfig.Rx_filter
type RxFilter int32

var rxFilterNames = []string{
	"none",           // unix.HWTSTAMP_FILTER_NONE
	"all",            // unix.HWTSTAMP_FILTER_ALL
	"some",           // unix.HWTSTAMP_FILTER_SOME
	"ptpv1-l4-event", // unix.HWTSTAMP_FILTER_PTP_V1_L4_EVENT
	"ptpv1-l4-sync",
	"ptpv1-l4-delay-req",
	"ptpv2-l4-event", // unix.HWTSTAMP_FILTER_PTP_V2_L4_EVENT
	"ptpv2-l4-sync",
	"ptpv2-l4-delay-req",
	"ptpv2-l2-event", // unix.HWTSTAMP_FILTER_PTP_V2_L2_EVENT
	"ptpv2-l2-sync",
	"ptpv2-l2-delay-req",
	"ptpv2-event", // unix.HWTSTAMP_FILTER_PTP_V2_EVENT
	"ptpv2-sync",
	"ptpv2-delay-req",
}

// String implements fmt.Stringer interface
func (f RxFilter) String() string {
	if f >= 0 && int(f) < len(rxFilterNames) {
		return rxFilterNames[f]
	}
	return "?"
}

// RxFilterCaps represents a value of unix.EthtoolTsInfo.Rx_filters
type RxFilterCaps uint32

// String implements fmt.Stringer interface
func (c RxFilterCaps) String() string {
	s := ""
	for i, name := range rxFilterNames {
		if c&(1<<i) != 0 {
			if s != "" {
				s += ", "
			}
			s += fmt.Sprintf("%s (%d)", name, i)
		}
	}
	if s == "" {
		s = "-"
	}
	return s
}
