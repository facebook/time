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
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv6"
)

const (
	// Promiscuous sets the pcap handle promiscuous flag
	Promiscuous = false
	// RecvTimeout sets the resolution for the listener
	RecvTimeout = 1 * time.Microsecond
	// SnapshotLen sets max length of packets
	SnapshotLen = 1024
)

// Receiver listens for PTP packets
type Receiver struct {
	Config *Config

	runningHandlers int
	*sync.Mutex
}

// Start listens for PTP packets and reply to sender
func (r *Receiver) Start() error {
	handle, err := pcap.OpenLive(r.Config.Device, SnapshotLen, Promiscuous, RecvTimeout)
	if err != nil {
		return fmt.Errorf("unable to open device interface: %w", err)
	}
	defer handle.Close()

	filter := "udp and port " + strconv.Itoa(r.Config.DestinationPort)
	if err := handle.SetBPFFilter(filter); err != nil {
		return fmt.Errorf("unable to set BPF Filter: %w", err)
	}

	log.Infof("listening on port %v for PTP EVENT packets (SYNC/DELAY_REQ) with ZiffyHexa signature. Sending back the packets as icmp\n\n",
		r.Config.DestinationPort)

	r.Mutex = &sync.Mutex{}
	r.runningHandlers = 0

	pktSrc := gopacket.NewPacketSource(handle, handle.LinkType())
	for pkt := range pktSrc.Packets() {
		if r.incRunningHandlers() {
			go r.handlePacket(pkt)
		}
	}
	return nil
}

func (r *Receiver) incRunningHandlers() bool {
	r.Lock()
	defer r.Unlock()
	if r.Config.PTPRecvHandlers > r.runningHandlers {
		r.runningHandlers++
		return true
	}
	return false
}

func (r *Receiver) decRunningHandlers() bool {
	r.Lock()
	defer r.Unlock()
	r.runningHandlers--
	return r.runningHandlers >= 0
}

func (r *Receiver) handlePacket(rawPacket gopacket.Packet) {
	defer func() {
		if !r.decRunningHandlers() {
			log.Fatal("decRunningHandlers: number of goroutines negative")
		}
	}()

	ptpSync, srcIP, srcPort, err := parseSyncPacket(rawPacket)
	if err != nil {
		log.Tracef("unable to parse PTP: %v", err)
		return
	}
	if ptpSync.Header.ControlField != ZiffyHexa {
		log.Tracef("no ziffy packet")
		return
	}

	log.Debugf("Type=%v, CF=%v, sPort=%v, sPort2=%v, sIP=%v", ptpSync.Header.MessageType().String(),
		ptpSync.Header.CorrectionField, ptpSync.Header.SourcePortIdentity.PortNumber, srcPort, srcIP)

	if err := r.sendResponse(ptpSync, srcIP, rawPacket); err != nil {
		log.Tracef("unable to send response: %v", err)
		return
	}
}

//sendResponse sends ICMPTypeTimeExceeded to sender
func (r *Receiver) sendResponse(packet *ptp.SyncDelayReq, sourceIP string, rawPacket gopacket.Packet) error {
	dst, err := net.ResolveIPAddr("ip6:ipv6-icmp", sourceIP)
	if err != nil {
		return fmt.Errorf("unable to resolve sender address: %w", err)
	}
	conn, err := net.ListenPacket("ip6:ipv6-icmp", "")
	if err != nil {
		return fmt.Errorf("unable to establish connection: %w", err)
	}
	defer conn.Close()

	mess := icmp.Message{
		Type: ipv6.ICMPTypeTimeExceeded, Code: 0,
		Body: &icmp.RawBody{
			Data: rawPacket.Data()[PTPUnusedSize:],
		},
	}
	buf, err := mess.Marshal(nil)
	if err != nil {
		return fmt.Errorf("unable to marshal the icmp packet: %w", err)
	}
	if _, err := conn.WriteTo(buf, dst); err != nil {
		return fmt.Errorf("unable to write to connection: %w", err)
	}
	return nil
}

func parseSyncPacket(packet gopacket.Packet) (*ptp.SyncDelayReq, string, string, error) {
	ipHeader, ok := packet.NetworkLayer().(*layers.IPv6)
	if !ok {
		return nil, "", "", fmt.Errorf("unable to parse IPv6 Header")
	}
	udpHeader, ok := packet.TransportLayer().(*layers.UDP)
	if !ok {
		return nil, "", "", fmt.Errorf("unable to parse UDP Header")
	}

	ptpPacket, err := ptp.DecodePacket(packet.ApplicationLayer().Payload())
	if err != nil {
		return nil, "", "", fmt.Errorf("unable to decode ptp packet: %w", err)
	}
	return ptpPacket.(*ptp.SyncDelayReq), ipHeader.SrcIP.String(), strconv.Itoa(int(udpHeader.SrcPort)), nil
}
