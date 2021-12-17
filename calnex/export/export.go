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
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/facebook/time/calnex/api"
	log "github.com/sirupsen/logrus"
)

var errNoUsedChannels = errors.New("no used channels")
var errNoTarget = errors.New("no target succeeds")

// Export data from the device about specified channels via protocol to the output
func Export(aproto api.APIProto, source string, channels []api.Channel, output io.WriteCloser) (err error) {
	var success bool
	calnexAPI := api.NewAPI(aproto, source)

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
			log.Errorf("Failed to fetch protocol from the channel %s: %v", channel, err)
			success = success || false
			continue
		}

		target, err := calnexAPI.FetchChannelTargetName(channel, *probe)
		if err != nil {
			log.Errorf("Failed to fetch target from the channel %s: %v", channel, err)
			success = success || false
			continue
		}

		csvLines, err := calnexAPI.FetchCsv(channel)
		if err != nil {
			log.Errorf("Failed to fetch data from channel %s: %v", channel, err)
			success = success || false
			continue
		}

		for _, csvLine := range csvLines {
			entry, err := entryFromCSV(csvLine, channel.String(), target, probe.String(), source)
			if err != nil {
				printSuccess = false
				success = success || printSuccess
				log.Errorf("Failed to generate scribe line for data channel %s: %v", channel, err)
				break
			}

			entryj, _ := json.Marshal(entry)
			fmt.Fprintln(output, string(entryj))
		}
		success = success || printSuccess
	}

	if !success {
		return errNoTarget
	}

	return nil
}
