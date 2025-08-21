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
	"math"
	"math/rand"
	"net"
	"net/netip"
	"time"

	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/sptp/client"
	"github.com/facebook/time/timestamp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// flags
var (
	ifacef     string
	countf     int
	dscpf      int
	timeoutf   time.Duration
	listenAddr string
)

func init() {
	RootCmd.AddCommand(ptpingCmd)
	ptpingCmd.Flags().StringVarP(&ifacef, "iface", "i", "eth0", "network interface to use")
	ptpingCmd.Flags().StringVarP(&listenAddr, "listenaddr", "l", "::", "IP address to use")
	ptpingCmd.Flags().IntVarP(&countf, "count", "c", 5, "number of probes to send")
	ptpingCmd.Flags().IntVarP(&dscpf, "dscp", "d", 35, "dscp value (QoS)")
	ptpingCmd.Flags().DurationVarP(&timeoutf, "timeout", "t", time.Second, "request timeout/interval")
}

type timestamps struct {
	t1 time.Time
	t2 time.Time
	t3 time.Time
	t4 time.Time
	ts time.Time // software timestamp SYNC message received
}

func (t *timestamps) reset() {
	t.t1 = time.Time{}
	t.t2 = time.Time{}
	t.t3 = time.Time{}
	t.t4 = time.Time{}
	t.ts = time.Time{}
}

type ptping struct {
	iface  string
	dscp   int
	target netip.Addr

	clockID   ptp.ClockIdentity
	eventConn client.UDPConnWithTS
	client    *client.Client
	ts        timestamps
}

func (p *ptping) init() error {
	iface, err := net.InterfaceByName(p.iface)
	if err != nil {
		return err
	}

	cid, err := ptp.NewClockIdentity(iface.HardwareAddr)
	if err != nil {
		return err
	}
	p.clockID = cid

	p.eventConn, err = client.NewUDPConnTS(net.ParseIP(listenAddr), 0, timestamp.HW, iface, p.dscp)
	if err != nil {
		return err
	}
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
			p.ts.ts = time.Now()
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

func ptpingOutput(count int, server string, totalRTT time.Duration, ts timestamps) {
	fw := ts.t4.Sub(ts.t3)
	bk := ts.t2.Sub(ts.t1)
	netRTT := fw + bk
	// if we didn't get a valid T1 or T2, set the reverse latency to 0 (
	if ts.t1.IsZero() || ts.t2.IsZero() {
		bk = 0
	}
	if bk == 0 {
		fmt.Printf("%s: seq=%d net=%f (->%s + <-%f)\trtt=%s\n", server, count, math.NaN(), fw, math.NaN(), totalRTT)
	} else {
		fmt.Printf("%s: seq=%d net=%s (->%s + <-%s)\trtt=%s\n", server, count, netRTT, fw, bk, totalRTT)
	}
}

func ptpingRun(iface string, dscp int, server string, count int, timeout time.Duration) error {
	var err error
	p := &ptping{
		iface: iface,
		dscp:  dscp,
	}

	p.target, err = client.LookupNetIP(server)
	if err != nil {
		return err
	}

	if err = p.init(); err != nil {
		return err
	}
	// We want to avoid first 10 which may be used by other tools.
	// Intn(65524) will generate a random number between 0 and 65524
	portID := uint16(rand.Intn(65524) + 11)

	for c := 1; c <= count; c++ {
		p.ts.reset()
		start := time.Now()
		_, p.ts.t3, err = p.client.SendEventMsg(client.ReqDelay(p.clockID, portID))

		if err != nil {
			log.Errorf("failed to send request: %s", err)
			continue
		}

		if err = p.timestamps(timeout); err != nil {
			log.Errorf("failed to read sync response: %v", err)
			continue
		}
		totalRTT := p.ts.ts.Sub(start)
		ptpingOutput(c, server, totalRTT, p.ts)
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
