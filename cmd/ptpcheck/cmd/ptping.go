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

package cmd

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/facebook/time/dscp"
	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/sptp/client"
	"github.com/facebook/time/timestamp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

// flags
var (
	ifacef   string
	countf   int
	dscpf    int
	timeoutf time.Duration
)

func init() {
	RootCmd.AddCommand(ptpingCmd)
	ptpingCmd.Flags().StringVarP(&ifacef, "iface", "i", "eth0", "network interface to use")
	ptpingCmd.Flags().IntVarP(&countf, "count", "c", 5, "number of probes to send")
	ptpingCmd.Flags().IntVarP(&dscpf, "dscp", "d", 35, "dscp value (QoS)")
	ptpingCmd.Flags().DurationVarP(&timeoutf, "timeout", "t", time.Second, "request timeout/interval")
}

type ptping struct {
	iface  string
	dscp   int
	target string

	clockID   ptp.ClockIdentity
	eventConn client.UDPConnWithTS
	client    *client.Client

	inChan chan *client.InPacket
}

func (p *ptping) init() error {
	i, err := net.InterfaceByName(p.iface)
	if err != nil {
		return err
	}

	cid, err := ptp.NewClockIdentity(i.HardwareAddr)
	if err != nil {
		return err
	}
	p.clockID = cid

	// bind to event port
	eventConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("::"), Port: 0})
	if err != nil {
		return err
	}

	// get FD of the connection. Can be optimized by doing this when connection is created
	connFd, err := timestamp.ConnFd(eventConn)
	if err != nil {
		return err
	}

	localEventAddr := eventConn.LocalAddr()
	localEventIP := localEventAddr.(*net.UDPAddr).IP
	localEventPort := localEventAddr.(*net.UDPAddr).Port
	if err = dscp.Enable(connFd, localEventIP, p.dscp); err != nil {
		return fmt.Errorf("setting DSCP on event socket: %w", err)
	}

	// we need to enable HW or SW timestamps on event port
	if err = timestamp.EnableHWTimestamps(connFd, p.iface); err != nil {
		return fmt.Errorf("failed to enable hardware timestamps on port %d: %w", localEventPort, err)
	}
	// set it to blocking mode, otherwise recvmsg will just return with nothing most of the time
	if err = unix.SetNonblock(connFd, false); err != nil {
		return fmt.Errorf("failed to set event socket to blocking: %w", err)
	}

	p.eventConn = client.NewUDPConnTS(eventConn, connFd)
	timestamp.AttemptsTXTS = 5
	timestamp.TimeoutTXTS = 100 * time.Millisecond
	p.client, err = client.NewClient(p.target, ptp.PortEvent, p.clockID, p.eventConn, &client.Config{}, &client.JSONStats{})
	go p.runReader()

	return err
}

// timestamps returns t1, t2, t4
func (p *ptping) timestamps(timeout time.Duration) (time.Time, time.Time, time.Time, error) {
	var t1 time.Time
	var t2 time.Time
	var t4 time.Time
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			if t4.IsZero() {
				return t1, t2, t4, fmt.Errorf("timeout waiting")
			}
			return t1, t2, t4, nil
		case p := <-p.inChan:
			msgType, err := ptp.ProbeMsgType(p.Data())
			if err != nil {
				return t1, t2, t4, err
			}
			switch msgType {
			case ptp.MessageSync, ptp.MessageDelayReq:
				t2 = p.TS()
				b := &ptp.SyncDelayReq{}
				if err = ptp.FromBytes(p.Data(), b); err != nil {
					return t1, t2, t4, fmt.Errorf("reading sync msg: %w", err)
				}
				t4 = b.OriginTimestamp.Time()
				continue

			case ptp.MessageAnnounce:
				b := &ptp.Announce{}
				if err = ptp.FromBytes(p.Data(), b); err != nil {
					return t1, t2, t4, fmt.Errorf("reading announce msg: %w", err)
				}
				t1 = b.OriginTimestamp.Time()
				continue
			default:
				log.Infof("got unsupported packet %v:", msgType)
			}
		}
	}
}

func (p *ptping) runReader() {
	for {
		response, _, rxts, _ := p.eventConn.ReadPacketWithRXTimestamp()
		p.inChan <- client.NewInPacket(response, rxts)
	}
}

func ptpingRun(iface string, dscp int, server string, count int, timeout time.Duration) error {
	p := &ptping{
		iface:  iface,
		dscp:   dscp,
		target: server,
		inChan: make(chan *client.InPacket),
	}

	if err := p.init(); err != nil {
		return err
	}
	// We want to avoid first 10 which may be used by other tools
	portID := uint16(rand.Intn(10+65535) - 10)

	for c := 1; c <= count; c++ {
		_, t3, err := p.client.SendEventMsg(client.ReqDelay(p.clockID, portID))
		if err != nil {
			log.Errorf("failed to send request: %s", err)
			continue
		}

		t1, t2, t4, err := p.timestamps(timeout)
		if err != nil {
			log.Errorf("failed to read sync response: %v", err)
			continue
		}
		fw := t4.Sub(t3)
		bk := t2.Sub(t1)
		if t1.IsZero() {
			bk = 0
		}

		fmt.Printf("%s: seq=%d time=%s\t(->%s + <-%s)\n", server, c, fw+bk, fw, bk)
	}
	return nil
}

var ptpingCmd = &cobra.Command{
	Use:        "ptping {server}",
	Short:      "sptp-based ping",
	Long:       "measure real network latency between 2 sptp-enabled hosts",
	Args:       cobra.ExactArgs(1),
	ArgAliases: []string{"server"},
	Run: func(_ *cobra.Command, args []string) {
		ConfigureVerbosity()

		if err := ptpingRun(ifacef, dscpf, args[0], countf, timeoutf); err != nil {
			log.Fatal(err)
		}
	},
}
