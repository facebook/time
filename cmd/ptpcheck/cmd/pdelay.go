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
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/facebook/time/ptp/pdelay"
	ptp "github.com/facebook/time/ptp/protocol"
	"github.com/facebook/time/ptp/sptp/client"
	"github.com/facebook/time/timestamp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

// Default probe configuration
const (
	// DefaultProbeInterval is 5 minutes as per PTP in-rack linearizability requirements
	DefaultProbeInterval = 5 * time.Minute
	// DefaultProbeJitterMax is maximum random jitter to add to probe interval
	DefaultProbeJitterMax = 30 * time.Second
)

// flags for pdelay command
var (
	pdelayIfacef    string
	pdelayIntervalf time.Duration
	pdelayJitterf   time.Duration
	pdelayDscpf     int
	pdelayTimeoutf  time.Duration
	pdelayTsf       string
	pdelayIPv4f     bool
	pdelayCountf    int
)

func init() {
	RootCmd.AddCommand(pdelayCmd)
	pdelayCmd.Flags().StringVarP(&pdelayIfacef, "iface", "i", "eth0", "network interface to use")
	pdelayCmd.Flags().DurationVarP(&pdelayIntervalf, "interval", "I", DefaultProbeInterval, "probe interval (e.g., 5m)")
	pdelayCmd.Flags().DurationVarP(&pdelayJitterf, "jitter", "j", DefaultProbeJitterMax, "maximum random jitter to add to interval")
	pdelayCmd.Flags().IntVarP(&pdelayDscpf, "dscp", "d", 35, "DSCP value (QoS)")
	pdelayCmd.Flags().DurationVarP(&pdelayTimeoutf, "timeout", "t", time.Second, "timeout for collecting responses")
	pdelayCmd.Flags().StringVarP(&pdelayTsf, "timestamping", "T", "hardware", "timestamping mode (hardware or software)")
	pdelayCmd.Flags().BoolVarP(&pdelayIPv4f, "ipv4", "4", false, "use IPv4 multicast (224.0.0.107) instead of IPv6 (ff02::6b)")
	pdelayCmd.Flags().IntVarP(&pdelayCountf, "count", "c", 0, "number of peer delay requests to send (0 = run continuously with interval)")
}

// CalculateJitter returns a random duration between 0 and maxJitter
func CalculateJitter(maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(maxJitter)))
}

// ProbeConfig holds configuration for periodic probing
type ProbeConfig struct {
	Iface        string
	Interval     time.Duration
	Jitter       time.Duration
	Dscp         int
	Timeout      time.Duration
	Timestamping string
	IPv4         bool
	// Count is the number of probe cycles to run. 0 means run continuously.
	Count int
}

// ProbeResultCallback is called with all measurement results after each probe cycle
type ProbeResultCallback func(results []*pdelay.Result)

// OnProbeResult is an optional callback that gets called with all results from each probe cycle.
// Set this from external packages (e.g., internal) to add custom logging.
var OnProbeResult ProbeResultCallback

