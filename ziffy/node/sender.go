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
	"encoding/binary"
	"fmt"
	"net"
	"sort"
	"syscall"
	"time"

	ptp "github.com/facebookincubator/ptp/protocol"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/ipv6"
)

const (
	// RackMaskBits identifies the rack ipv6 prefix
	RackMaskBits = 64
	// FaceHexaTop is the upper byte of 0xface
	FaceHexaTop = 0xfa
	// FaceHexaBot is the lower byte of 0xface
	FaceHexaBot = 0xce
	// LLDPTypeStr Ether type string
	LLDPTypeStr = "0x88cc"
)

// Sender sweeps the network with PTP packets
type Sender struct {
	Config *Config

	icmpConn *net.IPConn

	inputQueue chan *SwitchTrafficInfo
	icmpDone   chan bool

	routes         []PathInfo
	destHop        int
	rackSwHostname string

	currentRoute int
}

// Start sending PTP packets
func (s *Sender) Start() ([]PathInfo, error) {
	icmpAddr, err := net.ResolveIPAddr("ip6:ipv6-icmp", "")
	if err != nil {
		return nil, fmt.Errorf("unable to resolve source address: %w", err)
	}
	icmpConn, err := net.ListenIP("ip6:ipv6-icmp", icmpAddr)
	if err != nil {
		return nil, fmt.Errorf("unable to listen to icmp: %w", err)
	}
	defer icmpConn.Close()

	if s.rackSwHostname, err = rackSwHostnameMonitor(s.Config.Device, s.Config.LLDPWaitTime); err != nil {
		log.Warn("unable to learn name of rack switch via LLDP")
	}

	s.inputQueue = make(chan *SwitchTrafficInfo, s.Config.QueueCap)
	s.icmpDone = make(chan bool)
	s.icmpConn = icmpConn
	s.currentRoute = 0

	go s.monitorIcmp(s.icmpConn)

	log.Infof("sending %v flows of PTP %v packets to %v from source port range %v-%v with max hop count of %v and min hop count of %v and "+
		"sweeping %v other addresses in target network prefix with a per hop timeout of %v. Total flows %v.\n\n",
		s.Config.PortCount, s.Config.MessageType, s.Config.DestinationAddress,
		s.Config.SourcePort, s.Config.SourcePort+s.Config.PortCount-1, s.Config.HopMax, s.Config.HopMin, s.Config.IPCount, s.Config.IcmpTimeout,
		s.Config.PortCount+s.Config.PortCount*s.Config.IPCount)

	for i := 0; i < s.Config.PortCount; i++ {
		s.routes = append(s.routes, PathInfo{switches: nil, rackSwHostname: s.rackSwHostname})
		_, err := s.traceRoute(s.Config.DestinationAddress, s.Config.SourcePort+i, false)
		if err != nil {
			log.Errorf("traceRoute failed: %v", err)
			continue
		}

		s.currentRoute++
		s.popAllQueue()
	}
	if s.Config.IPCount != 0 {
		s.sweepRackPrefix()
	}

	// Waiting for late packets, if any
	time.Sleep(s.Config.IcmpReplyTime)
	s.popAllQueue()

	s.icmpDone <- true
	return s.clearPaths(), nil
}

// Insert late packets into corresponding path
// Fixes the scenario in which packets arrive after traceRoute finished
func (s *Sender) popAllQueue() {
	for len(s.inputQueue) > 0 {
		sw := <-s.inputQueue
		s.routes[sw.routeIdx].switches = append(s.routes[sw.routeIdx].switches, *sw)
	}
}

func sortSwitchesByHop(swArray []SwitchTrafficInfo) {
	sort.Slice(swArray, func(i, j int) bool {
		return swArray[i].hop < swArray[j].hop
	})
}

// clearPaths fixes corner case scenarios
func (s *Sender) clearPaths() []PathInfo {
	var retPaths []PathInfo
	idx := 0
	for _, route := range s.routes {
		retPaths = append(retPaths, PathInfo{switches: nil, rackSwHostname: s.rackSwHostname})
		// Sort each route. This fixes the scenario where a
		// packet with lower hop arrives after a packet with higher hop
		sortSwitchesByHop(route.switches)
		for j, sw := range route.switches {
			// Strip out duplicate ICMP replies to address issue where
			// switch sends more than one ICMP Hop Limit Exceeded message with same hop number
			if j != len(route.switches)-1 && route.switches[j].hop == route.switches[j+1].hop {
				continue
			}
			retPaths[idx].switches = append(retPaths[idx].switches, sw)
		}
		idx++
	}
	return retPaths
}

