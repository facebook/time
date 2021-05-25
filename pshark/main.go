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
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"

	ptp "github.com/facebookincubator/ptp/protocol"
)

// for flags

// MultiMessageType is a wrapper around []string to parse from flags
type MultiMessageType []ptp.MessageType

// Set adds message type to the filter
func (m *MultiMessageType) Set(messageType string) error {
	for v, s := range ptp.MessageTypeToString {
		if s == strings.ToUpper(messageType) {
			*m = append([]ptp.MessageType(*m), v)
			return nil
		}
	}
	return fmt.Errorf("unsupported msg type %q", messageType)
}

// String returns joined list of message types
func (m *MultiMessageType) String() string {
	s := []string{}
	for _, v := range []ptp.MessageType(*m) {
		s = append(s, v.String())
	}
	return strings.Join(s, ",")
}

// GetDefaults returns default message type filter
func (m *MultiMessageType) GetDefaults() []ptp.MessageType {
	res := []ptp.MessageType{}
	for v := range ptp.MessageTypeToString {
		res = append(res, v)
	}
	return res
}

// SetDefault sets default message type filter
func (m *MultiMessageType) SetDefault() {
	if len([]ptp.MessageType(*m)) != 0 {
		return
	}
	for _, v := range m.GetDefaults() {
		*m = append([]ptp.MessageType(*m), v)
	}
}

// Tiny wrapper code to support gopacket integration

// LayerPTP wraps around ptp packet
type LayerPTP struct {
	layers.BaseLayer

	Packet ptp.Packet
}

// LayerTypePTP is registered as a layer with gopacket
var LayerTypePTP = gopacket.RegisterLayerType(
	1588,
	gopacket.LayerTypeMetadata{
		Name:    "PTPv2",
		Decoder: gopacket.DecodeFunc(decodePTP),
	},
)

// LayerType returns type this layer implements
func (l *LayerPTP) LayerType() gopacket.LayerType {
	return LayerTypePTP
}

// Payload is empty as it's the final layer
func (l *LayerPTP) Payload() []byte {
	return nil
}

// decodePTP actually does the decoding
func decodePTP(data []byte, p gopacket.PacketBuilder) error {
	d := &LayerPTP{}
	ptpPacket, err := ptp.DecodePacket(data)
	if err != nil {
		return fmt.Errorf("decoding PTPv2 packet: %w", err)
	}
	d.BaseLayer = layers.BaseLayer{Contents: data[:]}
	d.Packet = ptpPacket
	p.AddLayer(d)
	p.SetApplicationLayer(d)
	return nil
}

// packetHandle abstracts packet handles provided by pcapgo.Reader and pcapgo.NGReader
type packetHandle interface {
	gopacket.PacketDataSource
	LinkType() layers.LinkType
}

func run(input string, filter []ptp.MessageType) error {
	// register mapping betwenn ports and our custom PTP layer
	layers.RegisterUDPPortLayerType(ptp.PortEvent, LayerTypePTP)
	layers.RegisterUDPPortLayerType(ptp.PortGeneral, LayerTypePTP)

	filterMap := map[ptp.MessageType]bool{}
	for _, v := range filter {
		filterMap[v] = true
	}

	var handle packetHandle
	var err error

	// open the input file
	f, err := os.Open(input)
	if err != nil {
		return err
	}
	defer f.Close()

	// try NGReader, if it fails - fall back to Reader
	handle, err = pcapgo.NewNgReader(f, pcapgo.DefaultNgReaderOptions)
	if err != nil {
    if _, ierr := f.Seek(0, 0); ierr != nil {
      return fmt.Errorf("seeking in %s: %w", input, ierr)
    }
		handle, err = pcapgo.NewReader(f)
		if err != nil {
			return fmt.Errorf("decoding %s: %w", input, err)
		}
	}

	// Loop through packets in file
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	for packet := range packetSource.Packets() {
		// thanks to the mapping we can easily pick PTP packets
		ptpLayer := packet.Layer(LayerTypePTP)
		if ptpLayer != nil {
			ptpContent, _ := ptpLayer.(*LayerPTP)
			if !filterMap[ptpContent.Packet.MessageType()] {
				continue
			}
			// decode src and dst adddress and port
			var srcIP, dstIP net.IP
			var srcPort, dstPort layers.UDPPort
			ip6Layer := packet.Layer(layers.LayerTypeIPv6)
			if ip6Layer != nil {
				ip, _ := ip6Layer.(*layers.IPv6)
				srcIP = ip.SrcIP
				dstIP = ip.DstIP
			} else {
				ip4Layer := packet.Layer(layers.LayerTypeIPv4)
				ip, _ := ip4Layer.(*layers.IPv4)
				srcIP = ip.SrcIP
				dstIP = ip.DstIP
			}

			udpLayer := packet.Layer(layers.LayerTypeUDP)
			if udpLayer != nil {
				udp, _ := udpLayer.(*layers.UDP)
				srcPort = udp.SrcPort
				dstPort = udp.DstPort
			}
			// dump ip:port info on stdout
			spew.Printf("%s -> %s\n",
				net.JoinHostPort(srcIP.String(), strconv.Itoa(int(srcPort))),
				net.JoinHostPort(dstIP.String(), strconv.Itoa(int(dstPort))),
			)
			// dump the packet itself
			spew.Dump(ptpContent.Packet)
			spew.Println()
		}
		if err := packet.ErrorLayer(); err != nil {
			return fmt.Errorf("failed to decode: %w", err.Error())
		}
	}
	return nil
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "pshark: PTP-specific poor man's tshark. Dumps PTPv2 packets parsed from capture file to stdout.\nUsage:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "%s [file]\n", os.Args[0])
		fmt.Fprint(flag.CommandLine.Output(), "where [file] is any .pcap or .pcapng packet capture\n")
		flag.PrintDefaults()
	}
	var msgTypes MultiMessageType
	flag.Var(&msgTypes, "msgtype", fmt.Sprintf("Only print certain PTP message types. Choose from: %v. Repeat for multiple", msgTypes.GetDefaults()))
	flag.Parse()
	if len(flag.Args()) != 1 {
		flag.Usage()
		os.Exit(1)
	}
	msgTypes.SetDefault()
	if err := run(flag.Arg(0), msgTypes); err != nil {
		log.Fatal(err)
	}
}
