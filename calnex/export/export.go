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

package export

import (
	"errors"
	"net"
	"net/url"
	"time"

	"github.com/facebook/time/calnex/api"
	log "github.com/sirupsen/logrus"
)

var errNoUsedChannels = errors.New("no used channels")
var errNoTarget = errors.New("no target succeeds")

// returns true if the error is a hard failure
func isHardFailure(err error) bool {
	var opErr *net.OpError
	var urlErr *url.Error
	if errors.As(err, &opErr) {
		if opErr.Timeout() || !opErr.Temporary() {
			return true
		}
	}
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() || !urlErr.Temporary() {
			return true
		}
	}
	return false
}

// Export data from the device about specified channels to the specified output
func Export(source string, insecureTLS bool, allData bool, channels []api.Channel, l Logger) (err error) {
	var success bool
	calnexAPI := api.NewAPI(source, insecureTLS, 2*time.Minute)

	if len(channels) == 0 {
		channels, err = calnexAPI.FetchUsedChannels()
		if err != nil {
			return errNoUsedChannels
		}
	}

	for _, channel := range channels {
		printSuccess := true

		probe, err := calnexAPI.FetchChannelProbe(channel)
		if err != nil {
			log.Warnf("%s: failed to fetch protocol from channel %s: %v", source, channel, err)
			success = success || false
			if isHardFailure(err) {
				return err
			}
			continue
		}
		target, err := calnexAPI.FetchChannelTarget(channel, *probe)
		if err != nil {
			log.Warnf("%s: failed to fetch target from channel %s: %v", source, channel, err)
			success = success || false
			if isHardFailure(err) {
				return err
			}
			continue
		}
		csvLines, err := calnexAPI.FetchCsv(channel, allData)
		if err != nil {
			log.Warnf("%s: failed to fetch data from channel %s: %v", source, channel, err)
			success = success || false
			if isHardFailure(err) {
				return err
			}
			continue
		}

		for _, csvLine := range csvLines {
			entry, err := entryFromCSV(csvLine, string(channel), target, string(*probe), source)
			if err != nil {
				printSuccess = false
				success = success || printSuccess
				log.Warnf("%s failed to generate scribe line for channel %s: %v", source, channel, err)
				break
			}

			l.PrintEntry(entry)
		}
		success = success || printSuccess
	}

	if !success {
		return errNoTarget
	}

	return nil
}
