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
	"fmt"
	"net"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/facebookincubator/ntp/protocol/chrony"
	log "github.com/sirupsen/logrus"
)

// Runner is something that can produce NTPCheckResult
type Runner interface {
	Run() (*NTPCheckResult, error)
}

type flavour int

const (
	flavourNTPD flavour = iota
	flavourChrony
)

const netFile = "/proc/net/udp6"

func getDefaultServer(f flavour) string {
	if f == flavourChrony {
		_, err := os.Stat(chrony.ChronySocketPath)
		if err == nil {
			return chrony.ChronySocketPath
		}
		log.Debug("Unable to use chrony socket for communication, operating in degraded mode")
		return "[::1]:323"
	}
	return "[::1]:123"
}

func getFlavour() flavour {
	// chronyd can be listening to udp6 intermittently,
	// it is safe to talk to chronyd over unix socket
	_, err := os.Stat(chrony.ChronySocketPath)
	if err == nil {
		log.Debug("Will use chrony protocol (socket file found)")
		return flavourChrony
	}

	// fallback to udp detetcion, such as when running from tupperware
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
	var checker Runner
	var err error
	var conn net.Conn
	timeout := 5 * time.Second
	deadline := time.Now().Add(timeout)
	flavour := getFlavour()
	if address == "" {
		address = getDefaultServer(flavour)
	}
	if strings.HasPrefix(address, "/") {
		addr, err := net.ResolveUnixAddr("unixgram", address)
		if err != nil {
			return nil, err
		}
		base, _ := path.Split(address)
		local := path.Join(base, fmt.Sprintf("chronyc.%d.sock", os.Getpid()))
		localAddr, _ := net.ResolveUnixAddr("unixgram", local)
		conn, err = net.DialUnix("unixgram", localAddr, addr)
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		defer os.RemoveAll(local)
		if err := os.Chmod(local, 0666); err != nil {
			return nil, err
		}
		if err := conn.SetReadDeadline(deadline); err != nil {
			return nil, err
		}
	} else {
		conn, err = net.DialTimeout("udp", address, timeout)
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		if err := conn.SetReadDeadline(deadline); err != nil {
			return nil, err
		}
	}
	checker = getChecker(flavour, conn)
	log.Debugf("connected to %s", address)
	return checker.Run()
}
