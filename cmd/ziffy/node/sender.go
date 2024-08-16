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
	"sort"
	"sync"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/timestamp"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/ipv6"
	"golang.org/x/sys/unix"
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

type traceTask struct {
	destinationIP string
	sendingPort   int
	routeID       int
}

// Sender sweeps the network with PTP packets
type Sender struct {
	Config *Config

	icmpConn *net.IPConn

	inputQueue []chan *SwitchTrafficInfo
	icmpDone   chan bool

	rackSwHostname string
}

func prepareTracing(c *Config) []traceTask {
	destIPs := []string{c.DestinationAddress}
	for i := 1; i <= c.IPCount; i++ {
		newIP := formNewDest(c, i)
		destIPs = append(destIPs, newIP.String())
	}
	tasks := []traceTask{}
	for i, destIP := range destIPs {
		for portID := 0; portID < c.PortCount; portID++ {
			tasks = append(tasks, traceTask{
				destinationIP: destIP,
				sendingPort:   c.SourcePort + portID,
				routeID:       i*c.PortCount + portID,
			})
		}
	}
	return tasks
}

// Start sending PTP packets
func (s *Sender) Start() ([]*PathInfo, error) {
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

	s.icmpDone = make(chan bool)
	s.icmpConn = icmpConn

	log.Infof("sending %v flows of PTP %v packets to %v from source port range %v-%v to destination port %v with %v packets per hop, max hop count of %v and min hop count of %v and "+
		"sweeping %v other addresses in target network prefix with a per hop timeout of %v. Total flows %v.\n\n",
		s.Config.PortCount, s.Config.MessageType, s.Config.DestinationAddress,
		s.Config.SourcePort, s.Config.SourcePort+s.Config.PortCount-1, s.Config.DestinationPort,
		s.Config.PacketsPerHop,
		s.Config.HopMax, s.Config.HopMin, s.Config.IPCount, s.Config.IcmpTimeout,
		s.Config.PortCount+s.Config.PortCount*s.Config.IPCount)

	var g sync.WaitGroup
	// prepare tasks
	tasks := prepareTracing(s.Config)
	// prepare input queue
	s.inputQueue = make([]chan *SwitchTrafficInfo, len(tasks))
	for i := range tasks {
		s.inputQueue[i] = make(chan *SwitchTrafficInfo, s.Config.QueueCap)
	}
	// start icmp listener
	go s.monitorIcmp(s.icmpConn)

	routes := make([]*PathInfo, len(tasks))
	var mu sync.Mutex
	for _, t := range tasks {
		g.Add(1)
		go func(t traceTask) {
			defer g.Done()
			route, err := s.traceRoute(t.destinationIP, t.sendingPort, t.routeID)
			if err != nil {
				log.Errorf("traceRoute failed: %v", err)
				return
			}
			mu.Lock()
			routes[t.routeID] = route
			mu.Unlock()
		}(t)
	}
	g.Wait()

	// Waiting for late packets, if any
	time.Sleep(s.Config.IcmpReplyTime)
	s.popAllQueue(routes)

	s.icmpDone <- true
	return s.clearPaths(routes), nil
}

// Insert late packets into corresponding path
// Fixes the scenario in which packets arrive after traceRoute finished
func (s *Sender) popAllQueue(routes []*PathInfo) {
	for i := 0; i < len(routes); i++ {
		for len(s.inputQueue[i]) > 0 {
			sw := <-s.inputQueue[i]
			routes[sw.routeIdx].switches = append(routes[sw.routeIdx].switches, *sw)
		}
	}
}

func sortSwitchesByHop(swArray []SwitchTrafficInfo) {
	sort.Slice(swArray, func(i, j int) bool {
		return swArray[i].hop < swArray[j].hop
	})
}

