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

type timestamps struct {
	t1 time.Time
	t2 time.Time
	t3 time.Time
	t4 time.Time
}

func (t *timestamps) reset() {
	t.t1 = time.Time{}
	t.t2 = time.Time{}
	t.t3 = time.Time{}
	t.t4 = time.Time{}
}

type ptping struct {
	iface  string
	dscp   int
	target string

	clockID   ptp.ClockIdentity
	eventConn client.UDPConnWithTS
	client    *client.Client
	ts        timestamps
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
	go func() {
		if err := p.runReader(); err != nil {
			log.Error(err)
		}
	}()

	return err
}

// timestamps fills timestamps
func (p *ptping) timestamps(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	<-ctx.Done()
	if p.ts.t4.IsZero() {
		return fmt.Errorf("timeout waiting")
	}
	return nil
}

func (p *ptping) runReader() error {
	sync := &ptp.SyncDelayReq{}
	buf := make([]byte, timestamp.PayloadSizeBytes)
	oob := make([]byte, timestamp.ControlSizeBytes)
	for {
		bbuf, _, rxts, _ := p.eventConn.ReadPacketWithRXTimestampBuf(buf, oob)
		msgType, err := ptp.ProbeMsgType(buf[:bbuf])
		if err != nil {
			return fmt.Errorf("can't read a message type")
		}

		switch msgType {
		case ptp.MessageSync, ptp.MessageDelayReq:
			p.ts.t2 = rxts
			if err = ptp.FromBytes(buf[:bbuf], sync); err != nil {
				return fmt.Errorf("reading sync msg: %w", err)
			}
			p.ts.t4 = sync.OriginTimestamp.Time()
		case ptp.MessageAnnounce:
			announce := &ptp.Announce{}
			if err = ptp.FromBytes(buf[:bbuf], announce); err != nil {
				return fmt.Errorf("reading announce msg: %w", err)
			}
			p.ts.t1 = announce.OriginTimestamp.Time()
		default:
			log.Infof("got unsupported packet %v:", msgType)
		}
	}
}

func ptpingRun(iface string, dscp int, server string, count int, timeout time.Duration) error {
	var err error
	p := &ptping{
		iface:  iface,
		dscp:   dscp,
		target: server,
	}

	if err = p.init(); err != nil {
		return err
	}
	// We want to avoid first 10 which may be used by other tools
	portID := uint16(rand.Intn(10+65535) - 10)

	for c := 1; c <= count; c++ {
		p.ts.reset()
		_, p.ts.t3, err = p.client.SendEventMsg(client.ReqDelay(p.clockID, portID))
		if err != nil {
			log.Errorf("failed to send request: %s", err)
			continue
		}

		if err = p.timestamps(timeout); err != nil {
			log.Errorf("failed to read sync response: %v", err)
			continue
		}
		fw := p.ts.t4.Sub(p.ts.t3)
		bk := p.ts.t2.Sub(p.ts.t1)
		if p.ts.t1.IsZero() {
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