// RunPeriodicProbe runs periodic peer delay measurements using multicast
// Sends PDelay_Req to ff02::6b multicast address as per IEEE 1588 specification
// All SPTP clients in the rack that joined the multicast group will respond
// If cfg.Count > 0, runs exactly that many probe cycles back-to-back without
// waiting for the interval between them, then returns. Otherwise, runs
// continuously using cfg.Interval (plus jitter) between cycles.
func RunPeriodicProbe(ctx context.Context, cfg ProbeConfig) error {
	multicastStr := ptp.PDelayMulticastIPv6
	periodic := false
	if cfg.IPv4 {
		multicastStr = ptp.PDelayMulticastIPv4
	}
	if cfg.Count > 0 {
		log.Infof("[pdelay] starting multicast probe on %s (count: %d)", cfg.Iface, cfg.Count)
	} else {
		log.Infof("[pdelay] starting periodic multicast probe on %s (interval: %s, jitter: %s)", cfg.Iface, cfg.Interval, cfg.Jitter)
		periodic = true
	}
	log.Infof("[pdelay] sending PDelay_Req to multicast address %s", multicastStr)
	nextProbe := time.Second

	for periodic || cfg.Count > 0 {
		// Calculate next probe time with jitter
		if periodic {
			jitter := CalculateJitter(cfg.Jitter)
			nextProbe = cfg.Interval + jitter
		} else {
			cfg.Count--
		}

		log.Debugf("[pdelay] waiting %s until next probe cycle", nextProbe)

		select {
		case <-ctx.Done():
			log.Infof("[pdelay] stopping probes")
			return ctx.Err()
		case <-time.After(nextProbe):
		}

		// Run multicast probe
		results, err := runMulticastProbe(cfg)
		if err != nil {
			log.Warningf("[pdelay] multicast probe failed: %v", err)
			continue
		}

		log.Infof("[pdelay] received %d responses from rack servers", len(results))

		// Call callback with all results from this cycle
		if OnProbeResult != nil {
			OnProbeResult(results)
		}
	}
	return nil
}

// selectNonLinkLocalAddr returns the first global unicast IPv6 address from addrs.
// Returns error if no suitable address is found.
func selectNonLinkLocalAddr(addrs []net.Addr) (net.IP, error) {
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP
		if ip.To4() != nil {
			continue
		}
		if ip.IsLinkLocalUnicast() {
			continue
		}
		if ip.IsGlobalUnicast() {
			return ip, nil
		}
	}
	return nil, fmt.Errorf("no global unicast IPv6 address found")
}

// getNonLinkLocalAddr returns a global unicast IPv6 address for the given interface.
func getNonLinkLocalAddr(iface *net.Interface) (net.IP, error) {
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, fmt.Errorf("getting addresses for %s: %w", iface.Name, err)
	}
	return selectNonLinkLocalAddr(addrs)
}

// getIPv4Addr returns the first IPv4 address for the given interface.
func getIPv4Addr(iface *net.Interface) (net.IP, error) {
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, fmt.Errorf("getting addresses for %s: %w", iface.Name, err)
	}
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if ip := ipNet.IP.To4(); ip != nil {
			return ip, nil
		}
	}
	return nil, fmt.Errorf("no IPv4 address found on interface %s", iface.Name)
}

// pdelaySequence is the sequence counter for PDelay_Req messages
var pdelaySequence uint16

