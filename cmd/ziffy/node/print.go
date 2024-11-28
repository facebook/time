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

package node

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/olekukonko/tablewriter"
	log "github.com/sirupsen/logrus"
)

type status int

const (
	tcTrue = iota
	tcFalse
	tcNA
)

var statusToString = map[status]string{
	tcTrue:  "On",
	tcFalse: "Off",
	tcNA:    "Unknown",
}

const maxColWidth = 100

type keyPair struct {
	host string
	hop  int
}

// getHostNoPrefix has ip as input and returns device hostname without interface prefix
func getHostNoPrefix(ip string) string {
	lun := getLookUpName(ip)
	if strings.Contains(lun, ".") && lun[:3] == "eth" {
		return lun[strings.Index(lun, ".")+1:]
	}
	return lun
}

// getHostIfPrefix has ip as input and returns device interface without suffix
func getHostIfPrefix(ip string) string {
	lun := getLookUpName(ip)
	if strings.Contains(lun, ".") && lun[:3] == "eth" {
		return lun[:strings.Index(lun, ".")]
	}
	return ""
}

// PrettyPrint formats and prints the output to stdout
func PrettyPrint(c *Config, routes []*PathInfo, cfThreshold ptp.Correction) {
	debugPrint(routes)
	info := computeInfo(c, routes, cfThreshold)
	detailedPrint(info)
}

func debugPrint(routes []*PathInfo) {
	var notTCEnabled []SwitchTrafficInfo
	tested := make(map[string]bool)
	enabled := 0

	for _, route := range routes {
		for swIndex, swh := range route.switches {
			if swIndex == len(route.switches)-1 {
				continue
			}
			corrField := route.switches[swIndex+1].corrField - route.switches[swIndex].corrField
			if corrField == ptp.Correction(0) && !tested[swh.ip] {
				notTCEnabled = append(notTCEnabled, swh)
			}
			tested[swh.ip] = true
			enabled++
		}
	}

	log.Debugf("%v switches tested: %v TC enabled | %v TC not enabled", len(tested), enabled, len(notTCEnabled))
	for _, brkSw := range notTCEnabled {
		log.Debugf("%v: PTP TC not enabled", getLookUpName(brkSw.ip))
		for index, swh := range routes[brkSw.routeIdx].switches {
			if index != len(routes[brkSw.routeIdx].switches)-1 {
				log.Debugf(" | %v", getLookUpName(swh.ip))
			} else {
				log.Debugf(" V %v", getLookUpName(swh.ip))
			}
		}
	}

	log.Debug("TESTED:")
	aux := make([]string, 0, len(tested))
	for key := range tested {
		aux = append(aux, getLookUpName(key))
	}
	sort.Slice(aux, func(i, j int) bool {
		return aux[i] > aux[j]
	})
	for index, element := range aux {
		log.Debugf("%v %v", index, element)
	}
}

func correctionField(c *Config, swIndex int, switches []SwitchTrafficInfo) ptp.Correction {
	if !c.Raw {
		return switches[swIndex+1].corrField - switches[swIndex].corrField
	}
	return switches[swIndex+1].corrField
}

func computeInfo(c *Config, routes []*PathInfo, cfThreshold ptp.Correction) map[keyPair]*SwitchPrintInfo {
	discovered := make(map[keyPair]*SwitchPrintInfo)

	for _, route := range routes {
		for swIndex, swh := range route.switches {
			corrField := ptp.Correction(0)
			last := false
			host := getHostNoPrefix(swh.ip)
			intf := getHostIfPrefix(swh.ip)
			sp := swh.routeIdx + c.SourcePort

			if swh.ip == route.switches[len(route.switches)-1].ip {
				last = true
			} else {
				// If switches are not adjacent (hop count difference > 1) do not subtract CF
				// This may happen in two cases: switch does not send ICMP Hop Limit Exceeded
				// back to source or the packet may be lost in transit
				if route.switches[swIndex+1].hop != route.switches[swIndex].hop+1 {
					last = true
				} else {
					corrField = correctionField(c, swIndex, route.switches)
				}
			}

			if swh.hop == 1 && route.rackSwHostname != "" {
				host = route.rackSwHostname
			}

			pair := keyPair{
				host: host,
				hop:  swh.hop,
			}
			if discovered[pair] == nil {
				discovered[pair] = &SwitchPrintInfo{
					ip:        swh.ip,
					hostname:  host,
					sampleSP:  sp,
					interf:    intf,
					totalCF:   corrField,
					routes:    1,
					hop:       swh.hop,
					last:      last,
					maxCF:     corrField,
					minCF:     corrField,
					divRoutes: 1,
				}
			} else {
				discovered[pair].routes++
				if !last {
					discovered[pair].totalCF += corrField
					discovered[pair].divRoutes++
					if discovered[pair].maxCF < corrField {
						discovered[pair].maxCF = corrField
					}
					if discovered[pair].minCF > corrField {
						discovered[pair].minCF = corrField
					}
				}
			}
		}
	}
	for _, sw := range discovered {
		if sw.last {
			sw.avgCF = ptp.Correction(0)
			sw.tcEnable = tcNA
			continue
		}
		sw.avgCF = sw.totalCF / ptp.Correction(sw.divRoutes)

		avgCFNs := sw.avgCF.Nanoseconds()
		cfThresholdNs := cfThreshold.Nanoseconds()
		if math.Abs(avgCFNs) > cfThresholdNs {
			sw.tcEnable = tcTrue
		} else {
			sw.tcEnable = tcFalse
		}
	}
	return discovered
}

