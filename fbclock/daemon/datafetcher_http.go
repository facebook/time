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

package daemon

import (
	"fmt"

	"github.com/facebook/time/ptp/sptp/stats"

	log "github.com/sirupsen/logrus"
)

// HTTPFetcher provides data fetcher implementation using http
type HTTPFetcher struct {
	DataFetcher
}

// FetchGMs fetches GMs via http
func (hf *HTTPFetcher) FetchGMs(cfg *Config) (targets []string, err error) {
	url := fmt.Sprintf("http://%s/", cfg.PTPClientAddress)
	sm, err := stats.FetchStats(url)
	if err != nil {
		return nil, err
	}

	for gmIP, entry := range sm {
		// skip the current best master
		if entry.Selected {
			continue
		}
		// skip GMs we didn't get announce from
		if entry.Error != "" {
			continue
		}
		targets = append(targets, gmIP)
	}
	return
}

// FetchStats fetches GMs via http
func (hf *HTTPFetcher) FetchStats(cfg *Config) (*DataPoint, error) {
	url := fmt.Sprintf("http://%s/", cfg.PTPClientAddress)
	sm, err := stats.FetchStats(url)
	if err != nil {
		return nil, err
	}
	log.Debugf("TIME_STATUS_NP: %+v", sm)

	for _, s := range sm {
		if s.Selected {
			accuracyNS := s.ClockQuality.ClockAccuracy.Duration().Nanoseconds()
			return &DataPoint{
				IngressTimeNS:   s.IngressTime,
				MasterOffsetNS:  s.Offset,
				PathDelayNS:     s.MeanPathDelay,
				ClockAccuracyNS: float64(int64(s.GMPresent) * accuracyNS),
			}, nil
		}
	}
	return nil, fmt.Errorf("no selected grandmaster")
}
