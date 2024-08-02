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

package node

import (
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
)

const (
	// ZiffyHexa signs zi(0xff)y PTP packets
	ZiffyHexa = 0xff
	// Ipv6HeaderSize is ipv6 header size
	Ipv6HeaderSize = 40
	// UDPHeaderSize is udp header size
	UDPHeaderSize = 8
	// ICMPHeaderSize is icmp ipv6 header size
	ICMPHeaderSize = 8
	// PTPUnusedSize received from sender
	PTPUnusedSize = 10
)

// Config is the Ziffy config struct
type Config struct {
	Mode               string
	LogLevel           string
	Device             string
	CsvFile            string
	DestinationAddress string
	DestinationPort    int
	SourcePort         int
	PortCount          int
	HopMax             int
	HopMin             int
	PacketsPerHop      int
	IPCount            int
	DSCP               int
	PTPRecvHandlers    int
	ContReached        bool
	IcmpTimeout        time.Duration
	MessageType        ptp.MessageType
	LLDPWaitTime       time.Duration
	IcmpReplyTime      time.Duration

	// QueueCap used to store ICMP messages. Capacity is the number of late messages
	// stored until the queue is cleared. After each traceRoute, sender clears the queue.
	// For worst case scenario maximum capacity should be (HopMax - HopMin + 1) * PortCount * IPCount
	QueueCap int
}

// SwitchPrintInfo contains print information for switches
type SwitchPrintInfo struct {
	ip        string
	hostname  string
	sampleSP  int
	interf    string
	routes    int
	divRoutes int
	totalCF   ptp.Correction
	avgCF     ptp.Correction
	maxCF     ptp.Correction
	minCF     ptp.Correction
	tcEnable  status
	hop       int
	last      bool
}

// SwitchTrafficInfo contains information about a switch
type SwitchTrafficInfo struct {
	ip        string
	corrField ptp.Correction
	routeIdx  int
	hop       int
}

// PathInfo contains the list of switches in a path
type PathInfo struct {
	switches       []SwitchTrafficInfo
	rackSwHostname string
}