func parseSwitchMap(info map[keyPair]*SwitchPrintInfo) []SwitchPrintInfo {
	aux := make([]SwitchPrintInfo, 0, len(info))
	for _, val := range info {
		aux = append(aux, *val)
	}
	sort.Slice(aux, func(i, j int) bool {
		return aux[i].hop < aux[j].hop
	})
	return aux
}

func hopCount(sw []SwitchPrintInfo, hop int) int {
	width := 0
	for _, val := range sw {
		if val.hop == hop {
			width++
		}
	}
	return width
}

func colNumber(header []string, colName string) (int, error) {
	for ind, val := range header {
		if val == colName {
			return ind, nil
		}
	}
	return -1, fmt.Errorf("no column with this name")
}

func computePrintData(sw []SwitchPrintInfo) [][]string {
	ret := [][]string{
		{"uniq", "width", "hop", "ip_address", "sample_sp", "intf", "hostname", "flows", "TC", "avg_CF(ns)", "max_CF(ns)", "min_CF(ns)"},
	}

	// unique counts number of devices discovered
	unique := 1
	already := make(map[string]bool)

	for _, val := range sw {
		avgDisplay := strconv.FormatFloat(val.avgCF.Nanoseconds(), 'f', 4, 64)
		minDisplay := strconv.FormatFloat(val.minCF.Nanoseconds(), 'f', 4, 64)
		maxDisplay := strconv.FormatFloat(val.maxCF.Nanoseconds(), 'f', 4, 64)

		uniqDisplay := ""
		if val.last {
			avgDisplay = ""
			minDisplay = ""
			maxDisplay = ""
		}
		if !already[val.hostname] {
			uniqDisplay = strconv.Itoa(unique)
			already[val.hostname] = true
			unique++
		}
		ret = append(ret, []string{uniqDisplay, strconv.Itoa(hopCount(sw, val.hop)), strconv.Itoa(val.hop), val.ip, strconv.Itoa(val.sampleSP), val.interf, val.hostname,
			strconv.Itoa(val.routes), statusToString[val.tcEnable], avgDisplay, maxDisplay, minDisplay})
	}
	return ret
}

func detailedPrint(info map[keyPair]*SwitchPrintInfo) {
	aux := parseSwitchMap(info)
	data := computePrintData(aux)

	// currentHop is incremented each time we pass to the next hop. Used to print newline between hops
	currentHop := 0
	// headerRow is the row index for the header
	headerRow := 0
	// blank space for data
	blank := []string{"", "", "", "", "", "", "", "", "", "", ""}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetColWidth(maxColWidth)
	table.SetHeader(data[headerRow])

	for _, val := range data[headerRow+1:] {
		hopInd, err := colNumber(data[headerRow], "hop")
		if err != nil {
			log.Errorf("detailedPrint failed: %v", err)
			return
		}

		nextHop, err := strconv.Atoi(val[hopInd])
		if err != nil {
			log.Errorf("strconv atoi failed: %v", err)
			return
		}

		if nextHop > currentHop {
			table.Append(blank)
		}
		table.Append(val)

		currentHop = nextHop
	}
	table.Render()
}

// CsvPrint outputs the data in a csv file
func CsvPrint(c *Config, routes []*PathInfo, path string, cfThreshold ptp.Correction) {
	info := computeInfo(c, routes, cfThreshold)
	aux := parseSwitchMap(info)
	data := computePrintData(aux)

	file, err := os.Create(path)
	if err != nil {
		log.Errorf("unable to open file: %v", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	for _, val := range data {
		if err := writer.Write(val); err != nil {
			log.Debugf("unable to write to file: %v", err)
		}
	}
}
