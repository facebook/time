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
	"bufio"
	"net"
	"os"
	"regexp"
	"time"

	"github.com/facebook/time/ntp/chrony"
	log "github.com/sirupsen/logrus"
)

// Runner is something that can produce NTPCheckResult
type Runner interface {
	Run() (*NTPCheckResult, error)
	ServerStats() (*ServerStats, error)
}

type flavour int

const (
	flavourNTPD flavour = iota
	flavourChrony
)

const netFile = "/proc/net/udp6"

func getPublicServer(f flavour) string {
	if f == flavourChrony {
		return "[::1]:323"
	}
	return "[::1]:123"
}

func getPrivateServer(f flavour) string {
	if f == flavourChrony {
		return chrony.ChronySocketPath
	}
	return "[::1]:123"
}

func getFlavour() flavour {
	f, err := os.Open(netFile)
	if err != nil {
		return flavourNTPD
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	r := regexp.MustCompile(chrony.ChronyPortV6Regexp)
	for scanner.Scan() {
		if r.MatchString(scanner.Text()) {
			log.Debug("Will use chrony protocol")
			return flavourChrony
		}
	}
	log.Debug("Will use ntp control protocol")
	return flavourNTPD
}

func getChecker(f flavour, conn net.Conn) Runner {
	if f == flavourChrony {
		return NewChronyCheck(conn)
	}
	return NewNTPCheck(conn)
}

// RunCheck is a simple wrapper to connect to address and run NTPCheck.Run()
func RunCheck(address string) (*NTPCheckResult, error) {
	timeout := 5 * time.Second
	deadline := time.Now().Add(timeout)
	flavour := getFlavour()
	if address == "" {
		address = getPublicServer(flavour)
	}
	conn, err := net.DialTimeout("udp", address, timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, err
	}
	checker := getChecker(flavour, conn)
	log.Debugf("connected to %s", address)
	return checker.Run()
}

// RunNTPData is a simple wrapper to connect to address and run NTPCheck.Run()
// If using chrony it gathers extra info about the peers using the unix socket
func RunNTPData(address string) (*NTPCheckResult, error) {
	timeout := 5 * time.Second
	deadline := time.Now().Add(timeout)
	flavour := getFlavour()
	if flavour != flavourChrony {
		// NTPD does not have a separation between public and private
		// protocol. It does not use a unix socket.
		// RunCheck will gather the same information
		return RunCheck(address)
	}
	if address == "" {
		address = getPrivateServer(flavour)
	}
	conn, err := dialUnix(address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, err
	}
	checker := getChecker(flavour, conn)
	log.Debugf("connected to %s", address)
	return checker.Run()
}

// RunServerStats is a simple wrapper to connect to address and run NTPCheck.ServerStats()
func RunServerStats(address string) (*ServerStats, error) {
	var err error
	var conn net.Conn
	timeout := 5 * time.Second
	deadline := time.Now().Add(timeout)
	flavour := getFlavour()
	if address == "" {
		address = getPrivateServer(flavour)
	}
	if flavour == flavourChrony {
		conn, err = dialUnix(address)
	} else {
		conn, err = net.DialTimeout("udp", address, timeout)
	}
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, err
	}
	checker := getChecker(flavour, conn)
	log.Debugf("connected to %s", address)
	return checker.ServerStats()
}