// runMulticastProbe sends a PDelay_Req to the multicast address and collects responses
func runMulticastProbe(cfg ProbeConfig) ([]*pdelay.Result, error) {
	iface, err := net.InterfaceByName(cfg.Iface)
	if err != nil {
		return nil, fmt.Errorf("getting interface %s: %w", cfg.Iface, err)
	}

	// Parse timestamping mode
	var ts timestamp.Timestamp
	switch cfg.Timestamping {
	case "hardware", "hw":
		ts = timestamp.HW
	case "software", "sw":
		ts = timestamp.SW
	default:
		return nil, fmt.Errorf("invalid timestamping mode: %s", cfg.Timestamping)
	}

	cid, err := ptp.NewClockIdentity(iface.HardwareAddr)
	if err != nil {
		return nil, fmt.Errorf("creating clock identity: %w", err)
	}

	// Select source address based on address family
	var srcIP net.IP
	if cfg.IPv4 {
		srcIP, err = getIPv4Addr(iface)
	} else {
		srcIP, err = getNonLinkLocalAddr(iface)
	}
	if err != nil {
		return nil, fmt.Errorf("finding source address on %s: %w", cfg.Iface, err)
	}

	// Create connection for sending/receiving
	conn, err := client.NewUDPConnTS(srcIP, 0, ts, iface, cfg.Dscp)
	if err != nil {
		return nil, fmt.Errorf("creating UDP connection: %w", err)
	}
	defer conn.Close()

	// A host peer-delay-measuring itself is meaningless, and a locally-originated
	// multicast PDelay_Req is delivered to same-host group members (e.g. the local
	// sptp) without an RX timestamp, which sptp treats as a fatal error. Disable
	// multicast loopback so the probe is never delivered back to this host.
	loopLevel, loopOpt := unix.IPPROTO_IPV6, unix.IPV6_MULTICAST_LOOP
	if cfg.IPv4 {
		loopLevel, loopOpt = unix.IPPROTO_IP, unix.IP_MULTICAST_LOOP
	}
	if err := unix.SetsockoptInt(conn.ConnFd(), loopLevel, loopOpt, 0); err != nil {
		return nil, fmt.Errorf("disabling multicast loopback: %w", err)
	}

	timestamp.AttemptsTXTS = 5
	timestamp.TimeoutTXTS = 100 * time.Millisecond

	// Send PDelay_Req to multicast address with sequential sequence number
	seq := pdelaySequence
	pdelaySequence++
	req := ptp.ReqPDelay(cid, 1, seq)
	reqBytes, err := ptp.Bytes(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling PDelay_Req: %w", err)
	}

	// Select multicast address based on address family
	var multicastAddr netip.Addr
	var multicastStr string
	if cfg.IPv4 {
		multicastStr = ptp.PDelayMulticastIPv4
		multicastAddr = netip.MustParseAddr(multicastStr)
	} else {
		multicastStr = ptp.PDelayMulticastIPv6
		multicastAddr = netip.MustParseAddr(multicastStr).WithZone(cfg.Iface)
	}
	targetAddr := timestamp.AddrToSockaddr(multicastAddr, ptp.PortEvent)

	t1, err := conn.WriteToWithTS(reqBytes, targetAddr, seq)
	if err != nil {
		return nil, fmt.Errorf("sending PDelay_Req to multicast: %w", err)
	}

	log.Debugf("[pdelay] sent PDelay_Req seq=%d to %s, T1=%v", seq, multicastStr, t1)

	// Collect responses with timeout
	return collectMulticastResponses(conn, seq, t1, cfg.Timeout), nil
}

