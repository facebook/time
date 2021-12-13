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
	"time"

	ptp "github.com/facebookincubator/time/ptp/protocol"
	"github.com/facebookincubator/time/ptp/ziffy/node"
	log "github.com/sirupsen/logrus"
)

const doc = `
Ziffy is a CLI tool intended to triangulate datacenter switches that are not operating correctly as PTP TCs.

How to sweep the network topology?

	Ziffy sends PTP SYNC/DELAY_REQ packets between two hosts, a Ziffy sender and a Ziffy receiver in order to
	get data about the topology. It supports sending packets from a range of source ports to encourage hashing of
	traffic over multiple paths. In case the hashing is done using only destination IP and source IP, Ziffy can
	target multiple IPs in the same /64 prefix as the destination.

How to determine if a switch is operating as PTP TC?

	In order to determine if a switch is a PTP Transparent Clock, Ziffy sends IPV6 PTP SYNC/DELAY_REQ packets with
	the same (srcIP, srcPort, destIP, destPort) tuple but with consecutive Hop Limit. When a PTP packet is dropped
	by a switch, a ICMPv6 Hop limit exceeded packet containing the entire PTP packet is returned to the sender.

	Consider that Switch3 was hit using HopLimit=3 and Switch4 was hit using HopLimit=4.
	If (Switch4.CorrectionField - Switch3.CorrectionField < threshold) then Switch3 did not modify the CorrectionField,
	so it is not a Transparent Clock.

Flags:
`

func main() {
	c := &node.Config{}

	var messageType string
	var nsCFThreshold int

	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), doc)
		flag.PrintDefaults()
	}

	flag.StringVar(&c.LogLevel, "loglevel", "info", "set a log level. Can be: trace, debug, info, warning, error")
	flag.StringVar(&c.Mode, "mode", "receiver", "set the mode. Can be either sender or receiver")
	flag.StringVar(&c.Device, "if", "eth0", "network interface to use")
	flag.IntVar(&c.HopMax, "maxhop", 7, "max number of hops (used by sender)")
	flag.IntVar(&c.HopMin, "minhop", 1, "min number of hops (used by sender)")
	flag.DurationVar(&c.IcmpTimeout, "icmptime", 1*time.Second, "max timeout to wait for icmp packets (used by sender)")
	flag.StringVar(&c.DestinationAddress, "addr", "", "IP address of receiver (used by sender)")
	flag.IntVar(&c.DestinationPort, "dp", ptp.PortEvent, "destination port to send packets to")
	flag.IntVar(&c.SourcePort, "sp", 32768, "the base source port to start probing (used by sender)")
	flag.IntVar(&c.PortCount, "portcount", 1, "port count to be used for probing each target ip (used by sender)")
	flag.StringVar(&messageType, "type", "sync", "set the message type. Can be 'sync' (default), 'delay_req' or 'signaling' (used by sender)")
	flag.StringVar(&c.CsvFile, "csv", "", "csv output file path (used by sender)")
	flag.BoolVar(&c.ContReached, "continue", false, "continue incrementing hop count after destination host responds (used by sender)")
	flag.IntVar(&c.IPCount, "ipcount", 0, "number of additional IPs targeted in the same /64 prefix as destination to increase hashing entropy (used by sender)")
	flag.IntVar(&c.DSCP, "dscp", 0, "DSCP for PTP packets, valid values are between 0-63 (used by sender)")
	flag.DurationVar(&c.LLDPWaitTime, "lldptime", 5*time.Second, "max timeout to wait for LLDP packets (used by sender)")
	flag.IntVar(&c.QueueCap, "qcap", 10000, "ICMP queue capacity (used by sender)")
	flag.DurationVar(&c.IcmpReplyTime, "replytime", 1*time.Second, "waiting time for late icmp packets (used by sender)")
	flag.IntVar(&nsCFThreshold, "cfthreshold", 250, "CorrectionField threshold (CF difference < abs(CFThreshold) => switch not TC) (used by sender)")
	flag.IntVar(&c.PTPRecvHandlers, "maxhandlers", 10000, "maximum number of simultaneous goroutines used to handle PTP packets (used by receiver)")

	flag.Parse()

	log.Debugf("\nLogLevel = %v\nMode = %v\nDestinationAddress = %v\nDestinationPort = %v\nSourcePort = %v\n",
		c.LogLevel, c.Mode, c.DestinationAddress, c.DestinationPort, c.SourcePort)

	if c.DSCP < 0 || c.DSCP > 63 {
		log.Fatalf("unsupported DSCP value %v", c.DSCP)
	}

	if c.IcmpTimeout < 100*time.Millisecond {
		log.Warnf("setting timeout < 100ms for ICMP replies may lead to inaccurate results")
	}

	switch messageType {
	case "delay_req":
		c.MessageType = ptp.MessageDelayReq
	case "sync":
		c.MessageType = ptp.MessageSync
	case "signaling":
		c.MessageType = ptp.MessageSignaling
	default:
		log.Fatalf("unsupported message type %q", messageType)
	}

	switch c.LogLevel {
	case "trace":
		log.SetLevel(log.TraceLevel)
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warning":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	default:
		log.Fatalf("unrecognized log level: %v", c.LogLevel)
	}

	switch c.Mode {
	case "receiver":
		log.Infof("RECEIVER")

		r := node.Receiver{
			Config: c,
		}
		if err := r.Start(); err != nil {
			log.Errorf("receiver start failed: %v", err)
		}
	case "sender":
		log.Infof("SENDER")

		s := node.Sender{
			Config: c,
		}

		info, err := s.Start()
		if err != nil {
			log.Errorf("sender start failed: %v", err)
		}

		node.PrettyPrint(info, ptp.NewCorrection(float64(nsCFThreshold)))
		if s.Config.CsvFile != "" {
			node.CsvPrint(info, s.Config.CsvFile, ptp.NewCorrection(float64(nsCFThreshold)))
		}
	default:
		log.Errorf("--mode must be sender or receiver")
	}
}
