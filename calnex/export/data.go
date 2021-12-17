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
	"strconv"
	"strings"
)

// Entry is an entire line
type Entry struct {
	Float  *FloatData  `json:"float"`
	Int    *IntData    `json:"int"`
	Normal *NormalData `json:"normal"`
}

// FloatData data with floats
type FloatData struct {
	Value float64 `json:"value"`
}

// IntData data with int
type IntData struct {
	Time int `json:"time"`
}

// NormalData data with normal
type NormalData struct {
	Channel  string `json:"channel"`
	Target   string `json:"target"`
	Protocol string `json:"protocol"`
	Source   string `json:"source"`
}

// Files is a multitype for flag.Var
type Files []string

// entryFromCSV generates Entry from CSV
func entryFromCSV(csvLine []string, channel, target, protocol, source string) (*Entry, error) {
	timestamp, err := strconv.ParseInt(strings.Split(csvLine[0], ".")[0], 10, 64)
	if err != nil {
		return nil, err
	}

	intdata := &IntData{Time: int(timestamp)}

	s, err := strconv.ParseFloat(csvLine[1], 64)
	if err != nil {
		return nil, err
	}
	floatdata := &FloatData{Value: s}

	normaldata := &NormalData{Channel: channel, Target: target, Protocol: protocol, Source: source}

	return &Entry{Float: floatdata, Int: intdata, Normal: normaldata}, nil
}
