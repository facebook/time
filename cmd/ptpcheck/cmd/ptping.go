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
	serverf  string
	countf   int
	dscpf    int
	timeoutf time.Duration
)

func init() {
	RootCmd.AddCommand(ptpingCmd)
	ptpingCmd.Flags().StringVarP(&ifacef, "iface", "i", "eth0", "network interface to use")
	ptpingCmd.Flags().StringVarP(&serverf, "server", "S", "", "remote sptp server to connect to")
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
	p.client, err = client.NewClient(p.target, ptp.PortEvent, p.clockID, p.eventConn, &client.Config{}, &client.JSONStats{})
	return err
}

func (p *ptping) t4(timeout time.Duration) (time.Time, error) {
	var t4 time.Time
	doneChan := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	go func() {
		response, _, _, err := p.eventConn.ReadPacketWithRXTimestamp()
		if err != nil {
			doneChan <- err
			return
		}
		var msgType ptp.MessageType
		msgType, err = ptp.ProbeMsgType(response)
		if err != nil {
			doneChan <- err
			return
		}
		switch msgType {
		case ptp.MessageSync, ptp.MessageDelayReq:
			b := &ptp.SyncDelayReq{}
			if err = ptp.FromBytes(response, b); err != nil {
				doneChan <- fmt.Errorf("reading sync msg: %w", err)
				return
			}
			t4 = b.OriginTimestamp.Time()
			doneChan <- nil
			return
		default:
			doneChan <- fmt.Errorf("got unsupported packet: %v", msgType)
			return
		}
	}()

	select {
	case <-ctx.Done():
		return t4, fmt.Errorf("tired of waiting for a response")
	case err := <-doneChan:
		return t4, err
	}
}

func ptpingRun(iface string, dscp int, server string, count int, timeout time.Duration) error {
	p := &ptping{
		iface:  iface,
		dscp:   dscp,
		target: server,
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

		t4, err := p.t4(timeout)
		if err != nil {
			log.Errorf("failed to read response: %s", err)
		} else {
			fmt.Printf("%s: seq=%d time=%s\n", server, c, t4.Sub(t3))
		}
		time.Sleep(timeout)
	}
	return nil
}

var ptpingCmd = &cobra.Command{
	Use:   "ptping",
	Short: "sptp-based ping",
	Long:  "measure real network latency between 2 sptp-enabled hosts",
	Run: func(c *cobra.Command, args []string) {
		ConfigureVerbosity()

		if serverf == "" {
			log.Fatal("remote server must be specified")
		}

		if err := ptpingRun(ifacef, dscpf, serverf, countf, timeoutf); err != nil {
			log.Fatal(err)
		}
	},
}