// clearPaths fixes corner case scenarios
func (s *Sender) clearPaths(routes []*PathInfo) []*PathInfo {
	retPaths := make([]*PathInfo, 0, len(routes))
	idx := 0
	for _, route := range routes {
		retPaths = append(retPaths, &PathInfo{switches: nil, rackSwHostname: s.rackSwHostname})
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

func (s *Sender) traceRoute(destinationIP string, sendingPort int, routeID int) (*PathInfo, error) {
	route := &PathInfo{switches: nil, rackSwHostname: s.rackSwHostname}
	ptpUDPAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(destinationIP, fmt.Sprint(s.Config.DestinationPort)))
	if err != nil {
		return route, fmt.Errorf("traceRoute unable to resolve UDPAddr: %w", err)
	}
	ptpAddr := timestamp.IPToSockaddr(ptpUDPAddr.IP, s.Config.DestinationPort)
	domain := unix.AF_INET6
	if ptpUDPAddr.IP.To4() != nil {
		domain = unix.AF_INET
	}
	connFd, err := unix.Socket(domain, unix.SOCK_DGRAM, unix.IPPROTO_UDP)
	if err != nil {
		return route, fmt.Errorf("traceRoute unable to create connection: %w", err)
	}
	defer unix.Close(connFd)
	// set SO_REUSEPORT so we can trace network path from same source port that ptp4u uses
	if err = unix.SetsockoptInt(connFd, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); err != nil {
		return route, fmt.Errorf("setting SO_REUSEPORT on sender socket: %w", err)
	}
	localAddr := timestamp.IPToSockaddr(net.IPv6zero, sendingPort)
	if err := unix.Bind(connFd, localAddr); err != nil {
		return route, fmt.Errorf("traceRoute unable to bind %v connection: %w", localAddr, err)
	}

	destReached := false
	hopMax := s.Config.HopMax

	// Stop incrementing hops when either the max hop count is reached or
	// the destination has responded unless continue is specified
	for hop := s.Config.HopMin; hop <= hopMax && (!destReached || s.Config.ContReached); hop++ {
		for i := 0; i < s.Config.PacketsPerHop; i++ {
			if err := unix.SetsockoptInt(connFd, unix.IPPROTO_IPV6, unix.IPV6_UNICAST_HOPS, hop); err != nil {
				return route, err
			}
			// First 2 bits from Traffic Class are unused, so we shift the value 2 bits
			if err := unix.SetsockoptInt(connFd, unix.IPPROTO_IPV6, unix.IPV6_TCLASS, s.Config.DSCP<<2); err != nil {
				return route, err
			}
			var p ptp.Packet
			switch s.Config.MessageType {
			case ptp.MessageSync, ptp.MessageDelayReq:
				p = formSyncPacket(s.Config.MessageType, hop, routeID)
			case ptp.MessageSignaling:
				p = formSignalingPacket(hop, routeID)
			default:
				return route, fmt.Errorf("unsupported packet type %v", s.Config.MessageType)
			}

			if err := s.sendEventMsg(p, connFd, ptpAddr); err != nil {
				return route, err
			}

			select {
			case sw := <-s.inputQueue[routeID]:
				l := len(route.switches)
				newSampleForHop := true
				if l > 0 {
					lastSample := route.switches[l-1]
					// check previous sample for this hop to see if CF was lower.
					// this is intended to eliminate use cases where CF has a transient spike
					// which when subtracted from CF on subsequent hop can lead to negative values
					if lastSample.hop == sw.hop {
						newSampleForHop = false
						if sw.corrField < lastSample.corrField {
							log.Debugf("Received better sample for hop %v", sw.hop)
							route.switches[l-1] = *sw
						}
					}
				}
				if newSampleForHop {
					route.switches = append(route.switches, *sw)
				}
				if net.ParseIP(sw.ip).Equal(ptpUDPAddr.IP) {
					destReached = true
				}
			case <-time.After(s.Config.IcmpTimeout):
				continue
			}
		}
	}
	return route, nil
}

func (s *Sender) sendEventMsg(p ptp.Packet, ptpConn int, ptpAddr unix.Sockaddr) error {
	b, err := ptp.Bytes(p)
	if err != nil {
		return err
	}
	return unix.Sendto(ptpConn, b, 0, ptpAddr)
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
			s.handleIcmpPacket(buf, n, rAddr)
		}
	}
}

// handleIcmpPacket is a handler which gets called every time icmp packets arrive
func (s *Sender) handleIcmpPacket(rawPacket []byte, l int, rAddr net.Addr) {
	icmpType := rawPacket[0]
	if ipv6.ICMPType(icmpType) != ipv6.ICMPTypeTimeExceeded {
		log.Tracef("not ipv6 timeexceeded packet")
		return
	}
	ptpOffset := Ipv6HeaderSize + UDPHeaderSize + ICMPHeaderSize
	if ptpOffset > l {
		log.Tracef("packet too short")
		return
	}
	ptpPacket, err := ptp.DecodePacket(rawPacket[ptpOffset:l])
	if err != nil {
		log.Tracef("PTP not contained in ICMP")
		return
	}

	var (
		corrField  ptp.Correction
		sequenceID uint16
		portNum    uint16
	)
	switch v := ptpPacket.(type) {
	case *ptp.SyncDelayReq:
		corrField = v.Header.CorrectionField
		sequenceID = v.Header.SequenceID
		portNum = v.Header.SourcePortIdentity.PortNumber
	case *ptp.Signaling:
		corrField = v.Header.CorrectionField
		sequenceID = v.Header.SequenceID
		portNum = v.Header.SourcePortIdentity.PortNumber
	default:
		log.Errorf("Received unexpected packet %T, ignoring", v)
		return
	}

	if int(portNum) >= len(s.inputQueue) {
		log.Errorf("Received packet with invalid port number/traceID %v", portNum)
		return
	}

	s.inputQueue[portNum] <- &SwitchTrafficInfo{
		ip:        rAddr.String(),
		corrField: corrField,
		hop:       int(sequenceID),
		routeIdx:  int(portNum),
	}
	log.Debugf("routeIdx %d: %v cf: %v hop: %v", portNum, getLookUpName(rAddr.String()), corrField, sequenceID)
}

// formNewDest generates new ip address using the
// rack prefix /64 of DestinationAddress by adding
// :face:face:0:$i to the ipv6
func formNewDest(c *Config, i int) net.IP {
	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(c.DestinationAddress, fmt.Sprintf("%d", c.DestinationPort)))
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
	return ip
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