// sweepRackPrefix iterates over additional IP addresses
// (within the same /64 as the destination IP) and targets
// those addresses.
func (s *Sender) sweepRackPrefix() {
	for i := 1; i <= s.Config.IPCount; i++ {
		newIP := s.formNewDest(i)
		for j := 0; j < s.Config.PortCount; j++ {
			s.routes = append(s.routes, PathInfo{rackSwHostname: s.rackSwHostname})

			if _, err := s.traceRoute(newIP.String(), s.Config.SourcePort+j, true); err != nil {
				log.Errorf("sweepRackPrefix traceRoute failed: %v", err)
				continue
			}
			s.currentRoute++

			s.popAllQueue()
		}
		fmt.Printf("\r %v/%v IPs tested. Current: %v", i, s.Config.IPCount, newIP.String())
	}
	fmt.Printf("\n\n")
}

func (s *Sender) traceRoute(destinationIP string, sendingPort int, sweep bool) ([]SwitchTrafficInfo, error) {
	var route []SwitchTrafficInfo
	ptpAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(destinationIP, fmt.Sprint(s.Config.DestinationPort)))
	if err != nil {
		return nil, fmt.Errorf("traceRoute unable to resolve UDPAddr: %w", err)
	}
	ptpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(""), Port: sendingPort})
	if err != nil {
		return nil, fmt.Errorf("traceRoute unable to establish connection: %w", err)
	}
	defer ptpConn.Close()

	file, err := ptpConn.File()
	if err != nil {
		return nil, fmt.Errorf("traceRoute unable to open ptpConn file: %w", err)
	}
	defer file.Close()

	destReached := false
	hopMax := s.Config.HopMax

	// if sweep is activated and the destination was found
	if sweep && s.destHop > 0 {
		hopMax = s.destHop - 1
	}
	// Stop incrementing hops when either the max hop count is reached or
	// the destination has responded unless continue is specified
	for hop := s.Config.HopMin; hop <= hopMax && (!destReached || s.Config.ContReached); hop++ {
		if err := syscall.SetsockoptInt(int(file.Fd()), syscall.IPPROTO_IPV6, syscall.IPV6_UNICAST_HOPS, hop); err != nil {
			return route, err
		}
		// First 2 bits from Traffic Class are unused, so we shift the value 2 bits
		if err := syscall.SetsockoptInt(int(file.Fd()), syscall.IPPROTO_IPV6, syscall.IPV6_TCLASS, s.Config.DSCP<<2); err != nil {
			return route, err
		}
		if err := s.sendEventMsg(s.formSyncPacket(hop, s.currentRoute), ptpConn, ptpAddr); err != nil {
			return route, err
		}

		select {
		case sw := <-s.inputQueue:
			s.routes[sw.routeIdx].switches = append(s.routes[sw.routeIdx].switches, *sw)
			if sw.ip == ptpAddr.IP.String() {
				destReached = true
				s.destHop = hop
			}
		case <-time.After(s.Config.IcmpTimeout):
			continue
		}
	}
	return route, nil
}

func (s *Sender) sendEventMsg(p ptp.Packet, ptpConn *net.UDPConn, ptpAddr *net.UDPAddr) error {
	b, err := ptp.Bytes(p)
	if err != nil {
		return err
	}
	_, err = ptpConn.WriteTo(b, ptpAddr)
	if err != nil {
		return err
	}
	return nil
}

// formSyncPacket creates PTP SYNC header
// SequenceId contains origin hop; PortNumber contains origin port;
// ControlField contains the Zi(0xff)y identifier
func (s *Sender) formSyncPacket(hop int, routeIndex int) *ptp.SyncDelayReq {
	return &ptp.SyncDelayReq{
		Header: ptp.Header{
			SdoIDAndMsgType: ptp.NewSdoIDAndMsgType(s.Config.MessageType, 0),
			Version:         ptp.Version,
			MessageLength:   uint16(binary.Size(ptp.SyncDelayReq{})),
			FlagField:       ptp.FlagUnicast,
			SequenceID:      uint16(hop),
			SourcePortIdentity: ptp.PortIdentity{
				PortNumber: uint16(routeIndex),
			},
			ControlField:       ZiffyHexa, //identifier for zi(0xff)y
			LogMessageInterval: 0x7f,
		},
	}
}

