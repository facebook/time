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
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
)

// port stats prefixes
const (
	PortStatsTxPrefix = "sptp.portstats.tx."
	PortStatsRxPrefix = "sptp.portstats.rx."
)

// Stats is a representation of a monitoring struct for sptp client
type Stats struct {
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

// FetchStats returns populated Stats structure fetched from the url
func FetchStats(url string) (map[string]Stats, error) {
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

	s := make(map[string]Stats)
	err = json.Unmarshal(b, &s)

	return s, err
}

// FetchCounters returns counters map fetched from the url
func FetchCounters(url string) (map[string]float64, error) {
	counters := make(map[string]float64)
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
