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

package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/facebook/time/ntp/chrony"
	log "github.com/sirupsen/logrus"
)

// dialUnix opens a unixgram connection with chrony
func dialUnix(address string) (*net.UnixConn, string, error) {
	base, _ := path.Split(address)
	local := path.Join(base, fmt.Sprintf("testchrony.%d.sock", os.Getpid()))
	conn, err := net.DialUnix("unixgram",
		&net.UnixAddr{Name: local, Net: "unixgram"},
		&net.UnixAddr{Name: address, Net: "unixgram"},
	)
	if err != nil {
		return nil, "", err
	}
	if err := os.Chmod(local, 0666); err != nil {
		return nil, "", err
	}
	return conn, local, nil
}

// all commands implemented in our chrony protocol implementation
var commands = []string{
	"sourcedata",
	"tracking",
	"sourcestats",
	"activity",
	"serverstats",
	"ntpdata",
	"sourcename",
	"selectdata",
}

func runCommand(address string, cmd string) error {
	var conn net.Conn
	var err error
	var local string
	if strings.HasPrefix(address, "/") {
		conn, local, err = dialUnix(address)
		if err != nil {
			return err
		}
		defer os.Remove(local)
		defer conn.Close()
	} else {
		conn, err = net.DialTimeout("udp", address, time.Second)
		if err != nil {
			return err
		}
		defer conn.Close()
	}
	client := &chrony.Client{Sequence: 1, Connection: conn}

	fmt.Printf("Testing %q command\n\n", cmd)

	// make sure to add new commands to the list above
	switch cmd {
	case "sourcedata":
		req := chrony.NewSourcesPacket()
		response, err := client.Communicate(req)
		if err != nil {
			return err
		}
		nSources := response.(*chrony.ReplySources).NSources
		fmt.Printf("Got %d sources\n", nSources)
		for i := range nSources {
			req := chrony.NewSourceDataPacket(int32(i))
			sourceData, err := client.Communicate(req)
			if err != nil {
				return err
			}
			fmt.Printf("Source %d: %+v\n", i, sourceData)
		}
	case "tracking":
		req := chrony.NewTrackingPacket()
		response, err := client.Communicate(req)
		if err != nil {
			return err
		}
		fmt.Printf("%+v\n", response)
	case "sourcestats":
		req := chrony.NewSourcesPacket()
		response, err := client.Communicate(req)
		if err != nil {
			return err
		}
		nSources := response.(*chrony.ReplySources).NSources
		fmt.Printf("Got %d sources\n", nSources)
		for i := range nSources {
			req := chrony.NewSourceStatsPacket(int32(i))
			selectData, err := client.Communicate(req)
			if err != nil {
				return err
			}
			fmt.Printf("Source %d: %+v\n", i, selectData)
		}
	case "activity":
		req := chrony.NewActivityPacket()
		response, err := client.Communicate(req)
		if err != nil {
			return err
		}
		fmt.Printf("%+v\n", response)
	case "serverstats":
		req := chrony.NewServerStatsPacket()
		response, err := client.Communicate(req)
		if err != nil {
			return err
		}
		fmt.Printf("%+v\n", response)
	case "ntpdata":
		req := chrony.NewSourcesPacket()
		response, err := client.Communicate(req)
		if err != nil {
			return err
		}
		nSources := response.(*chrony.ReplySources).NSources
		fmt.Printf("Got %d sources\n", nSources)
		for i := range nSources {
			sourceDataReq := chrony.NewSourceDataPacket(int32(i))
			packet, err := client.Communicate(sourceDataReq)
			if err != nil {
				return fmt.Errorf("failed to get 'sourcedata' response for source #%d: %w", i, err)
			}
			sourceData := packet.(*chrony.ReplySourceData)
			req := chrony.NewNTPDataPacket(sourceData.IPAddr)
			selectData, err := client.Communicate(req)
			if err != nil {
				return err
			}
			fmt.Printf("Source %d (%s): %+v\n", i, sourceData.IPAddr, selectData)
		}
	case "sourcename":
		req := chrony.NewSourcesPacket()
		response, err := client.Communicate(req)
		if err != nil {
			return err
		}
		nSources := response.(*chrony.ReplySources).NSources
		fmt.Printf("Got %d sources\n", nSources)
		for i := range nSources {
			sourceDataReq := chrony.NewSourceDataPacket(int32(i))
			packet, err := client.Communicate(sourceDataReq)
			if err != nil {
				return fmt.Errorf("failed to get 'sourcedata' response for source #%d: %w", i, err)
			}
			sourceData := packet.(*chrony.ReplySourceData)
			req := chrony.NewNTPSourceNamePacket(sourceData.IPAddr)
			selectData, err := client.Communicate(req)
			if err != nil {
				return err
			}
			fmt.Printf("Source %d (%s): %+v\n", i, sourceData.IPAddr, selectData)
		}
	case "selectdata":
		req := chrony.NewSourcesPacket()
		response, err := client.Communicate(req)
		if err != nil {
			return err
		}
		nSources := response.(*chrony.ReplySources).NSources
		fmt.Printf("Got %d sources\n", nSources)
		for i := range nSources {
			req := chrony.NewSelectDataPacket(int32(i))
			selectData, err := client.Communicate(req)
			if err != nil {
				return err
			}
			fmt.Printf("Source %d: %+v\n", i, selectData)
		}
	default:
		return fmt.Errorf("unknown command %s, supported commands are %v", cmd, commands)
	}
	return nil
}

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), "Tool to test chrony protocol against real chronyd\n")
	fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flags] <command>\n Where command is one of %v\n", os.Args[0], commands)
	flag.PrintDefaults()
}

func main() {
	var (
		verboseFlag bool
		addressFlag string
	)
	flag.Usage = usage

	flag.BoolVar(&verboseFlag, "verbose", false, "verbose output")
	flag.StringVar(&addressFlag, "address", "localhost:323", "Address of chronyd to connect to")

	flag.Parse()

	log.SetLevel(log.InfoLevel)
	if verboseFlag {
		chrony.Logger = log.StandardLogger()
		log.SetLevel(log.DebugLevel)
	}

	if len(flag.Args()) < 1 {
		fmt.Printf("no command specified\n")
		usage()
		os.Exit(1)
	}
	if err := runCommand(addressFlag, flag.Arg(0)); err != nil {
		fmt.Printf("failed to run command: %v\n", err)
		fmt.Printf("if you got UNATH - try to run with sudo and -address flag set to chronyd unix socket (%q by default)\n", chrony.ChronySocketPath)
		os.Exit(1)
	}
}
