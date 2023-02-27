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

package stats

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
)

// port stats prefixes
const (
	PortStatsTxPrefix = "ptp.sptp.portstats.tx."
	PortStatsRxPrefix = "ptp.sptp.portstats.rx."
)

// Stat is a representation of a monitoring struct for sptp client
type Stat struct {
	GMAddress         string           `json:"gm_address"`
	ClockQuality      ptp.ClockQuality `json:"clock_quality"`
	Error             string           `json:"error"`
	GMPresent         int              `json:"gm_present"`
	IngressTime       int64            `json:"ingress_time"`
	MeanPathDelay     float64          `json:"mean_path_delay"`
	Offset            float64          `json:"offset"`
	PortIdentity      string           `json:"port_identity"`
	Priority1         uint8            `json:"priority1"`
	Priority2         uint8            `json:"priority2"`
	Priority3         uint8            `json:"priority3"`
	Selected          bool             `json:"selected"`
	StepsRemoved      int              `json:"steps_removed"`
	CorrectionFieldRX int64            `json:"cf_rx"`
	CorrectionFieldTX int64            `json:"cf_tx"`
}

// Stats is a list of Stat
type Stats []*Stat

func (s Stats) Len() int { return len(s) }
func (s Stats) Less(i, j int) bool {
	if s[i].Priority3 == s[j].Priority3 {
		return s[i].GMAddress < s[j].GMAddress
	}
	return s[i].Priority3 < s[j].Priority3
}
func (s Stats) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// Index returns the index of the e if it's already in s. Otherwise -1
func (s Stats) Index(e *Stat) int {
	for i, a := range s {
		if a.GMAddress == e.GMAddress {
			return i
		}
	}
	return -1
}

// Counters is various counters exported by SPTP client
type Counters map[string]int64

// PortStats returns two maps: packet type to counter, TX and RX
func (c Counters) PortStats() (tx map[string]uint64, rx map[string]uint64) {
	tx = map[string]uint64{}
	rx = map[string]uint64{}
	for k, v := range c {
		if strings.HasPrefix(k, PortStatsTxPrefix) {
			tx[strings.TrimPrefix(k, PortStatsTxPrefix)] = uint64(v)
		}
		if strings.HasPrefix(k, PortStatsRxPrefix) {
			rx[strings.TrimPrefix(k, PortStatsRxPrefix)] = uint64(v)
		}
	}
	return
}

// SysStats return sys stats from counters
func (c Counters) SysStats() map[string]int64 {
	res := map[string]int64{}
	for k, v := range c {
		if strings.HasPrefix(k, PortStatsTxPrefix) {
			continue
		}
		if strings.HasPrefix(k, PortStatsRxPrefix) {
			continue
		}
		res[k] = v
	}
	return res
}

// FetchStats returns populated Stats structure fetched from the url
func FetchStats(url string) (Stats, error) {
	c := http.Client{
		Timeout: time.Second * 2,
	}

	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var s Stats
	err = json.Unmarshal(b, &s)

	return s, err
}

// FetchCounters returns counters map fetched from the url
func FetchCounters(url string) (Counters, error) {
	counters := make(Counters)
	url = fmt.Sprintf("%s/counters", url)
	c := http.Client{
		Timeout: time.Second * 2,
	}

	resp, err := c.Get(url)
	if err != nil {
		return counters, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return counters, err
	}
	err = json.Unmarshal(b, &counters)
	return counters, err
}

// FetchPortStats fetches all counters and then returns two maps: packet type to counter, TX and RX
func FetchPortStats(url string) (tx map[string]uint64, rx map[string]uint64, err error) {
	counters, err := FetchCounters(url)
	if err != nil {
		return nil, nil, err
	}
	tx, rx = counters.PortStats()
	return tx, rx, err
}

// FetchSysStats fetches all counters and return sys stats from them
func FetchSysStats(url string) (map[string]int64, error) {
	counters, err := FetchCounters(url)
	if err != nil {
		return nil, err
	}
	return counters.SysStats(), nil
}