// collectMulticastResponses collects PDelay responses from multiple responders
func collectMulticastResponses(conn client.UDPConnWithTS, expectedSeq uint16, t1 time.Time, timeout time.Duration) []*pdelay.Result {
	buf := make([]byte, timestamp.PayloadSizeBytes)
	oob := make([]byte, timestamp.ControlSizeBytes)

	// Track responses by source address
	type responderState struct {
		t2       time.Time     // From PDelay_Resp
		t3       time.Time     // From PDelay_Resp_Follow_Up
		t4       time.Time     // RX timestamp of PDelay_Resp
		cfReq    time.Duration // CorrectionField from PDelay_Resp (request path)
		cfResp   time.Duration // CorrectionField from PDelay_Resp_Follow_Up (response path)
		respAddr netip.Addr
	}
	responders := make(map[string]*responderState)

	deadline := time.Now().Add(timeout)

	// Set socket read timeout so ReadPacketWithRXTimestampBuf doesn't block forever
	tv := unix.NsecToTimeval(timeout.Nanoseconds())
	if err := unix.SetsockoptTimeval(conn.ConnFd(), unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv); err != nil {
		log.Warningf("[pdelay] failed to set socket timeout: %v", err)
	}

	for time.Now().Before(deadline) {
		// Set read deadline
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}

		n, saddr, rxts, err := conn.ReadPacketWithRXTimestampBuf(buf, oob)
		if err != nil {
			// Timeout or error - stop collecting
			break
		}

		// Get source address
		srcAddr := timestamp.SockaddrToAddr(saddr)
		srcKey := srcAddr.String()

		msgType, err := ptp.ProbeMsgType(buf[:n])
		if err != nil {
			log.Debugf("[pdelay] failed to probe message type: %v", err)
			continue
		}

		switch msgType {
		case ptp.MessagePDelayResp:
			resp := &ptp.PDelayResp{}
			if err := ptp.FromBytes(buf[:n], resp); err != nil {
				log.Debugf("[pdelay] failed to parse PDelay_Resp: %v", err)
				continue
			}

			if resp.SequenceID != expectedSeq {
				log.Debugf("[pdelay] ignoring PDelay_Resp with wrong seq: got %d, expected %d", resp.SequenceID, expectedSeq)
				continue
			}

			state, ok := responders[srcKey]
			if !ok {
				state = &responderState{respAddr: srcAddr}
				responders[srcKey] = state
			}
			state.t2 = resp.RequestReceiptTimestamp.Time()
			state.t4 = rxts
			state.cfReq = resp.CorrectionField.Duration()

			log.Debugf("[pdelay] received PDelay_Resp from %s, T2=%v, T4=%v, CFReq=%v", srcAddr, state.t2, state.t4, state.cfReq)

			// Check for one-step mode
			if resp.FlagField&ptp.FlagTwoStep == 0 {
				state.t3 = state.t2
			}

		case ptp.MessagePDelayRespFollowUp:
			followUp := &ptp.PDelayRespFollowUp{}
			if err := ptp.FromBytes(buf[:n], followUp); err != nil {
				log.Debugf("[pdelay] failed to parse PDelay_Resp_Follow_Up: %v", err)
				continue
			}

			if followUp.SequenceID != expectedSeq {
				continue
			}

			state, ok := responders[srcKey]
			if !ok {
				state = &responderState{respAddr: srcAddr}
				responders[srcKey] = state
			}
			state.t3 = followUp.ResponseOriginTimestamp.Time()
			state.cfResp = followUp.CorrectionField.Duration()

			log.Debugf("[pdelay] received PDelay_Resp_Follow_Up from %s, T3=%v, CFResp=%v", srcAddr, state.t3, state.cfResp)
		}
	}

	// Convert collected responses to results
	results := make([]*pdelay.Result, 0, len(responders))
	for _, state := range responders {
		result := &pdelay.Result{
			Responder:           state.respAddr,
			T1:                  t1,
			T2:                  state.t2,
			T3:                  state.t3,
			T4:                  state.t4,
			CorrectionFieldReq:  state.cfReq,
			CorrectionFieldResp: state.cfResp,
			Timestamp:           time.Now(),
		}

		if !result.Valid() {
			result.Error = fmt.Errorf("incomplete response")
		}

		results = append(results, result)

		// Print output
		if result.Valid() {
			fmt.Printf("%s: offset=%s path_delay=%s\n", state.respAddr, result.Offset(), result.PathDelay())
		} else {
			fmt.Printf("%s: incomplete response\n", state.respAddr)
		}
	}

	return results
}

var pdelayCmd = &cobra.Command{
	Use:   "pdelay",
	Short: "Periodic in-rack peer delay measurement using multicast",
	Long: `Run periodic peer delay measurements against all hosts in the same rack.

This command sends PDelay_Req messages to the PTP peer delay multicast address
(ff02::6b for IPv6, 224.0.0.107 for IPv4) as per IEEE 1588. All SPTP clients
in the same L2 domain (rack) that have joined the multicast group will receive
the request and respond.

The probe runs every 5 minutes (configurable) with random jitter to avoid
synchronized probing across the fleet. This implements in-rack linearizability
checks for detecting time offset asymmetries between servers.

Pdelay_Req, Pdelay_Resp, and Pdelay_Resp_Follow_Up messages are used for
accurate peer delay measurement with hardware timestamping.`,
	Args: cobra.NoArgs,
	Run: func(_ *cobra.Command, _ []string) {
		ConfigureVerbosity()

		cfg := ProbeConfig{
			Iface:        pdelayIfacef,
			Interval:     pdelayIntervalf,
			Jitter:       pdelayJitterf,
			Dscp:         pdelayDscpf,
			Timeout:      pdelayTimeoutf,
			Timestamping: pdelayTsf,
			IPv4:         pdelayIPv4f,
			Count:        pdelayCountf,
		}

		// Set up signal handling for graceful shutdown
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-sigChan
			log.Infof("[pdelay] received shutdown signal")
			cancel()
		}()

		if err := RunPeriodicProbe(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
	},
}
