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

package api

import (
	"errors"
	"strings"
)

var errBadAPIProto = errors.New("api protocol is not recognized")

// APIProto is a protocol to communicate with Calnex API
type APIProto int

// Protocols we support
const (
	HTTPS APIProto = iota
	HTTP
)

// Set the network protocol
func (p *APIProto) Set(value string) error {
	switch value {
	case "https":
		*p = HTTPS
		return nil
	case "http":
		*p = HTTP
		return nil
	default:
		return errBadAPIProto
	}
}

var protoToString = map[APIProto]string{
	HTTPS: "https",
	HTTP:  "http",
}

// String returns the protocol name
func (p APIProto) String() string {
	return protoToString[p]
}

// Type returns possible types
func (p APIProto) Type() string {
	var ret []string
	for _, p := range protoToString {
		ret = append(ret, p)
	}
	return strings.Join(ret, ", ")
}
