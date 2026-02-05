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

package checker

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"

	ptp "github.com/facebook/time/ptp/protocol"
)

// Flavour is the type of PTP client
type Flavour int

// Supported PTP client flavours
const (
	FlavourPTP4L Flavour = iota
	FlavourSPTP
)

func isPTP4lListening() bool {
	var err error
	if _, err = os.Stat(ptp.PTP4lSock); err == nil {
		return true
	}
	log.Debugf("checking for ptp4l socket: %v", err)
	return false
}

// GetFlavour returns detected flavour of ptp client
func GetFlavour() Flavour {
	if isPTP4lListening() {
		log.Debug("Will use PTP4L")
		return FlavourPTP4L
	}
	log.Debug("Will use SPTP")
	return FlavourSPTP
}

// GetServerAddress returns the address to talk to the client, based on flavour and manual address override
func GetServerAddress(address string, f Flavour) string {
	if address != "" {
		return address
	}
	if f == FlavourPTP4L {
		return ptp.PTP4lSock
	}
	return "http://[::1]:4269"
}

// RunCheck is a simple wrapper to connect to address and run Run()
func RunCheck(address string, domainNumber uint8) (*PTPCheckResult, error) {
	flavour := GetFlavour()
	address = GetServerAddress(address, flavour)
	log.Debugf("using address %q", address)
	switch flavour {
	case FlavourPTP4L:
		c, cleanup, err := PrepareMgmtClient(address)
		defer cleanup()
		if err != nil {
			return nil, err
		}
		c.SetDomainNumber(domainNumber)
		log.Debugf("connected to %s", address)
		return RunPTP4L(c)
	case FlavourSPTP:
		return RunSPTP(address)
	}
	return nil, fmt.Errorf("unknown PTP client flavour %v", flavour)
}