func (s *Sender) monitorIcmp(conn net.PacketConn) {
	buf := make([]byte, 128)
	for {
		select {
		case <-s.icmpDone:
			return
		default:
			n, rAddr, err := conn.ReadFrom(buf)
			if err != nil {
				log.Debugf("icmp listener error: %v", err)
				continue
			}
			go s.handleIcmpPacket(buf, n, rAddr)
		}
	}
}

// handleIcmpPacket is a handler which gets called every time icmp packets arrive
func (s *Sender) handleIcmpPacket(rawPacket []byte, len int, rAddr net.Addr) {
	icmpType := rawPacket[0]
	if ipv6.ICMPType(icmpType) != ipv6.ICMPTypeTimeExceeded {
		log.Tracef("not ipv6 timeexceeded packet")
		return
	}
	ptpOffset := Ipv6HeaderSize + UDPHeaderSize + ICMPHeaderSize
	if ptpOffset > len {
		log.Tracef("packet too short")
		return
	}
	ptpPacket, err := ptp.DecodePacket(rawPacket[ptpOffset:len])
	if err != nil {
		log.Tracef("PTP not contained in ICMP")
		return
	}

	s.inputQueue <- &SwitchTrafficInfo{
		ip:        rAddr.String(),
		corrField: ptpPacket.(*ptp.SyncDelayReq).Header.CorrectionField,
		hop:       int(ptpPacket.(*ptp.SyncDelayReq).Header.SequenceID),
		routeIdx:  int(ptpPacket.(*ptp.SyncDelayReq).Header.SourcePortIdentity.PortNumber),
	}
	log.Debugf("%v cf: %v hop: %v", getLookUpName(rAddr.String()), ptpPacket.(*ptp.SyncDelayReq).Header.CorrectionField, int(ptpPacket.(*ptp.SyncDelayReq).Header.SequenceID))
}

// formNewDest generates new ip address using the
// rack prefix /64 of DestinationAddress by adding
// :face:face:0:$i to the ipv6
func (s *Sender) formNewDest(i int) net.IP {
	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(s.Config.DestinationAddress, fmt.Sprintf("%d", s.Config.DestinationPort)))
	if err != nil {
		return nil
	}
	m := net.CIDRMask(RackMaskBits, 8*net.IPv6len)
	maskIP := addr.IP.Mask(m)
	ip := net.ParseIP(maskIP.String())
	// add first :face: in the new ipv6
	ip[8] += FaceHexaTop
	ip[9] += FaceHexaBot
	// add second :face: in the new ipv6
	ip[10] += FaceHexaTop
	ip[11] += FaceHexaBot
	// add argument i at the end of the new ipv6
	ip[len(ip)-1] += byte(i)
	ip[len(ip)-2] += byte(i >> 8)
	// if rack switch /64 is 2401:db00:251c:2608:: and i is 4
	// resulting ip is 2401:db00:251c:2608:face:face:0:4
	return net.IP(ip)
}

// rackSwHostname listens to lldp packets from rack switch
func rackSwHostnameMonitor(device string, lldpTimeout time.Duration) (string, error) {
	log.Info("listening for LLDP packets from rack switch")

	handle, err := pcap.OpenLive(device, SnapshotLen, true, RecvTimeout)
	if err != nil {
		return "", fmt.Errorf("unable to OpenLive: %w", err)
	}
	defer handle.Close()

	filter := "ether proto " + LLDPTypeStr
	if err := handle.SetBPFFilter(filter); err != nil {
		return "", fmt.Errorf("unable to set BPF Filter: %w", err)
	}

	rackChan := make(chan string, 1)
	go func() {
		pktSrc := gopacket.NewPacketSource(handle, handle.LinkType())
		for pkt := range pktSrc.Packets() {
			p := gopacket.NewPacket(pkt.Data(), layers.LinkTypeEthernet, gopacket.DecodeOptions{})
			info := p.Layer(layers.LayerTypeLinkLayerDiscoveryInfo)
			rackChan <- info.(*layers.LinkLayerDiscoveryInfo).SysName
			break
		}
	}()

	select {
	case res := <-rackChan:
		return res, nil
	case <-time.After(lldpTimeout):
		return "", fmt.Errorf("unable to get rack hostname")
	}
}

func getLookUpName(ip string) string {
	addr, err := net.LookupAddr(ip)
	if err != nil {
		return ip
	}
	return addr[0]
}
